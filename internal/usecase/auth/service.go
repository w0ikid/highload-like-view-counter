package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"highload-counters/internal/domain"
	"highload-counters/pkg/passwordhash"
)

type Config struct{ RefreshTTL, ResetTTL, DBTimeout time.Duration }
type Service struct {
	users       UserRepository
	credentials CredentialRepository
	sessions    SessionRepository
	resets      PasswordResetRepository
	tx          TxManager
	tokens      TokenIssuer
	mail        Mailer
	cfg         Config
}

func New(users UserRepository, credentials CredentialRepository, sessions SessionRepository, resets PasswordResetRepository, tx TxManager, tokens TokenIssuer, mail Mailer, cfg Config) *Service {
	return &Service{users, credentials, sessions, resets, tx, tokens, mail, cfg}
}

func normalizeEmail(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 254 || strings.ContainsAny(value, "\r\n") {
		return "", domain.ErrEmailInvalid
	}
	return value, nil
}
func validatePassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 8 || utf8.RuneCountInString(password) > 128 {
		return domain.ErrPasswordInvalid
	}
	return nil
}
func randomToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func validToken(raw string) bool {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil && len(b) == 32
}
func authError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrTokenInvalid
	}
	return err
}

func (s *Service) transaction(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.DBTimeout)
	defer cancel()
	return s.tx.WithinTx(ctx, fn)
}

func (s *Service) Register(ctx context.Context, email, password, name string) (domain.TokenPair, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return domain.TokenPair{}, err
	}
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 100 {
		return domain.TokenPair{}, domain.ErrUserNameInvalid
	}
	if err = validatePassword(password); err != nil {
		return domain.TokenPair{}, err
	}
	if err = ctx.Err(); err != nil {
		return domain.TokenPair{}, err
	}
	hash, err := passwordhash.Hash(password)
	if err != nil {
		return domain.TokenPair{}, err
	}
	var pair domain.TokenPair
	err = s.transaction(ctx, func(ctx context.Context) error {
		user, err := s.users.Create(ctx, domain.CreateUserInput{Email: email, Name: name})
		if err != nil {
			return err
		}
		if err = s.credentials.Set(ctx, user.ID, hash); err != nil {
			return err
		}
		pair, err = s.issue(ctx, user.ID)
		return err
	})
	return pair, err
}

var dummyHash = sync.OnceValue(func() string { hash, _ := passwordhash.Hash("not-a-user-password"); return hash })

func (s *Service) Login(ctx context.Context, email, password string) (domain.TokenPair, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return domain.TokenPair{}, err
	}
	if err = validatePassword(password); err != nil {
		return domain.TokenPair{}, err
	}
	// Read/hash outside the transaction, then recheck the credential under the user lock.
	dbctx, cancel := context.WithTimeout(ctx, s.cfg.DBTimeout)
	user, err := s.users.FindByEmail(dbctx, email)
	var cred domain.Credential
	if err == nil {
		cred, err = s.credentials.Find(dbctx, user.ID)
	}
	cancel()
	if errors.Is(err, domain.ErrNotFound) {
		passwordhash.Verify(password, dummyHash())
		return domain.TokenPair{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.TokenPair{}, err
	}
	if err = ctx.Err(); err != nil {
		return domain.TokenPair{}, err
	}
	if !passwordhash.Verify(password, cred.PasswordHash) {
		return domain.TokenPair{}, domain.ErrInvalidCredentials
	}
	var pair domain.TokenPair
	err = s.transaction(ctx, func(ctx context.Context) error {
		if err := s.users.Lock(ctx, user.ID); err != nil {
			return err
		}
		current, err := s.credentials.Find(ctx, user.ID)
		if err != nil {
			return err
		}
		if current.PasswordHash != cred.PasswordHash {
			return domain.ErrInvalidCredentials
		}
		pair, err = s.issue(ctx, user.ID)
		return err
	})
	return pair, err
}

func (s *Service) issue(ctx context.Context, userID int64) (domain.TokenPair, error) {
	raw := randomToken()
	id := uuid.NewString()
	access, exp, err := s.tokens.Issue(userID, id)
	if err != nil {
		return domain.TokenPair{}, err
	}
	err = s.sessions.Create(ctx, domain.Session{ID: id, UserID: userID, RefreshTokenHash: tokenHash(raw), ExpiresAt: time.Now().Add(s.cfg.RefreshTTL)})
	return domain.TokenPair{AccessToken: access, RefreshToken: raw, ExpiresAt: exp}, err
}

func (s *Service) Refresh(ctx context.Context, raw string) (domain.TokenPair, error) {
	if !validToken(raw) {
		return domain.TokenPair{}, domain.ErrTokenInvalid
	}
	var pair domain.TokenPair
	err := s.transaction(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.FindByHash(ctx, tokenHash(raw))
		if err != nil {
			return authError(err)
		}
		if err = s.users.Lock(ctx, sess.UserID); err != nil {
			return err
		}
		// Re-read after lock: parallel refresh/reset/logout may have revoked it.
		sess, err = s.sessions.FindByHash(ctx, tokenHash(raw))
		if err != nil {
			return authError(err)
		}
		if !sess.Active(time.Now()) {
			return domain.ErrTokenInvalid
		}
		if err = s.sessions.Revoke(ctx, sess.ID); err != nil {
			return err
		}
		pair, err = s.issue(ctx, sess.UserID)
		return err
	})
	return pair, err
}

func (s *Service) Logout(ctx context.Context, raw string) error {
	if !validToken(raw) {
		return domain.ErrTokenInvalid
	}
	return s.transaction(ctx, func(ctx context.Context) error {
		sess, err := s.sessions.FindByHash(ctx, tokenHash(raw))
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = s.users.Lock(ctx, sess.UserID); err != nil {
			return err
		}
		return s.sessions.Revoke(ctx, sess.ID)
	})
}

func (s *Service) Me(ctx context.Context, raw string) (domain.User, error) {
	claims, err := s.tokens.Parse(raw)
	if err != nil {
		return domain.User{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.DBTimeout)
	defer cancel()
	sess, err := s.sessions.FindByID(ctx, claims.SessionID)
	if err != nil {
		return domain.User{}, authError(err)
	}
	if sess.UserID != claims.UserID || !sess.Active(time.Now()) {
		return domain.User{}, domain.ErrTokenInvalid
	}
	user, err := s.users.FindByID(ctx, claims.UserID)
	return user, authError(err)
}

func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	raw := randomToken()
	found := false
	err = s.transaction(ctx, func(ctx context.Context) error {
		user, err := s.users.FindByEmail(ctx, email)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err = s.users.Lock(ctx, user.ID); err != nil {
			return err
		}
		if err = s.resets.InvalidateByUser(ctx, user.ID); err != nil {
			return err
		}
		found = true
		return s.resets.Create(ctx, domain.PasswordReset{UserID: user.ID, TokenHash: tokenHash(raw), ExpiresAt: time.Now().Add(s.cfg.ResetTTL)})
	})
	if err != nil {
		return err
	}
	if found {
		// Same HTTP response for unknown email and SMTP failure; never log the raw token.
		if err = s.mail.SendPasswordReset(ctx, email, raw); err != nil {
			slog.WarnContext(ctx, "password reset mail delivery failed")
		}
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, raw, password string) error {
	if !validToken(raw) {
		return domain.ErrTokenInvalid
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hash, err := passwordhash.Hash(password)
	if err != nil {
		return err
	}
	return s.transaction(ctx, func(ctx context.Context) error {
		reset, err := s.resets.FindByHash(ctx, tokenHash(raw))
		if err != nil {
			return authError(err)
		}
		if err = s.users.Lock(ctx, reset.UserID); err != nil {
			return err
		}
		ok, err := s.resets.Consume(ctx, tokenHash(raw))
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrTokenInvalid
		}
		if err = s.credentials.Set(ctx, reset.UserID, hash); err != nil {
			return err
		}
		if err = s.sessions.RevokeByUser(ctx, reset.UserID); err != nil {
			return err
		}
		return s.resets.InvalidateByUser(ctx, reset.UserID)
	})
}
