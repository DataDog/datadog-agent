// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
//
//nolint:revive
package automultilinedetection

import (
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type SampleAggregator struct {
	tokenizer *Tokenizer
	sampler   *Sampler
}

type TokenizedMessage struct {
	message *message.Message
	tokens  []tokens.Token
}

func NewSampleAggregator() *SampleAggregator {

	s := &SampleAggregator{
		tokenizer: NewTokenizer(1024 * 10),
		sampler:   NewSampler(DefaultConfig()),
	}
	s.sampler.Start()
	return s
}

func (s *SampleAggregator) Process(msg *message.Message) *message.Message {

	// Reject JSON logs
	if jsonRegexp.Match(msg.GetContent()) {
		return msg
	}

	tokens, _ := s.tokenizer.tokenize(msg.GetContent())
	tokenizedMessage := &TokenizedMessage{
		message: msg,
		tokens:  tokens,
	}

	emit, message := s.sampler.Process(tokenizedMessage)

	if emit {
		fmt.Printf("EMIT thr=%v suppr=%4d fast=%5.2f share=%5.2f%% rare=%v burst=%4.1f\n",
			emit, message.SuppressedSinceLastEmit, message.FastRate, message.SlowShare*100, message.IsRare, message.BurstCreditRemaining)

		msg := message.TokenizedMessage.message
		if message.SuppressedSinceLastEmit > 0 {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, fmt.Sprintf("throttled_count:%d", message.SuppressedSinceLastEmit))
		}
		return msg
	}

	return nil
}

/// ------------------------------ Sampler -------------------------------- //

type Config struct {
	// Ceiling for common types: at most this many events per second per type.
	RateCeiling float64 // e.g., 1.0

	// EWMA half-lives (responsiveness).
	FastHalfLife time.Duration // e.g., 2s
	SlowHalfLife time.Duration // e.g., 60s

	// Rare classification by slow share: slow_rate[type]/global_slow_rate.
	RareEnterShare    float64 // e.g., 0.005 (0.5%)
	RareExitShare     float64 // e.g., 0.01 (1.0%)
	HysteresisSeconds int     // consecutive seconds beyond threshold to flip

	// Per-type burst credit (extra above ceiling) while rare.
	RareBurstRefill float64 // tokens/sec, e.g., 0.2
	RareBurstCap    float64 // e.g., 20

	// Per-type ceiling token bucket capacity (lets 1–2 accumulate).
	CeilingCap float64 // e.g., 2

	// Global shared pool for rare bursts (extra above ceiling across ALL rare types).
	GlobalRareBurstRefill float64 // tokens/sec, e.g., 50.0
	GlobalRareBurstCap    float64 // e.g., 500.0

	// Bootstrap burst for brand-new types (they start rare).
	NewTypeBurstBootstrap float64 // e.g., 2.0

	// Evict idle types after this duration with no traffic.
	EvictAfter time.Duration // e.g., 3 * time.Minute
}

func DefaultConfig() Config {
	return Config{
		RateCeiling:           1.0,
		FastHalfLife:          2 * time.Second,
		SlowHalfLife:          60 * time.Second,
		RareEnterShare:        0.005,
		RareExitShare:         0.01,
		HysteresisSeconds:     5,
		RareBurstRefill:       0.5,
		RareBurstCap:          100,
		CeilingCap:            5,
		GlobalRareBurstRefill: 100.0,
		GlobalRareBurstCap:    1000.0,
		NewTypeBurstBootstrap: 2.0,
		EvictAfter:            3 * time.Minute,
	}
}

type EventMeta struct {
	TokenizedMessage        *TokenizedMessage
	Throttled               bool
	SuppressedSinceLastEmit int
	FastRate                float64
	SlowShare               float64
	IsRare                  bool
	BurstCreditRemaining    float64
}

// per-type state
type typeState struct {
	tokenizedMessage *TokenizedMessage

	// meters
	secondCount int     // raw events counted this second
	fastEWMA    float64 // events/sec
	slowEWMA    float64 // events/sec

	// tokens
	ceilingTokens float64 // per-type ceiling
	burstTokens   float64 // per-type rare burst

	// pacing/meta
	suppressedSinceLastEmit int
	isRare                  bool
	rareEnterStreak         int
	rareExitStreak          int

	lastSeen time.Time
}

type Sampler struct {
	cfg Config

	mu    sync.Mutex
	table []*typeState

	// global meters
	globalSecondCount int
	globalSlowEWMA    float64

	// global rare burst pool (shared across all rare types)
	globalRareBurst float64

	// new-type tracking
	newTypeSeenThisMinute int
	minuteWindowStart     time.Time

	// precomputed alphas for 1s ticks
	alphaFast float64
	alphaSlow float64

	// lifecycle
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewSampler(cfg Config) *Sampler {
	s := &Sampler{
		cfg:                   cfg,
		table:                 make([]*typeState, 0, 100),
		stopCh:                make(chan struct{}),
		minuteWindowStart:     time.Now(),
		globalRareBurst:       0,
		globalSlowEWMA:        0,
		newTypeSeenThisMinute: 0,
	}
	s.alphaFast = ewmaAlpha(cfg.FastHalfLife, time.Second)
	s.alphaSlow = ewmaAlpha(cfg.SlowHalfLife, time.Second)
	return s
}

func ewmaAlpha(halfLife, dt time.Duration) float64 {
	if halfLife <= 0 {
		return 1.0
	}
	return 1 - math.Exp(-float64(dt)*math.Ln2/float64(halfLife))
}

func (s *Sampler) Start() {
	s.wg.Add(1)
	go s.backgroundTick()
}

func (s *Sampler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// get typestate - returns typestate and index
func (s *Sampler) getTypeState(tokenizedMessage *TokenizedMessage) (*typeState, int) {
	for i, st := range s.table {
		if isMatch(st.tokenizedMessage.tokens, tokenizedMessage.tokens, 0.90) {
			return st, i
		}
	}
	return nil, -1
}

// moveToFront moves the typeState at the given index to the front of the table.
// Most recently seen items are kept at index 0.
func (s *Sampler) moveToFront(idx int) {
	if idx == 0 {
		return
	}
	st := s.table[idx]
	s.table = slices.Delete(s.table, idx, idx+1)
	s.table = slices.Insert(s.table, 0, st)
}

// Process decides whether to emit now or suppress (pacing).
// Returns (emit?, meta). If emit=false, the event is suppressed for now.
func (s *Sampler) Process(tokenizedMessage *TokenizedMessage) (bool, EventMeta) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	st, idx := s.getTypeState(tokenizedMessage)
	if st == nil {
		// new type accounting window
		if now.Sub(s.minuteWindowStart) >= time.Minute {
			s.minuteWindowStart = now
			s.newTypeSeenThisMinute = 0
		}
		s.newTypeSeenThisMinute++

		// NEW == RARE, with bootstrap burst
		st = &typeState{
			tokenizedMessage: tokenizedMessage,
			ceilingTokens:    0,
			burstTokens:      math.Min(s.cfg.RareBurstCap, s.cfg.NewTypeBurstBootstrap),
			isRare:           true,
			lastSeen:         now,
		}
		// New items go directly to front (most recent)
		s.table = slices.Insert(s.table, 0, st)
		idx = 0
	}

	// book-keeping
	st.secondCount++
	st.lastSeen = now
	s.globalSecondCount++

	share := s.currentShareLocked(st)
	meta := EventMeta{
		TokenizedMessage:        tokenizedMessage,
		Throttled:               false,
		SuppressedSinceLastEmit: st.suppressedSinceLastEmit,
		FastRate:                st.fastEWMA,
		SlowShare:               share,
		IsRare:                  st.isRare,
		BurstCreditRemaining:    st.burstTokens,
	}

	// Move to front for fastest future lookups (only if not already there)
	if idx > 0 {
		s.moveToFront(idx)
	}

	emit, throttled := s.decideEmitLocked(st, st.isRare)
	if emit {
		meta.Throttled = throttled
		if throttled {
			meta.SuppressedSinceLastEmit = st.suppressedSinceLastEmit
			st.suppressedSinceLastEmit = 0
		} else {
			meta.SuppressedSinceLastEmit = 0
		}
		return true, meta
	}

	// suppress for now; include in next emitted event's count
	st.suppressedSinceLastEmit++
	meta.Throttled = true
	return false, meta
}

// decision logic:
// - If under ceiling by fast EWMA -> emit (not throttled).
// - If over ceiling and rare -> try global rare pool (then per-type burst), emit throttled if available.
// - Else use per-type ceiling tokens.
// - Else suppress.
func (s *Sampler) decideEmitLocked(st *typeState, isRare bool) (bool, bool) {
	// Under ceiling: let it through without spending tokens.
	if st.fastEWMA <= s.cfg.RateCeiling {
		return true, false
	}

	// Over ceiling: rare override path first
	if isRare {
		// Prefer global rare burst pool
		if s.globalRareBurst >= 1.0 {
			s.globalRareBurst -= 1.0
			// Optionally also spend per-type burst for fairness (kept here, but not required)
			if st.burstTokens >= 1.0 {
				st.burstTokens -= 1.0
			}
			return true, true
		}
		// Fall back to per-type rare burst
		if st.burstTokens >= 1.0 {
			st.burstTokens -= 1.0
			return true, true
		}
	}

	// Non-rare (or rare with no global/per-type burst): per-type ceiling tokens
	if st.ceilingTokens >= 1.0 {
		st.ceilingTokens -= 1.0
		return true, true
	}

	// Otherwise: suppress for now
	return false, true
}

// background tick: 1s cadence to update meters, refill tokens, classify, evict
func (s *Sampler) backgroundTick() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.mu.Lock()

			// update global slow EWMA from last second's volume
			globalRate := float64(s.globalSecondCount)
			s.globalSlowEWMA = s.globalSlowEWMA + s.alphaSlow*(globalRate-s.globalSlowEWMA)
			s.globalSecondCount = 0

			// refill global rare burst pool
			s.globalRareBurst = math.Min(
				s.cfg.GlobalRareBurstCap,
				s.globalRareBurst+s.cfg.GlobalRareBurstRefill,
			)

			// per-type updates - iterate backwards to safely delete
			for i := len(s.table) - 1; i >= 0; i-- {
				st := s.table[i]

				// evict idle types first (before updates)
				if now.Sub(st.lastSeen) > s.cfg.EvictAfter {
					s.table = slices.Delete(s.table, i, i+1)
					continue
				}

				// update EWMAs from per-second count
				rate := float64(st.secondCount)
				st.fastEWMA = st.fastEWMA + s.alphaFast*(rate-st.fastEWMA)
				st.slowEWMA = st.slowEWMA + s.alphaSlow*(rate-st.slowEWMA)
				st.secondCount = 0

				// refill per-type ceiling tokens
				st.ceilingTokens = math.Min(s.cfg.CeilingCap, st.ceilingTokens+s.cfg.RateCeiling)

				// per-type rare burst refill/bleed
				if st.isRare {
					st.burstTokens = math.Min(s.cfg.RareBurstCap, st.burstTokens+s.cfg.RareBurstRefill)
				} else {
					// faster bleed so a type exiting rare can't carry large burst
					st.burstTokens = math.Max(0, st.burstTokens-1.0)
				}

				// rarity classification with hysteresis on slow share
				share := s.currentShareLocked(st)
				if share <= s.cfg.RareEnterShare {
					st.rareEnterStreak++
					st.rareExitStreak = 0
				} else if share >= s.cfg.RareExitShare {
					st.rareExitStreak++
					st.rareEnterStreak = 0
				} else {
					// within deadband: decay streaks slowly
					if st.rareEnterStreak > 0 {
						st.rareEnterStreak--
					}
					if st.rareExitStreak > 0 {
						st.rareExitStreak--
					}
				}

				if !st.isRare && st.rareEnterStreak >= s.cfg.HysteresisSeconds {
					st.isRare = true
					// small kickstart for visibility
					st.burstTokens = math.Min(s.cfg.RareBurstCap, st.burstTokens+1.0)
				}
				if st.isRare && st.rareExitStreak >= s.cfg.HysteresisSeconds {
					st.isRare = false
					if st.burstTokens > 2.0 {
						st.burstTokens = 2.0
					}
				}
			}

			s.mu.Unlock()
		}
	}
}

func (s *Sampler) currentShareLocked(st *typeState) float64 {
	if s.globalSlowEWMA <= 0 {
		return 0
	}
	return st.slowEWMA / s.globalSlowEWMA
}
