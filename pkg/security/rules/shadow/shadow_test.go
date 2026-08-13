// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package shadow

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclcel"
)

// ruleSetWith builds a rule set the way the agent does, from expressions rather than from
// policy files, and returns it loaded.
func ruleSetWith(t *testing.T, exprs map[string]string, macros map[string]string) *rules.RuleSet {
	t.Helper()

	enabled := map[eval.EventType]bool{"*": true}
	ruleOpts, evalOpts := rules.NewBothOpts(enabled)
	ruleSet := rules.NewRuleSet(&model.Model{}, func() eval.Event { return model.NewFakeEvent() },
		ruleOpts, evalOpts)

	def := &rules.PolicyDef{}
	for id, expression := range macros {
		def.Macros = append(def.Macros, &rules.MacroDefinition{ID: id, Expression: expression})
	}
	for id, expression := range exprs {
		def.Rules = append(def.Rules, &rules.RuleDefinition{ID: id, Expression: expression})
	}

	policy, err := rules.LoadPolicyFromDefinition(&rules.PolicyInfo{Name: "test", Source: "test"}, def, nil, nil)
	require.NoError(t, err)

	// Through the loader rather than AddRules, because the shadow reads the policies the
	// rule set holds — that is where a macro's expression is, the compiled form keeping
	// only its AST.
	loader := rules.NewPolicyLoader(&staticProvider{policies: []*rules.Policy{policy}})
	_, errs := ruleSet.LoadPolicies(loader, rules.PolicyLoaderOpts{})
	require.Empty(t, errs.ErrorOrNil())

	return ruleSet
}

// staticProvider serves one policy, which is what the loader wants and a test has.
type staticProvider struct {
	policies []*rules.Policy
}

func (p *staticProvider) LoadPolicies([]rules.MacroFilter, []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	return p.policies, nil
}

func (p *staticProvider) SetOnNewPoliciesReadyCb(func(bool)) {}
func (p *staticProvider) Start()                             {}
func (p *staticProvider) Close() error                       { return nil }
func (p *staticProvider) Type() string                       { return "test" }

func TestShadowAgreesWithSECL(t *testing.T) {
	ruleSet := ruleSetWith(t,
		map[string]string{
			"shell":     `exec.file.name in SHELLS`,
			"etc_write": `open.file.path in [ ~"/etc/*" ] && open.flags & O_WRONLY > 0`,
			"root_exec": `exec.file.path == "/usr/bin/bash" && exec.uid == 0`,
		},
		map[string]string{"SHELLS": `[ "sh", "bash", ~"z*" ]`},
	)

	shadow := New(ruleSet, Config{Enabled: true})
	assert.Empty(t, shadow.Skipped(), "every rule compiles for both engines")
	assert.Equal(t, 3, shadow.Rules())

	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)
	event.Exec.Process = &event.BaseEvent.ProcessContext.Process
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "bash"
	event.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr = "/usr/bin/bash"

	// The rule set has to have matched for the shadow to be measuring the same thing
	// production does, so both engines are exercised on an event that fires a rule.
	assert.True(t, ruleSet.Evaluate(event))

	shadow.Observe(event)
	assert.Empty(t, shadow.Disagreements())
	assert.Equal(t, uint64(1), shadow.stats.evaluations.Load())
	assert.Equal(t, uint64(2), shadow.stats.ruleEvaluations.Load(), "two exec rules")
	assert.Zero(t, shadow.stats.celErrors.Load())
	assert.NotZero(t, shadow.stats.seclDurationNs[0].Load()+shadow.stats.seclDurationNs[1].Load())
	assert.NotZero(t, shadow.stats.celDurationNs[0].Load()+shadow.stats.celDurationNs[1].Load())
}

// TestShadowReportsADisagreement is the counter the whole thing exists for, so it is worth
// knowing it fires: a rule is compiled for one engine and swapped for a translation that
// says something else.
func TestShadowReportsADisagreement(t *testing.T) {
	ruleSet := ruleSetWith(t, map[string]string{"shell": `exec.file.name == "bash"`}, nil)

	shadow := New(ruleSet, Config{Enabled: true})
	require.Equal(t, 1, shadow.Rules())

	// The other engine now answers the opposite, which is what a mistranslation would
	// look like from here.
	bucket := shadow.buckets[model.ExecEventType.String()]
	other, err := seclcel.NewRule(shadow.policy.Env, `exec.file.name != "bash"`, shadow.policy.FieldTypes)
	require.NoError(t, err)
	bucket[0].cel = other
	shadow.buckets[model.ExecEventType.String()] = bucket

	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)
	event.Exec.Process = &event.BaseEvent.ProcessContext.Process
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "bash"

	shadow.Observe(event)
	assert.Equal(t, map[string]uint64{"shell": 1}, shadow.Disagreements())
}

// TestShadowCountsWhatItCannotCompile covers the one gap the agent's own rules do not have
// but SECL allows: a macro that is a boolean expression over an event rather than a value.
// The policy environment evaluates a macro once, at load, with no event — which is what
// makes one free per event — so such a macro cannot be declared, and the rules naming it
// cannot compile.
//
// Both are counted rather than fatal, which is the property that matters: a policy the
// translation does not fully cover still gets measured for the rules it does cover.
func TestShadowCountsWhatItCannotCompile(t *testing.T) {
	ruleSet := ruleSetWith(t,
		map[string]string{
			"fine":        `exec.file.name == "bash"`,
			"unsupported": `IS_SHELL && exec.uid == 0`,
		},
		map[string]string{"IS_SHELL": `exec.file.name == "bash"`},
	)

	shadow := New(ruleSet, Config{Enabled: true})
	assert.Equal(t, 1, shadow.Rules(), "the rule that needs no macro is still measured")
	assert.Equal(t, uint64(1), shadow.stats.rulesSkipped.Load())
	assert.Equal(t, uint64(1), shadow.stats.macrosSkipped.Load())
	assert.Equal(t, map[string]uint64{"undeclared": 1}, shadow.Skipped(),
		"and the reason says which gap it is")
}

func TestReasonOf(t *testing.T) {
	for _, tt := range []struct{ err, reason string }{
		{"cannot be translated: unexpected `[`", "translation"},
		{"ERROR: <input>:1:5: undeclared reference to 'FOO'", "undeclared"},
		{"ERROR: <input>:1:1: found no matching overload for '_==_'", "types"},
		{"planning \"x\": `**` is not allowed in patterns", "planning"},
		{"something else entirely", "other"},
	} {
		t.Run(tt.reason, func(t *testing.T) {
			assert.Equal(t, tt.reason, reasonOf(errors.New(tt.err)))
		})
	}
}

func TestShadowSamples(t *testing.T) {
	ruleSet := ruleSetWith(t, map[string]string{"shell": `exec.file.name == "bash"`}, nil)
	shadow := New(ruleSet, Config{Enabled: true, Rate: 10})

	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)
	event.Exec.Process = &event.BaseEvent.ProcessContext.Process
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "bash"

	for range 100 {
		shadow.Observe(event)
	}
	assert.Equal(t, uint64(10), shadow.stats.evaluations.Load(), "one event in ten")
}

func TestShadowIsOffByDefault(t *testing.T) {
	ruleSet := ruleSetWith(t, map[string]string{"shell": `exec.file.name == "bash"`}, nil)
	shadow := New(ruleSet, Config{})

	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)
	event.Exec.Process = &event.BaseEvent.ProcessContext.Process

	shadow.Observe(event)
	assert.Zero(t, shadow.stats.evaluations.Load())
}

// TestShadowSurvivesAPanic is what keeps a measurement from taking the agent down. The
// generated readers dereference the model the way the accessors do, so an event that leaves
// part of itself unset can panic in either engine.
func TestShadowSurvivesAPanic(t *testing.T) {
	ruleSet := ruleSetWith(t, map[string]string{"shell": `exec.file.name == "bash"`}, nil)
	shadow := New(ruleSet, Config{Enabled: true})

	// An exec event whose process is nil, which the reader dereferences.
	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)

	assert.NotPanics(t, func() { shadow.Observe(event) })
	assert.Equal(t, uint64(1), shadow.stats.panics.Load())
	assert.True(t, shadow.disabled.Load(), "and it stays off")

	// Which is the whole point: a second event costs nothing and cannot panic again.
	shadow.Observe(event)
	assert.Equal(t, uint64(1), shadow.stats.panics.Load())
}

func TestShadowSendsStats(t *testing.T) {
	ruleSet := ruleSetWith(t, map[string]string{"shell": `exec.file.name == "bash"`}, nil)
	shadow := New(ruleSet, Config{Enabled: true})

	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)
	event.Exec.Process = &event.BaseEvent.ProcessContext.Process
	event.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = "bash"
	shadow.Observe(event)

	client := &captureClient{}
	require.NoError(t, shadow.SendStats(client))

	assert.Contains(t, client.gauges, "datadog.runtime_security.rules.cel_shadow.rules")
	assert.Contains(t, client.counts, "datadog.runtime_security.rules.cel_shadow.evaluations")
	assert.Contains(t, client.counts, "datadog.runtime_security.rules.cel_shadow.rule_evaluations")
	assert.Contains(t, client.counts, "datadog.runtime_security.rules.cel_shadow.duration_ns")

	// The counters are flushed, so a second interval with no event reports nothing.
	client = &captureClient{}
	require.NoError(t, shadow.SendStats(client))
	assert.NotContains(t, client.counts, "datadog.runtime_security.rules.cel_shadow.evaluations")
}

type captureClient struct {
	statsd.NoOpClient
	gauges map[string][]float64
	counts map[string][]int64
}

func (c *captureClient) Gauge(name string, value float64, _ []string, _ float64) error {
	if c.gauges == nil {
		c.gauges = map[string][]float64{}
	}
	c.gauges[name] = append(c.gauges[name], value)
	return nil
}

func (c *captureClient) Count(name string, value int64, _ []string, _ float64) error {
	if c.counts == nil {
		c.counts = map[string][]int64{}
	}
	c.counts[name] = append(c.counts[name], value)
	return nil
}
