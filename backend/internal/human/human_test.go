package human

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/thdelmas/e-agora/backend/internal/token"
)

const secret = "human-secret"

func newChecker(t *testing.T) *Checker {
	t.Helper()
	c, err := New(secret, time.Minute)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// passOf decodes the signed envelope (white-box) to learn the pass option.
func passOf(t *testing.T, ch Challenge) string {
	t.Helper()
	payload, err := token.Open(secret, ch.ChallengeID)
	if err != nil {
		t.Fatalf("open challenge: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return env.Pass
}

func TestNewChallenge_Shape(t *testing.T) {
	ch, err := newChecker(t).NewChallenge()
	if err != nil {
		t.Fatalf("NewChallenge: %v", err)
	}
	if ch.Prompt == "" || ch.ChallengeID == "" {
		t.Error("challenge missing prompt or id")
	}
	if len(ch.Options) != 2 {
		t.Fatalf("want 2 options, got %d", len(ch.Options))
	}
	if ch.Kind != "oath" && ch.Kind != "control" {
		t.Errorf("unexpected kind %q", ch.Kind)
	}
}

func TestVerify_DissentPasses_AffirmFails(t *testing.T) {
	c := newChecker(t)
	ch, _ := c.NewChallenge()
	pass := passOf(t, ch)

	if ok, reason := c.Verify(ch.ChallengeID, pass, Timing{DecideMs: 1500}); !ok {
		t.Errorf("correct answer should pass, reason=%q", reason)
	}

	wrong := "yes"
	if pass == "yes" {
		wrong = "no"
	}
	if ok, reason := c.Verify(ch.ChallengeID, wrong, Timing{DecideMs: 1500}); ok || reason != "try_again" {
		t.Errorf("wrong answer: ok=%v reason=%q, want try_again", ok, reason)
	}
}

func TestVerify_Timing(t *testing.T) {
	c := newChecker(t)
	ch, _ := c.NewChallenge()
	pass := passOf(t, ch)

	// Instant click is rejected even with the right answer.
	if ok, reason := c.Verify(ch.ChallengeID, pass, Timing{Instant: true}); ok || reason != "too_fast" {
		t.Errorf("instant: ok=%v reason=%q, want too_fast", ok, reason)
	}
	// Implausibly fast is rejected.
	if ok, _ := c.Verify(ch.ChallengeID, pass, Timing{DecideMs: 200}); ok {
		t.Error("200ms should be rejected")
	}
	// Missing timing must NOT block (accessibility).
	if ok, reason := c.Verify(ch.ChallengeID, pass, Timing{}); !ok {
		t.Errorf("missing timing must pass, reason=%q", reason)
	}
}

func TestVerify_Invalid(t *testing.T) {
	c := newChecker(t)
	if ok, reason := c.Verify("garbage", "no", Timing{DecideMs: 1000}); ok || reason != "invalid" {
		t.Errorf("garbage challenge: ok=%v reason=%q, want invalid", ok, reason)
	}
}

func TestVerify_Expired(t *testing.T) {
	c := newChecker(t)
	// Hand-build an expired, validly-signed challenge (white-box).
	payload, _ := json.Marshal(envelope{Kind: "oath", Pass: "no", Nonce: "n", Exp: time.Now().Add(-time.Minute).Unix()})
	id := token.Sign(secret, payload)
	if ok, reason := c.Verify(id, "no", Timing{DecideMs: 1000}); ok || reason != "expired" {
		t.Errorf("expired: ok=%v reason=%q, want expired", ok, reason)
	}
}
