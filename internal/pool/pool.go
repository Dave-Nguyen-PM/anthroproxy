package pool

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Dave-Nguyen-PM/anthroproxy/internal/config"
)

type TokenState struct {
	Token        config.Token
	CooldownUntil time.Time
	RequestCount  atomic.Int64
	mu            sync.Mutex
}

func (ts *TokenState) InCooldown() bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return time.Now().Before(ts.CooldownUntil)
}

func (ts *TokenState) SetCooldown(d time.Duration) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.CooldownUntil = time.Now().Add(d)
}

func (ts *TokenState) CooldownRemaining() time.Duration {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if time.Now().Before(ts.CooldownUntil) {
		return time.Until(ts.CooldownUntil)
	}
	return 0
}

type Pool struct {
	mu       sync.Mutex
	states   []*TokenState
	cursor   int
	cooldown time.Duration
}

func New(tokens []config.Token, cooldownMinutes int) *Pool {
	states := make([]*TokenState, len(tokens))
	for i, t := range tokens {
		states[i] = &TokenState{Token: t}
	}
	return &Pool{
		states:   states,
		cooldown: time.Duration(cooldownMinutes) * time.Minute,
	}
}

// Next returns the next available token state, skipping those in cooldown.
// Returns nil if all tokens are in cooldown.
func (p *Pool) Next() *TokenState {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(p.states)
	if n == 0 {
		return nil
	}

	for i := 0; i < n; i++ {
		idx := (p.cursor + i) % n
		ts := p.states[idx]
		if !ts.InCooldown() {
			p.cursor = (idx + 1) % n
			return ts
		}
	}
	return nil
}

// NextAfter returns the next available token after skipping the given one.
func (p *Pool) NextAfter(skip *TokenState) *TokenState {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(p.states)
	if n == 0 {
		return nil
	}

	// Find skip index
	skipIdx := -1
	for i, ts := range p.states {
		if ts == skip {
			skipIdx = i
			break
		}
	}

	start := 0
	if skipIdx >= 0 {
		start = (skipIdx + 1) % n
	}

	for i := 0; i < n; i++ {
		idx := (start + i) % n
		ts := p.states[idx]
		if ts == skip {
			continue
		}
		if !ts.InCooldown() {
			p.cursor = (idx + 1) % n
			return ts
		}
	}
	return nil
}

func (p *Pool) MarkCooldown(ts *TokenState) {
	ts.SetCooldown(p.cooldown)
}

func (p *Pool) States() []*TokenState {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]*TokenState, len(p.states))
	copy(cp, p.states)
	return cp
}

func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.states)
}

// ErrNoTokens is returned when no tokens are available.
var ErrNoTokens = fmt.Errorf("no available tokens in pool")
