package auth

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"highload-counters/internal/domain"
	"highload-counters/internal/transport/rest/response"
	usecase "highload-counters/internal/usecase/auth"
	"highload-counters/pkg/traceid"
)

type RateLimiter interface {
	Allow(context.Context, string) (bool, error)
}
type Handler struct {
	Service *usecase.Service
	Limiter RateLimiter
	Timeout time.Duration
	slots   chan struct{}
}

func New(service *usecase.Service, limiter RateLimiter, timeout time.Duration) *Handler {
	return &Handler{Service: service, Limiter: limiter, Timeout: timeout, slots: make(chan struct{}, 4)}
}
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	for _, action := range []string{"register", "login", "refresh", "logout", "forgot-password", "reset-password"} {
		mux.Handle("POST /v1/auth/"+action, h.wrap(h.action(action), true))
	}
	mux.Handle("GET /v1/users/me", h.wrap(h.me, false))
}

type handlerFunc func(http.ResponseWriter, *http.Request) error

func (h *Handler) wrap(fn handlerFunc, limited bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := traceid.New()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
		defer cancel()
		r = r.WithContext(ctx)
		var err error
		if limited {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			var allowed bool
			allowed, err = h.Limiter.Allow(ctx, host)
			if err != nil {
				if ctx.Err() != nil {
					err = ctx.Err()
				} else {
					err = domain.ErrUnavailable
				}
			} else if !allowed {
				err = domain.ErrRateLimited
			}
			if err == nil {
				select {
				case h.slots <- struct{}{}:
					defer func() { <-h.slots }()
				case <-ctx.Done():
					err = ctx.Err()
				}
			}
		}
		if err == nil {
			err = fn(w, r)
		}
		if err != nil {
			response.HandleError(w, r, id, err)
		}
	})
}
func decode(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return domain.ErrValidation
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ErrValidation
	}
	return nil
}
func respond(w http.ResponseWriter, status int, body any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
	return nil
}
func (h *Handler) action(action string) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		switch action {
		case "register":
			var in struct {
				Email    string `json:"email"`
				Password string `json:"password"`
				Name     string `json:"name"`
			}
			if err := decode(w, r, &in); err != nil {
				return err
			}
			pair, err := h.Service.Register(r.Context(), in.Email, in.Password, in.Name)
			if err != nil {
				return err
			}
			return respond(w, 201, pair)
		case "login":
			var in struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			if err := decode(w, r, &in); err != nil {
				return err
			}
			pair, err := h.Service.Login(r.Context(), in.Email, in.Password)
			if err != nil {
				return err
			}
			return respond(w, 200, pair)
		case "refresh", "logout":
			var in struct {
				RefreshToken string `json:"refreshToken"`
			}
			if err := decode(w, r, &in); err != nil {
				return err
			}
			if action == "logout" {
				if err := h.Service.Logout(r.Context(), in.RefreshToken); err != nil {
					return err
				}
				return respond(w, 204, nil)
			}
			pair, err := h.Service.Refresh(r.Context(), in.RefreshToken)
			if err != nil {
				return err
			}
			return respond(w, 200, pair)
		case "forgot-password":
			var in struct {
				Email string `json:"email"`
			}
			if err := decode(w, r, &in); err != nil {
				return err
			}
			if err := h.Service.ForgotPassword(r.Context(), in.Email); err != nil {
				return err
			}
			return respond(w, 202, map[string]string{"message": "If the account exists, a reset email has been requested."})
		case "reset-password":
			var in struct {
				Token    string `json:"token"`
				Password string `json:"password"`
			}
			if err := decode(w, r, &in); err != nil {
				return err
			}
			if err := h.Service.ResetPassword(r.Context(), in.Token, in.Password); err != nil {
				return err
			}
			return respond(w, 204, nil)
		}
		return domain.ErrNotFound
	}
}
func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return domain.ErrUnauthorized
	}
	user, err := h.Service.Me(r.Context(), parts[1])
	if err != nil {
		return err
	}
	return respond(w, 200, struct {
		ID        int64     `json:"id,string"`
		Email     string    `json:"email"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"createdAt"`
	}{user.ID, user.Email, user.Name, user.CreatedAt})
}
