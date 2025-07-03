// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package monitor holds rules related files
package monitor

import (
	"runtime"
	"testing"

	"github.com/Masterminds/semver/v3"
	gocmp "github.com/google/go-cmp/cmp"
	gocmpopts "github.com/google/go-cmp/cmp/cmpopts"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/rules/filtermodel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

type testPolicy struct {
	info rules.PolicyInfo
	def  rules.PolicyDef
}

type testCase struct {
	name                 string
	policies             []*testPolicy
	expectedPolicyStates []*PolicyState
}

func TestPolicyMonitorPolicyState(t *testing.T) {

	testCases := []*testCase{
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
					Status: PolicyStatusPartiallyFiltered,
					Rules: []*RuleState{
						{
							ID:         "rule_a",
							Expression: `exec.file.path == "/etc/foo/bar"`,
							Status:     "loaded",
						},
						{
							ID:                     "rule_b",
							Expression:             `exec.file.path == "/etc/foo/baz"`,
							Status:                 "filtered",
							Message:                "this agent version doesn't support this rule",
							FilterType:             string(rules.FilterTypeAgentVersion),
							AgentVersionConstraint: "< 0.0.1",
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
			expectedPolicyStates: []*PolicyState{
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy A",
						Source: "test",
					},
					Status: PolicyStatusFullyFiltered,
					Rules: []*RuleState{
						{
							ID:                     "rule_a",
							Expression:             `exec.file.path == "/etc/foo/bar"`,
							Status:                 "filtered",
							Message:                "this agent version doesn't support this rule",
							FilterType:             string(rules.FilterTypeAgentVersion),
							AgentVersionConstraint: "< 0.0.1",
						},
						{
							ID:                     "rule_b",
							Expression:             `exec.file.path == "/etc/foo/baz"`,
							Status:                 "filtered",
							Message:                "this agent version doesn't support this rule",
							FilterType:             string(rules.FilterTypeAgentVersion),
							AgentVersionConstraint: "< 0.0.2",
						},
					},
				},
			},
		},
		{
			name: "multiple rules with one agent constraint passing",
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
								ID:                     "rule_a",
								Expression:             `exec.file.path == "/etc/foo/bar" && exec.file.name == "bar"`,
								AgentVersionConstraint: "< 0.0.2",
							},
							{
								ID:                     "rule_a",
								Expression:             `exec.file.path == "/etc/foo/bar" && exec.file.name == "bar" && exec.pid == 42`,
								AgentVersionConstraint: "~7.x",
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
							ID:                     "rule_a",
							Expression:             `exec.file.path == "/etc/foo/bar" && exec.file.name == "bar" && exec.pid == 42`,
							Status:                 "loaded",
							AgentVersionConstraint: "~7.x",
						},
					},
				},
			},
		},
		{
			name: "multiple policies with same rules but different constraints",
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
				{
					info: rules.PolicyInfo{
						Name:   "Policy B",
						Source: "test",
					},
					def: rules.PolicyDef{
						Rules: []*rules.RuleDefinition{
							{
								ID:                     "rule_a",
								Expression:             `exec.file.path == "/etc/foo/bar" && exec.pid == 42`,
								AgentVersionConstraint: "~7.x",
							},
							{
								ID:                     "rule_b",
								Expression:             `exec.file.path == "/etc/foo/baz"`,
								AgentVersionConstraint: "< 0.0.2",
							},
							{
								ID:                     "rule_c",
								Expression:             `exec.file.path == "/etc/foo/qwak"`,
								AgentVersionConstraint: ">= 7.42.0",
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
					Status: PolicyStatusFullyFiltered,
					Rules: []*RuleState{
						{
							ID:                     "rule_a",
							Expression:             `exec.file.path == "/etc/foo/bar"`,
							Status:                 "filtered",
							Message:                "this agent version doesn't support this rule",
							FilterType:             string(rules.FilterTypeAgentVersion),
							AgentVersionConstraint: "< 0.0.1",
						},
						{
							ID:                     "rule_b",
							Expression:             `exec.file.path == "/etc/foo/baz"`,
							Status:                 "filtered",
							Message:                "this agent version doesn't support this rule",
							FilterType:             string(rules.FilterTypeAgentVersion),
							AgentVersionConstraint: "< 0.0.2",
						},
					},
				},
				{
					PolicyMetadata: PolicyMetadata{
						Name:   "Policy B",
						Source: "test",
					},
					Status: PolicyStatusPartiallyFiltered,
					Rules: []*RuleState{
						{
							ID:                     "rule_a",
							Expression:             `exec.file.path == "/etc/foo/bar" && exec.pid == 42`,
							Status:                 "loaded",
							AgentVersionConstraint: "~7.x",
						},
						{
							ID:                     "rule_b",
							Expression:             `exec.file.path == "/etc/foo/baz"`,
							Status:                 "filtered",
							Message:                "this agent version doesn't support this rule",
							FilterType:             string(rules.FilterTypeAgentVersion),
							AgentVersionConstraint: "< 0.0.2",
						},
						{
							ID:                     "rule_c",
							Expression:             `exec.file.path == "/etc/foo/qwak"`,
							Status:                 "loaded",
							AgentVersionConstraint: ">= 7.42.0",
						},
					},
				},
			},
		},
	}

	if runtime.GOOS == "linux" {
		testCases = append(testCases, []*testCase{
			{
				name: "policy with os filters",
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
									Filters:    []string{"os == \"linux\""},
								},
								{
									ID:         "rule_b",
									Expression: `exec.file.path == "/etc/foo/baz"`,
									Filters:    []string{"os == \"windows\""},
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
						Status: PolicyStatusPartiallyFiltered,
						Rules: []*RuleState{
							{
								ID:         "rule_a",
								Expression: `exec.file.path == "/etc/foo/bar"`,
								Status:     "loaded",
								Filters:    []string{"os == \"linux\""},
							},
							{
								ID:         "rule_b",
								Expression: `exec.file.path == "/etc/foo/baz"`,
								Status:     "filtered",
								Message:    "none of the rule filters matched the host or configuration of this agent",
								FilterType: string(rules.FilterTypeRuleFilter),
								Filters:    []string{"os == \"windows\""},
							},
						},
					},
				},
			},
		}...)
	} else if runtime.GOOS == "windows" {
		testCases = append(testCases, []*testCase{
			{
				name: "policy with os filters",
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
									Filters:    []string{"os == \"linux\""},
								},
								{
									ID:         "rule_b",
									Expression: `exec.file.path == "/etc/foo/baz"`,
									Filters:    []string{"os == \"windows\""},
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
						Status: PolicyStatusPartiallyFiltered,
						Rules: []*RuleState{
							{
								ID:         "rule_a",
								Expression: `exec.file.path == "/etc/foo/bar"`,
								Status:     "filtered",
								Message:    "none of the rule filters matched the host or configuration of this agent",
								FilterType: string(rules.FilterTypeRuleFilter),
								Filters:    []string{"os == \"linux\""},
							},
							{
								ID:         "rule_b",
								Expression: `exec.file.path == "/etc/foo/baz"`,
								Status:     "loaded",
								Filters:    []string{"os == \"windows\""},
							},
						},
					},
				},
			},
		}...)
	}

	var macroFilters []rules.MacroFilter
	var ruleFilters []rules.RuleFilter

	seclRuleFilter := rules.NewSECLRuleFilter(filtermodel.NewOSOnlyFilterModel(runtime.GOOS))

	macroFilters = append(macroFilters, seclRuleFilter)
	ruleFilters = append(ruleFilters, seclRuleFilter)

	agentVersionFilter, err := rules.NewAgentVersionFilter(semver.MustParse("7.42.0"))
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

			assert.True(t, gocmp.Equal(tc.expectedPolicyStates, policyStates, goCmpOpts...), gocmp.Diff(tc.expectedPolicyStates, policyStates, goCmpOpts...))
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
