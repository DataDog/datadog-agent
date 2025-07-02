// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package monitor holds rules related files
package monitor

import (
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

type testPolicy struct {
	info rules.PolicyInfo
	def  rules.PolicyDef
}

func TestPolicyMonitorPolicyState(t *testing.T) {
	testCases := []struct {
		name                 string
		policies             []*testPolicy
		testPolicyStatesCb   func(t *testing.T, states []*PolicyState)
		expectedPolicyStates []*PolicyState
	}{
		{
			name:                 "no policy",
			policies:             nil,
			expectedPolicyStates: nil,
		},
		{
			name: "single policy",
			policies: []*testPolicy{
				{
					info: rules.PolicyInfo{
						Name:   "Policy A",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID:         "rule_a",
								Expression: `exec.file.path == "/etc/foo/bar"`,
							},
						},
					},
				},
			},
			expectedPolicyStates: []*PolicyState{
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy A",
						Source: "test",
					},
					Status: PolicyStatusLoaded,
					Rules: []*RuleState{
						{
							ID:         "rule_a",
							Expression: `exec.file.path == "/etc/foo/bar"`,
							Status:     "loaded",
						},
					},
				},
			},
		},
		{
			name: "rule with no expression",
			policies: []*testPolicy{
				{
					info: rules.PolicyInfo{
						Name:   "Policy A",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID: "rule_a",
							},
						},
					},
				},
			},
			expectedPolicyStates: []*PolicyState{
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy A",
						Source: "test",
					},
					Status: PolicyStatusFullyRejected,
					Rules: []*RuleState{
						{
							ID:      "rule_a",
							Status:  "error",
							Message: rules.ErrRuleWithoutExpression.Error(),
						},
					},
				},
			},
		},
		{
			name: "empty policy",
			policies: []*testPolicy{
				{
					info: rules.PolicyInfo{
						Name:   "Empty Policy",
						Source: "test",
					},
					def: rules.PolicyDef{},
				},
			},
			expectedPolicyStates: []*PolicyState{
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Empty Policy",
						Source: "test",
					},
					Status:  PolicyStatusError,
					Message: rules.ErrPolicyIsEmpty.Error(),
				},
			},
		},
		{
			name: "multiple policies with same erroneous rule",
			policies: []*testPolicy{
				{
					info: rules.PolicyInfo{
						Name:   "Policy A",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID:         "rule_a",
								Expression: `exec.foo.bar == "/etc/foo/bar"`,
							},
							{
								ID:         "rule_b",
								Expression: `foo.bar.path == "/etc/foo/bar"`,
							},
						},
					},
				},
				{
					info: rules.PolicyInfo{
						Name:   "Policy B",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID:         "rule_a",
								Expression: `exec.foo.bar == "/etc/foo/bar"`,
							},
						},
					},
				},
			},
			expectedPolicyStates: []*PolicyState{
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy A",
						Source: "test",
					},
					Status: PolicyStatusFullyRejected,
					Rules: []*RuleState{
						{
							ID:         "rule_a",
							Expression: `exec.foo.bar == "/etc/foo/bar"`,
							Status:     "error",
							Message:    "rule compilation error: field `exec.foo.bar` not found",
						},
						{
							ID:         "rule_b",
							Expression: `foo.bar.path == "/etc/foo/bar"`,
							Status:     "error",
							Message:    "rule compilation error: field `foo.bar.path` not found",
						},
					},
				},
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy B",
						Source: "test",
					},
					Status: PolicyStatusFullyRejected,
					Rules: []*RuleState{
						{
							ID:         "rule_a",
							Expression: `exec.foo.bar == "/etc/foo/bar"`,
							Status:     "error",
							Message:    "rule compilation error: field `exec.foo.bar` not found",
						},
					},
				},
			},
		},
		{
			name: "partially loaded policy",
			policies: []*testPolicy{
				{
					info: rules.PolicyInfo{
						Name:   "Policy A",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID:         "rule_a",
								Expression: `exec.foo.bar == "/etc/foo/bar"`,
							},
							{
								ID:         "rule_b",
								Expression: `exec.file.path == "/etc/foo/bar"`,
							},
						},
					},
				},
				{
					info: rules.PolicyInfo{
						Name:   "Policy B",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID:         "rule_a",
								Expression: `exec.foo.bar == "/etc/foo/bar"`,
							},
						},
					},
				},
			},
			expectedPolicyStates: []*PolicyState{
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy A",
						Source: "test",
					},
					Status: PolicyStatusPartiallyLoaded,
					Rules: []*RuleState{
						{
							ID:         "rule_b",
							Expression: `exec.file.path == "/etc/foo/bar"`,
							Status:     "loaded",
						},
						{
							ID:         "rule_a",
							Expression: `exec.foo.bar == "/etc/foo/bar"`,
							Status:     "error",
							Message:    "rule compilation error: field `exec.foo.bar` not found",
						},
					},
				},
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy B",
						Source: "test",
					},
					Status: PolicyStatusFullyRejected,
					Rules: []*RuleState{
						{
							ID:         "rule_a",
							Expression: `exec.foo.bar == "/etc/foo/bar"`,
							Status:     "error",
							Message:    "rule compilation error: field `exec.foo.bar` not found",
						},
					},
				},
			},
		},
		{
			name: "policy with one filtered rule",
			policies: []*testPolicy{
				{
					info: rules.PolicyInfo{
						Name:   "Policy A",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID:         "rule_a",
								Expression: `exec.file.path == "/etc/foo/bar"`,
							},
							{
								ID:                     "rule_b",
								Expression:             `exec.file.path == "/etc/foo/baz"`,
								AgentVersionConstraint: "< 0.0.1",
							},
						},
					},
				},
			},
			expectedPolicyStates: []*PolicyState{
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy A",
						Source: "test",
					},
					Status: PolicyStatusLoaded,
					Rules: []*RuleState{
						{
							ID:         "rule_a",
							Expression: `exec.file.path == "/etc/foo/bar"`,
							Status:     "loaded",
						},
					},
				},
			},
		},
		{
			name: "policy with only filtered rules",
			policies: []*testPolicy{
				{
					info: rules.PolicyInfo{
						Name:   "Policy A",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID:                     "rule_a",
								Expression:             `exec.file.path == "/etc/foo/bar"`,
								AgentVersionConstraint: "< 0.0.1",
							},
							{
								ID:                     "rule_b",
								Expression:             `exec.file.path == "/etc/foo/baz"`,
								AgentVersionConstraint: "< 0.0.2",
							},
						},
					},
				},
			},
			expectedPolicyStates: nil,
		},
	}

	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		t.Fatal("failed to get agent version:", err)
	}

	var macroFilters []rules.MacroFilter
	var ruleFilters []rules.RuleFilter

	agentVersionFilter, err := rules.NewAgentVersionFilter(agentVersion)
	if err != nil {
		t.Fatal("failed to create agent version filter:", err)
	} else {
		macroFilters = append(macroFilters, agentVersionFilter)
		ruleFilters = append(ruleFilters, agentVersionFilter)
	}

	eventCtor := func() eval.Event {
		return &model.Event{}
	}
	ruleOpts, evalOpts := rules.NewBothOpts(map[eval.EventType]bool{"*": true})

	// Sort options for gocmp to ensure consistent ordering of slices
	goCmpOpts := []gocmp.Option{
		gocmpopts.SortSlices(func(a, b *PolicyState) bool {
			return a.PolicyMetadata.Name < b.PolicyMetadata.Name
		}),
		gocmpopts.SortSlices(func(a, b *RuleState) bool {
			return a.ID < b.ID
		}),
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rs := rules.NewRuleSet(&model.Model{}, eventCtor, ruleOpts, evalOpts)
			loader := rules.NewPolicyLoader(newTestPolicyProvider(tc.policies...))
			filteredRules, errs := rs.LoadPolicies(loader, rules.PolicyLoaderOpts{MacroFilters: macroFilters, RuleFilters: ruleFilters})
			policyStates := NewPoliciesState(rs, filteredRules, errs, false)

			assert.True(t, gocmp.Equal(tc.expectedPolicyStates, policyStates, goCmpOpts...), gocmp.Diff(tc.expectedPolicyStates, policyStates, goCmpOpts...), "policy states mismatch")
		})
	}
}

type testPolicyProvider struct {
	policies []*testPolicy
}

func newTestPolicyProvider(policies ...*testPolicy) *testPolicyProvider {
	return &testPolicyProvider{
		policies: policies,
	}
}

func (p *testPolicyProvider) Type() string {
	return "test"
}

func (p *testPolicyProvider) LoadPolicies(macroFilters []rules.MacroFilter, ruleFilters []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	var policies []*rules.Policy
	var multiErr *multierror.Error

	for _, policy := range p.policies {
		p, err := rules.LoadPolicyFromDefinition(&policy.info, &policy.def, macroFilters, ruleFilters)
		if p != nil {
			policies = append(policies, p)
		}
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	return policies, multiErr
}

func (p *testPolicyProvider) Start() {
	// No-op for test provider
}

func (p *testPolicyProvider) Close() error {
	// No-op for test provider
	return nil
}

func (p *testPolicyProvider) SetOnNewPoliciesReadyCb(_ func()) {
	// No-op for test provider
}
