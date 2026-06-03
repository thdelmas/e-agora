package human

import (
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	"github.com/thdelmas/e-agora/backend/data"
	"github.com/thdelmas/e-agora/backend/internal/token"
)

// controlEvery: roughly 1 in N challenges is a sincere "control" statement
// (pass = agree) so an "always dissent" bot also fails
// (docs/01-functional-spec.md S4).
const controlEvery = 4

// Pool is the rotating prompt set (data/humanity_prompts.json).
type Pool struct {
	Oaths    []string `json:"oaths"`
	Controls []string `json:"controls"`
}

// Checker mints and verifies dissent-based humanity challenges (R12).
// Stateless: the challenge is a signed envelope, so no challenge table is
// needed.
type Checker struct {
	secret       string
	prompts      Pool
	challengeTTL time.Duration
}

// Option is a click choice shown to the visitor (the pass option is never
// marked).
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Challenge is the public, client-facing challenge.
type Challenge struct {
	ChallengeID string   `json:"challengeId"`
	Prompt      string   `json:"prompt"`
	Kind        string   `json:"kind"` // "oath" | "control"
	Options     []Option `json:"options"`
}

// Timing is the soft, client-collected interaction summary (no keystrokes).
type Timing struct {
	DecideMs     int  `json:"decideMs"`
	Instant      bool `json:"instant"`
	PointerMoves int  `json:"pointerMoves"`
}

// envelope is the signed, server-only challenge state (opaque to the client).
type envelope struct {
	Kind  string `json:"k"`
	Pass  string `json:"p"` // pass option id
	Nonce string `json:"n"`
	Exp   int64  `json:"e"`
}

// New loads the prompt pool and returns a Checker. challengeTTL bounds how
// long a challenge may be solved (short); the 24h human-verified window is
// applied by the caller on success.
func New(secret string, challengeTTL time.Duration) (*Checker, error) {
	pool, err := loadPool()
	if err != nil {
		return nil, err
	}
	if len(pool.Oaths) == 0 {
		return nil, errors.New("human: prompt pool has no oaths")
	}
	return &Checker{
		secret: secret, prompts: pool, challengeTTL: challengeTTL,
	}, nil
}

// NewChallenge builds a fresh, signed challenge with randomized option order.
func (c *Checker) NewChallenge() (Challenge, error) {
	var kind, prompt, pass string
	var opts []Option
	if len(c.prompts.Controls) > 0 && randIntn(controlEvery) == 0 {
		kind, pass = "control", "yes"
		prompt = c.prompts.Controls[randIntn(len(c.prompts.Controls))]
		opts = []Option{{"yes", "Agreed"}, {"no", "I disagree"}}
	} else {
		kind, pass = "oath", "no"
		prompt = c.prompts.Oaths[randIntn(len(c.prompts.Oaths))]
		opts = []Option{{"yes", "I swear it"}, {"no", "I won't swear to that"}}
	}
	shuffle(opts)

	nonce, err := token.NewID()
	if err != nil {
		return Challenge{}, err
	}
	payload, err := json.Marshal(envelope{
		Kind: kind, Pass: pass, Nonce: nonce,
		Exp: time.Now().Add(c.challengeTTL).Unix(),
	})
	if err != nil {
		return Challenge{}, err
	}
	return Challenge{
		ChallengeID: token.Sign(c.secret, payload),
		Prompt:      prompt,
		Kind:        kind,
		Options:     opts,
	}, nil
}

// Verify checks a submitted answer. It returns whether the session should
// become human-verified and a machine reason on failure. The timing signal is
// SOFT: it only rejects on positive evidence of an instant/scripted click —
// missing timing never blocks (accessibility, R12).
func (c *Checker) Verify(
	challengeID, answer string, t Timing,
) (ok bool, reason string) {
	payload, err := token.Open(c.secret, challengeID)
	if err != nil {
		return false, "invalid"
	}
	var env envelope
	if json.Unmarshal(payload, &env) != nil {
		return false, "invalid"
	}
	if time.Now().Unix() >= env.Exp {
		return false, "expired"
	}
	if answer != env.Pass {
		return false, "try_again" // wrong choice (swore the oath / refused a control)
	}
	if t.Instant || (t.DecideMs > 0 && t.DecideMs < 400) {
		return false, "too_fast" // scripted-looking; a human just retries slower
	}
	return true, ""
}

func loadPool() (Pool, error) {
	b, err := data.FS.ReadFile("humanity_prompts.json")
	if err != nil {
		return Pool{}, err
	}
	var p Pool
	if err := json.Unmarshal(b, &p); err != nil {
		return Pool{}, err
	}
	return p, nil
}

// randIntn returns a crypto-random int in [0,n). n<=1 yields 0.
func randIntn(n int) int {
	if n <= 1 {
		return 0
	}
	var b [8]byte
	_, _ = crand.Read(b[:])
	return int(binary.BigEndian.Uint64(b[:]) % uint64(n))
}

func shuffle(opts []Option) {
	for i := len(opts) - 1; i > 0; i-- {
		j := randIntn(i + 1)
		opts[i], opts[j] = opts[j], opts[i]
	}
}
