package token

import (
	"testing"
	"time"
)

const secret = "test-secret"

func TestMintVerify_RoundTrip(t *testing.T) {
	tok, exp, err := Mint(secret, time.Hour)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if time.Until(exp) <= 0 {
		t.Errorf("exp should be in the future, got %v", exp)
	}
	c, err := Verify(secret, tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c.Iss != "e-agora" || c.Jti == "" || c.Exp == 0 {
		t.Errorf("claims incomplete: %+v", c)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	tok, _, _ := Mint(secret, time.Hour)
	if _, err := Verify("other-secret", tok); err != ErrInvalid {
		t.Errorf("wrong secret: err = %v, want ErrInvalid", err)
	}
}

func TestVerify_Tampered(t *testing.T) {
	tok, _, _ := Mint(secret, time.Hour)
	// Flip the last byte of the payload body.
	b := []byte(tok)
	b[0] ^= 0xff
	if _, err := Verify(secret, string(b)); err != ErrInvalid {
		t.Errorf("tampered: err = %v, want ErrInvalid", err)
	}
}

func TestVerify_Expired(t *testing.T) {
	tok, _, _ := Mint(secret, -time.Minute) // already expired
	if _, err := Verify(secret, tok); err != ErrExpired {
		t.Errorf("expired: err = %v, want ErrExpired", err)
	}
}

func TestOpen_Malformed(t *testing.T) {
	for _, s := range []string{"", "nodot", ".", "abc.", ".abc"} {
		if _, err := Open(secret, s); err != ErrInvalid {
			t.Errorf("Open(%q) err = %v, want ErrInvalid", s, err)
		}
	}
}

func TestJtiIsUnique(t *testing.T) {
	a, _, _ := Mint(secret, time.Hour)
	b, _, _ := Mint(secret, time.Hour)
	if a == b {
		t.Error("two mints produced identical tokens (jti not random?)")
	}
}
