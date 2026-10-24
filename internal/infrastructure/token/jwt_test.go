package token

import (
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestStrictJWTValidation(t *testing.T) {
	secret := "test-only-secret-at-least-thirty-two-bytes"
	issuer := New(secret, time.Minute)
	raw, _, err := issuer.Issue(42, "session-id")
	if err != nil {
		t.Fatal(err)
	}
	got, err := issuer.Parse(raw)
	if err != nil || got.UserID != 42 || got.SessionID != "session-id" {
		t.Fatalf("claims: %+v %v", got, err)
	}
	if _, err := New("different-secret", time.Minute).Parse(raw); err == nil {
		t.Fatal("accepted wrong signature")
	}
	expired, _, _ := New(secret, -time.Minute).Issue(42, "session-id")
	if _, err := issuer.Parse(expired); err == nil {
		t.Fatal("accepted expired token")
	}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS256, jwt.SigningMethodHS384} {
		c := claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "42", Issuer: "highload-counters", Audience: jwt.ClaimStrings{"highload-counters"}}, SessionID: "session-id"}
		if method == jwt.SigningMethodHS384 {
			c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Minute))
		}
		forged, _ := jwt.NewWithClaims(method, c).SignedString([]byte(secret))
		if _, err := issuer.Parse(forged); err == nil {
			t.Fatal("accepted missing expiry or incorrect algorithm")
		}
	}
}
