package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

const testSecret = "test-secret"

// sign builds a compact JWS from raw header/payload JSON, exactly as a real
// signer would, so tests exercise the same wire format Verify parses.
func sign(headerJSON, payloadJSON, secret string) string {
	enc := base64.RawURLEncoding.EncodeToString
	signingInput := enc([]byte(headerJSON)) + "." + enc([]byte(payloadJSON))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	return signingInput + "." + enc(mac.Sum(nil))
}

func token(t *testing.T, claims map[string]any, secret string) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return sign(`{"alg":"HS256","typ":"JWT"}`, string(payload), secret)
}

func TestVerify(t *testing.T) {
	now := time.Now().Unix()

	cases := []struct {
		name  string
		token string
		want  error
	}{
		{"valid with exp", token(t, map[string]any{"exp": now + 3600}, testSecret), nil},
		{"missing exp rejected", token(t, map[string]any{}, testSecret), ErrNoExpiry},
		{"wrong secret", token(t, map[string]any{"exp": now + 3600}, "other"), ErrSignature},
		{"expired", token(t, map[string]any{"exp": now - 1}, testSecret), ErrExpired},
		{"not yet valid", token(t, map[string]any{"nbf": now + 3600, "exp": now + 7200}, testSecret), ErrNotYet},
		{"alg none", sign(`{"alg":"none"}`, `{}`, testSecret), ErrAlgorithm},
		{"missing parts", "a.b", ErrMalformed},
		{"garbage segments", "!.!.!", ErrMalformed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Verify(tc.token, testSecret); !errors.Is(err, tc.want) {
				t.Fatalf("Verify() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestVerifyTamperedPayload(t *testing.T) {
	// Swap the payload for a different one without re-signing: the signature no
	// longer matches, so it must be rejected.
	valid := token(t, map[string]any{"exp": time.Now().Unix() + 3600}, testSecret)
	forged := token(t, map[string]any{"exp": time.Now().Unix() + 999999}, testSecret)

	parts := func(s string) []string { return splitDots(s) }
	tampered := parts(valid)[0] + "." + parts(forged)[1] + "." + parts(valid)[2]
	if err := Verify(tampered, testSecret); !errors.Is(err, ErrSignature) {
		t.Fatalf("Verify(tampered) = %v, want %v", err, ErrSignature)
	}
}

func splitDots(s string) []string {
	out := []string{"", "", ""}
	i := 0
	for _, r := range s {
		if r == '.' && i < 2 {
			i++
			continue
		}
		out[i] += string(r)
	}
	return out
}
