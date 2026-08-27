// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package darwin

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// NewRuleSet loads the policies in policiesDir into a RuleSet bound to the
// darwin model.
func NewRuleSet(policiesDir string, eventCtor func() eval.Event) (*rules.RuleSet, error) {
	opts := (&rules.Opts{}).WithEventTypeEnabled(SupportedEventTypes())

	rs := rules.NewRuleSet(NewDarwinModel(), eventCtor, opts, rules.NewEvalOpts())

	provider, err := rules.NewPoliciesDirProvider(policiesDir)
	if err != nil {
		return nil, fmt.Errorf("policy provider: %w", err)
	}

	loader := rules.NewPolicyLoader(provider)
	if _, merr := rs.LoadPolicies(loader, rules.PolicyLoaderOpts{}); merr.ErrorOrNil() != nil {
		return nil, fmt.Errorf("load policies: %w", merr.ErrorOrNil())
	}

	// A ruleset that loaded no rules would sit there consuming events and never
	// firing, which is indistinguishable from "nothing suspicious happened".
	if len(rs.GetRules()) == 0 {
		return nil, fmt.Errorf("no rules loaded from %s", policiesDir)
	}

	return rs, nil
}

// Match is a rule that fired, with the event that triggered it.
type Match struct {
	RuleID string
	Rule   *rules.Rule
	Event  *model.Event
}

// MatchRecorder collects rule matches. Used by both the tests and the collector.
type MatchRecorder struct {
	matches []Match
}

var _ rules.RuleSetListener = (*MatchRecorder)(nil)

// RuleMatch records a match. Returning true keeps the event flowing.
func (r *MatchRecorder) RuleMatch(_ *eval.Context, rule *rules.Rule, event eval.Event) bool {
	ev, ok := event.(*model.Event)
	if !ok {
		return true
	}
	r.matches = append(r.matches, Match{RuleID: rule.ID, Rule: rule, Event: ev})
	return true
}

// EventDiscarderFound is a no-op: darwin generates no discarders, because
// Endpoint Security has no in-kernel filtering for us to program.
func (r *MatchRecorder) EventDiscarderFound(_ *rules.RuleSet, _ eval.Event, _ eval.Field, _ eval.EventType) {
}

// Matches returns the recorded matches.
func (r *MatchRecorder) Matches() []Match { return r.matches }

// Reset clears the recorded matches.
func (r *MatchRecorder) Reset() { r.matches = nil }
