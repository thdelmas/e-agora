package token

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	// ErrInvalid means the signature or format is wrong (forged/tampered).
	ErrInvalid = errors.New("token: invalid signature or format")
	// ErrExpired means the signature is valid but the token is past its exp.
	ErrExpired = errors.New("token: expired")
)

// Sign returns a compact signed blob: base64url(payload).base64url(HMAC). The
// payload is opaque to Sign — callers (access token, humanity challenge)
// define their own JSON. The HMAC is computed over the base64 body.
func Sign(secret string, payload []byte) string {
	body := b64(payload)
	return body + "." + b64(mac(secret, body))
}

// Open verifies the HMAC (constant-time) and returns the payload bytes. It does
// not interpret the payload or check expiry — that is the caller's job.
func Open(secret, signed string) ([]byte, error) {
	dot := strings.IndexByte(signed, '.')
	if dot <= 0 || dot == len(signed)-1 {
		return nil, ErrInvalid
	}
	body, sig := signed[:dot], signed[dot+1:]
	if !hmac.Equal([]byte(sig), []byte(b64(mac(secret, body)))) {
		return nil, ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, ErrInvalid
	}
	return payload, nil
}

// Claims is the access-token payload (R10). It carries NO subject/session
// id — anonymity by design; jti is a random nonce.
type Claims struct {
	Iss string `json:"iss"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
	Jti string `json:"jti"`
}

// Mint issues a fixed-TTL access token and returns it plus its expiry.
func Mint(
	secret string, ttl time.Duration,
) (tok string, exp time.Time, err error) {
	jti, err := NewID()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	exp = now.Add(ttl)
	payload, err := json.Marshal(Claims{
		Iss: "e-agora", Iat: now.Unix(), Exp: exp.Unix(), Jti: jti,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return Sign(secret, payload), exp, nil
}

// Verify checks the signature and that the token has not expired.
func Verify(secret, tok string) (Claims, error) {
	payload, err := Open(secret, tok)
	if err != nil {
		return Claims{}, err
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, ErrInvalid
	}
	if time.Now().Unix() >= c.Exp {
		return Claims{}, ErrExpired
	}
	return c, nil
}

// NewID returns 32 hex chars of cryptographic randomness — used for jti and
// for anonymous session ids.
func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func mac(secret, body string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(body))
	return h.Sum(nil)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
