// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package shadow evaluates a rule set through CEL alongside SECL, to measure what the
// second engine would cost and to report where the two disagree.
//
// It is a measurement, not a feature: nothing it computes reaches a verdict, an action or
// an event. SECL decides, as it does today, and this runs after it on a sample of events.
//
// # What it measures
//
// Per sampled event, the rules of that event type are evaluated by both engines and the
// time each takes is added up. The per-rule cost is derived from the totals rather than
// timed directly: a clock read is around 50 ns, which is a third of what a rule costs, so
// timing each rule would measure the clock. Two reads per engine per event is the whole
// instrumentation.
//
// Which engine goes first alternates, and the figures are tagged with it. The second one to
// run finds the event's caches warm — SECL memoises a resolved array on the context, and the
// path resolver caches — so a fixed order would flatter one of them by exactly the amount
// this is trying to measure.
//
// # What it cannot measure
//
// The figures are per-engine costs on a *warm* event: production has already resolved
// whatever it needed, so what is compared is the two engines rather than the resolvers
// underneath them. And a shadow read can resolve a field production never resolved, which
// caches a value a later handler would otherwise have seen unresolved.
//
// # Safety
//
// A rule that does not compile is counted, not fatal. A panic below Observe disables the
// shadow for the lifetime of the process rather than taking the agent down with it: the
// generated readers dereference the model the way the accessors do, so an event shaped in a
// way no rule reached before can panic in either engine.
package shadow

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclcel"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// Config is what the shadow is turned on with.
type Config struct {
	// Enabled turns the shadow on. It is off by default: it evaluates every rule a
	// second time.
	Enabled bool
	// Rate is the sampling rate, as one event in Rate. Zero and one both mean every
	// event.
	Rate int
}

// Shadow is a rule set compiled for both engines.
type Shadow struct {
	// buckets holds the rules that compiled for both engines, per event type, in the
	// order the rule set holds them.
	buckets map[eval.EventType][]pair
	policy  *seclcel.PolicyEnv
	pool    sync.Pool
	rate    uint64

	seen     atomic.Uint64
	disabled atomic.Bool

	stats stats
}

// pair is one rule, compiled twice.
type pair struct {
	id   string
	secl *eval.Rule
	cel  *seclcel.Rule
}

// New compiles a rule set for the shadow. It never fails: a macro, a variable or a rule
// the translation cannot express is counted and left out, and the shadow measures the rest.
//
// The rules come from the rule set rather than from the policies, so what is compiled is
// the expression the agent compiled — after the `fim.write.*` expansion and after the
// filters — and the two engines are given the same text.
func New(ruleSet *rules.RuleSet, cfg Config) *Shadow {
	shadow := &Shadow{
		buckets: map[eval.EventType][]pair{},
		rate:    max(uint64(cfg.Rate), 1),
	}

	policy, err := seclcel.NewPolicyEnv(seclcel.Policy{
		Macros:    macrosOf(ruleSet),
		Variables: ruleSet.GetVariableStore(),
	})
	if err != nil {
		// The environment is the model's own declarations, so this is a bug rather than
		// something a policy can cause. Nothing is measured, and nothing else breaks.
		shadow.disabled.Store(true)
		shadow.stats.environment.Store(1)
		return shadow
	}
	shadow.policy = policy
	shadow.stats.macrosSkipped.Store(uint64(len(policy.MacroFailures)))
	shadow.stats.variablesSkipped.Store(uint64(len(policy.VariableFailures)))

	for _, rule := range ruleSet.GetRules() {
		eventType, err := rule.GetEventType()
		if err != nil {
			shadow.skip(rule.ID, "no_event_type", err)
			continue
		}

		compiled, err := seclcel.NewRule(policy.Env, rule.Expression, policy.FieldTypes)
		if err != nil {
			shadow.skip(rule.ID, reasonOf(err), err)
			continue
		}

		shadow.buckets[eventType] = append(shadow.buckets[eventType], pair{
			id:   rule.ID,
			secl: rule.Rule,
			cel:  compiled,
		})
		shadow.stats.rulesCompiled.Add(1)
	}

	shadow.pool.New = func() any { return newEvaluation(policy) }
	if !cfg.Enabled {
		shadow.disabled.Store(true)
	}
	return shadow
}

// macrosOf collects the macros the rule set accepted, which is what its own rules were
// compiled against. A macro may appear in several policies, and the rule set merges them by
// ID, so the last one wins here as it does there.
func macrosOf(ruleSet *rules.RuleSet) []seclcel.Macro {
	seen := map[string]int{}
	var macros []seclcel.Macro

	for _, policy := range ruleSet.GetPolicies() {
		for _, macro := range policy.GetAcceptedMacros() {
			declared := seclcel.Macro{ID: macro.Def.ID, Expression: macro.Def.Expression}
			if at, ok := seen[macro.Def.ID]; ok {
				macros[at] = declared
				continue
			}
			seen[macro.Def.ID] = len(macros)
			macros = append(macros, declared)
		}
	}
	return macros
}

// Observe evaluates the event through both engines, on a sample of events.
//
// It is called after the rule set has been evaluated and after a match has been reported,
// so a rule that fires is already on its way out when this runs — see the note on
// APIServer.SendEvent — and nothing here can delay or alter it.
func (s *Shadow) Observe(event *model.Event) {
	if s.disabled.Load() {
		return
	}
	if s.seen.Add(1)%s.rate != 0 {
		return
	}

	bucket := s.buckets[event.GetType()]
	if len(bucket) == 0 {
		return
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			// One panic is enough to say the shadow cannot be trusted on this agent's
			// events, and leaving it on would repeat it on every one of them.
			s.disabled.Store(true)
			s.stats.panics.Add(1)
		}
	}()

	evaluation := s.pool.Get().(*evaluation)
	defer func() {
		evaluation.reset()
		s.pool.Put(evaluation)
	}()
	evaluation.start(event, len(bucket))

	// The order alternates, so that neither engine is always the one paying for the
	// event's cold caches.
	celFirst := s.seen.Load()%(2*s.rate) == 0
	if celFirst {
		evaluation.evalCEL(bucket)
		evaluation.evalSECL(bucket)
	} else {
		evaluation.evalSECL(bucket)
		evaluation.evalCEL(bucket)
	}

	s.stats.record(bucket, evaluation, celFirst)
}

// evaluation is what one observation needs: a context per engine, and the verdicts.
//
// Each engine gets its own context because they share what a context caches. SECL memoises
// a resolved array on it, so the engine that ran second would read one that the first
// filled — which is the difference this is measuring.
//
// The CEL activation is bound to its context, and both are held for as long as this
// evaluation is pooled: building an activation per event would add its own cost to the
// figures, and the contexts are reset rather than replaced.
type evaluation struct {
	seclCtx *eval.Context

	celCtx     *eval.Context
	activation *seclcel.Activation

	seclVerdicts []bool
	celVerdicts  []bool
	celErrors    int

	seclDuration time.Duration
	celDuration  time.Duration
}

func newEvaluation(policy *seclcel.PolicyEnv) *evaluation {
	celCtx := eval.NewContext(nil)
	return &evaluation{
		seclCtx:    eval.NewContext(nil),
		celCtx:     celCtx,
		activation: policy.Activation(celCtx),
	}
}

func (e *evaluation) start(event *model.Event, rules int) {
	e.seclCtx.SetEvent(event)
	e.celCtx.SetEvent(event)
	e.seclVerdicts = slicesGrow(e.seclVerdicts, rules)
	e.celVerdicts = slicesGrow(e.celVerdicts, rules)
	e.celErrors = 0
}

func (e *evaluation) reset() {
	e.seclCtx.Reset()
	e.celCtx.Reset()
	e.seclVerdicts = e.seclVerdicts[:0]
	e.celVerdicts = e.celVerdicts[:0]
}

// evalSECL evaluates the bucket the way RuleSet.Evaluate does, minus the actions and the
// reporting: one context for the bucket, reset between rules.
func (e *evaluation) evalSECL(bucket []pair) {
	start := time.Now()
	for _, rule := range bucket {
		e.seclVerdicts = append(e.seclVerdicts, rule.secl.GetEvaluator().Eval(e.seclCtx))
		e.seclCtx.PerEvalReset()
	}
	e.seclDuration = time.Since(start)
}

func (e *evaluation) evalCEL(bucket []pair) {
	start := time.Now()
	for _, rule := range bucket {
		matched, err := rule.cel.Eval(e.activation)
		if err != nil {
			e.celErrors++
		}
		e.celVerdicts = append(e.celVerdicts, matched)
		e.celCtx.PerEvalReset()
	}
	e.celDuration = time.Since(start)
}

// SendStats flushes what has been measured. It is called from RuleEngine.SendStats, on the
// agent's usual statsd interval.
func (s *Shadow) SendStats(client statsd.ClientInterface) error {
	return s.stats.send(client, s.Rules())
}

// Rules reports how many rules the shadow evaluates, which is what says how much of the
// rule set the measurement covers.
func (s *Shadow) Rules() int {
	var count int
	for _, bucket := range s.buckets {
		count += len(bucket)
	}
	return count
}

// Disagreements returns the rules the two engines answered differently for, most recent
// count first. It is what a report or a test reads; the metrics carry the same numbers.
func (s *Shadow) Disagreements() map[string]uint64 {
	return s.stats.snapshot(&s.stats.disagreements)
}

// Skipped returns the rules the shadow could not compile, by reason.
func (s *Shadow) Skipped() map[string]uint64 {
	return s.stats.snapshot(&s.stats.skipped)
}

func (s *Shadow) skip(id, reason string, err error) {
	s.stats.count(&s.stats.skipped, reason)
	s.stats.rulesSkipped.Add(1)
	seclog.Debugf("cel shadow: rule %s is not evaluated (%s): %v", id, reason, err)
}

// reasonOf classifies why a rule could not be compiled, so that the metric says which gap
// to close rather than only how many rules are in it.
func reasonOf(err error) string {
	message := err.Error()
	switch {
	case containsAny(message, "cannot be translated", "unexpected", "unsupported"):
		return "translation"
	case containsAny(message, "undeclared reference"):
		return "undeclared"
	case containsAny(message, "no matching overload", "does not support field selection", "undefined field"):
		return "types"
	case containsAny(message, "planning"):
		return "planning"
	}
	return "other"
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func slicesGrow(values []bool, size int) []bool {
	if cap(values) < size {
		return make([]bool, 0, size)
	}
	return values[:0]
}

// stats is everything the shadow reports, kept in atomics so that an observation costs no
// locking, and in maps only where a lock is taken on a path that should be rare.
type stats struct {
	rulesCompiled    atomic.Uint64
	rulesSkipped     atomic.Uint64
	macrosSkipped    atomic.Uint64
	variablesSkipped atomic.Uint64
	environment      atomic.Uint64
	panics           atomic.Uint64

	evaluations     atomic.Uint64
	ruleEvaluations atomic.Uint64
	celErrors       atomic.Uint64

	seclDurationNs [2]atomic.Uint64
	celDurationNs  [2]atomic.Uint64

	mu            sync.Mutex
	disagreements map[string]uint64
	skipped       map[string]uint64
}

func (s *stats) record(bucket []pair, e *evaluation, celFirst bool) {
	order := 0
	if celFirst {
		order = 1
	}

	s.evaluations.Add(1)
	s.ruleEvaluations.Add(uint64(len(e.seclVerdicts)))
	s.seclDurationNs[order].Add(uint64(e.seclDuration))
	s.celDurationNs[order].Add(uint64(e.celDuration))
	s.celErrors.Add(uint64(e.celErrors))

	// The verdicts are what the whole thing is for. They are compared per rule, and a
	// disagreement is named: the count alone would say something is wrong without saying
	// which rule to look at, and the shape of the disagreements is what says whether the
	// translation is at fault — a rule reading builtins.uuid4 disagrees with itself in
	// either engine, for instance.
	for i, verdict := range e.seclVerdicts {
		if i < len(e.celVerdicts) && verdict != e.celVerdicts[i] {
			s.count(&s.disagreements, bucket[i].id)
		}
	}
}

func (s *stats) count(into *map[string]uint64, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if *into == nil {
		*into = map[string]uint64{}
	}
	(*into)[key]++
}

func (s *stats) snapshot(from *map[string]uint64) map[string]uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make(map[string]uint64, len(*from))
	for key, value := range *from {
		out[key] = value
	}
	return out
}

func (s *stats) send(client statsd.ClientInterface, compiled int) error {
	var errs error

	gauge := func(name string, value float64, tags ...string) {
		if err := client.Gauge(name, value, tags, 1.0); err != nil {
			errs = err
		}
	}
	count := func(name string, value uint64, tags ...string) {
		if value == 0 {
			return
		}
		if err := client.Count(name, int64(value), tags, 1.0); err != nil {
			errs = err
		}
	}

	// The rule set's state is a gauge: it says what is being measured, and it does not
	// accumulate.
	gauge(metrics.MetricCELShadowRules, float64(compiled), "state:compiled")
	gauge(metrics.MetricCELShadowRules, float64(s.rulesSkipped.Load()), "state:skipped")
	gauge(metrics.MetricCELShadowRules, float64(s.macrosSkipped.Load()), "state:macro_skipped")
	gauge(metrics.MetricCELShadowRules, float64(s.variablesSkipped.Load()), "state:variable_skipped")

	s.mu.Lock()
	for reason, value := range s.skipped {
		gauge(metrics.MetricCELShadowRules, float64(value), "state:skipped", "reason:"+reason)
	}
	disagreements := s.disagreements
	s.disagreements = nil
	s.mu.Unlock()

	count(metrics.MetricCELShadowEvaluations, s.evaluations.Swap(0))
	count(metrics.MetricCELShadowRuleEvaluations, s.ruleEvaluations.Swap(0))
	count(metrics.MetricCELShadowErrors, s.celErrors.Swap(0), "engine:cel")
	count(metrics.MetricCELShadowErrors, s.panics.Swap(0), "engine:cel", "kind:panic")
	count(metrics.MetricCELShadowErrors, s.environment.Swap(0), "engine:cel", "kind:environment")

	for order, tag := range []string{"first:secl", "first:cel"} {
		count(metrics.MetricCELShadowDuration, s.seclDurationNs[order].Swap(0), "engine:secl", tag)
		count(metrics.MetricCELShadowDuration, s.celDurationNs[order].Swap(0), "engine:cel", tag)
	}

	for rule, value := range disagreements {
		count(metrics.MetricCELShadowDisagreements, value, "rule_id:"+rule)
	}

	if errs != nil {
		return fmt.Errorf("sending the cel shadow metrics: %w", errs)
	}
	return nil
}
