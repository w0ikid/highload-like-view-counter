package token

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"highload-counters/internal/domain"
	"strconv"
	"time"
)

type claims struct {
	jwt.RegisteredClaims
	SessionID string `json:"sid"`
}
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func New(secret string, ttl time.Duration) *Issuer { return &Issuer{[]byte(secret), ttl} }
func (i *Issuer) Issue(userID int64, sessionID string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(i.ttl)
	c := claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: strconv.FormatInt(userID, 10), Issuer: "highload-counters", Audience: jwt.ClaimStrings{"highload-counters"},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
	}, SessionID: sessionID}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(i.secret)
	return raw, exp, err
}
func (i *Issuer) Parse(raw string) (domain.AccessClaims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(raw, &c, func(_ *jwt.Token) (any, error) { return i.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("highload-counters"), jwt.WithAudience("highload-counters"), jwt.WithExpirationRequired())
	if errors.Is(err, jwt.ErrTokenExpired) {
		return domain.AccessClaims{}, domain.ErrTokenExpired
	}
	if err != nil {
		return domain.AccessClaims{}, domain.ErrTokenInvalid
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil || id <= 0 || c.SessionID == "" {
		return domain.AccessClaims{}, domain.ErrTokenInvalid
	}
	return domain.AccessClaims{UserID: id, SessionID: c.SessionID}, nil
}
