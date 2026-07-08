// Package auth verifies the JWT that gates WebTransport connections. Tokens are
// HS256-signed with a shared secret; the payload carries the standard exp/nbf
// timing claims. Verification is stdlib-only (no external JWT dependency): it
// enforces alg=HS256, a valid HMAC signature (compared in constant time), a
// mandatory exp that has not passed, and — if present — nbf. A valid signature
// only guarantees the issuer's claims are untampered; it does not force an exp
// to exist, so we require one here to rule out eternal tokens.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrMalformed = errors.New("malformed token")
	ErrAlgorithm = errors.New("unexpected signing algorithm")
	ErrSignature = errors.New("invalid signature")
	ErrNoExpiry  = errors.New("token missing exp claim")
	ErrExpired   = errors.New("token expired")
	ErrNotYet    = errors.New("token not valid yet")
)

type header struct {
	Alg string `json:"alg"`
}

type claims struct {
	Exp int64 `json:"exp"`
	Nbf int64 `json:"nbf"`
}

// Verify checks a compact JWS (header.payload.signature) against secret. It
// returns nil only when the algorithm is HS256, the signature is valid, the
// token carries an exp that has not passed, and any present nbf places "now"
// inside the token's validity window.
func Verify(token, secret string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ErrMalformed
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ErrMalformed
	}
	var h header
	if err := json.Unmarshal(headerBytes, &h); err != nil {
		return ErrMalformed
	}
	// Reject anything but HS256 up front — this closes the "alg: none" and
	// algorithm-confusion holes before we touch the signature.
	if h.Alg != "HS256" {
		return ErrAlgorithm
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return ErrMalformed
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return ErrSignature
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ErrMalformed
	}
	var c claims
	if err := json.Unmarshal(payloadBytes, &c); err != nil {
		return ErrMalformed
	}
	now := time.Now().Unix()
	// Require an expiry: a validly-signed token with no exp would never expire
	// and (absent a revocation list) could not be withdrawn short of rotating
	// the shared secret, which drops every client.
	if c.Exp == 0 {
		return ErrNoExpiry
	}
	if now >= c.Exp {
		return ErrExpired
	}
	if c.Nbf != 0 && now < c.Nbf {
		return ErrNotYet
	}
	return nil
}
