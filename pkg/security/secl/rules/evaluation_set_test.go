// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
)

// Tests

func TestEvaluationSet_GetPolicies(t *testing.T) {
	type fields struct {
		RuleSets map[eval.RuleSetTagValue]*RuleSet
	}
	tests := []struct {
		name   string
		fields fields
		want   []*Policy
	}{
		{
			name: "duplicated policies",
			fields: fields{
				RuleSets: map[eval.RuleSetTagValue]*RuleSet{
					DefaultRuleSetTagValue: {
						policies: []*Policy{
							{Name: "policy 1"},
							{Name: "policy 2"},
						}},
					"threat_score": {
						policies: []*Policy{
							{Name: "policy 3"},
							{Name: "policy 2"},
						}},
					"special": {
						policies: []*Policy{
							{Name: "policy 3"},
							{Name: "policy 2"},
						}},
				},
			},
			want: []*Policy{{Name: "policy 1"},
				{Name: "policy 2"}, {Name: "policy 3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &EvaluationSet{
				RuleSets: tt.fields.RuleSets,
			}
			assert.Equalf(t, len(tt.want), len(es.GetPolicies()), "GetPolicies()")
			for _, policy := range tt.want {
				assert.Contains(t, es.GetPolicies(), policy)
			}
		})
	}
}

// go test -v github.com/DataDog/datadog-agent/pkg/security/secl/rules --run="TestEvaluationSet_LoadPolicies_Overriding"
func TestEvaluationSet_LoadPolicies_Overriding(t *testing.T) {
	type fields struct {
		Providers []PolicyProvider
		TagValues []eval.RuleSetTagValue
	}
	tests := []struct {
		name      string
		fields    fields
		want      func(t assert.TestingT, fields fields, got *EvaluationSet, msgs ...interface{}) bool
		wantErr   func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool
		wantRules map[eval.RuleID]*Rule
	}{
		{
			name: "duplicate IDs",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "default.policy",
								Source:  PolicyProviderTypeDir,
								Version: "",
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-default/shadow\"",
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-default/file\"",
									},
								},
								Macros: nil,
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myRC.policy",
								Source: PolicyProviderTypeRC,
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-custom/shadow\"",
									},
								},
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got *EvaluationSet, msgs ...interface{}) bool {
				gotNumberOfRules := len(got.RuleSets[DefaultRuleSetTagValue].rules)
				assert.Equal(t, 2, gotNumberOfRules)

				expectedRules := map[eval.RuleID]*Rule{
					"foo": {
						Rule: &eval.Rule{
							ID:         "foo",
							Expression: "open.file.path == \"/etc/local-default/shadow\"",
						},
						Definition: &RuleDefinition{
							ID:         "foo",
							Expression: "open.file.path == \"/etc/local-default/shadow\"",
						}},
					"bar": {
						Rule: &eval.Rule{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						},
						Definition: &RuleDefinition{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						}},
				}

				var r DiffReporter
				if !cmp.Equal(expectedRules, got.RuleSets[DefaultRuleSetTagValue].rules, cmp.Reporter(&r), cmpopts.IgnoreFields(Rule{}, "Opts", "Model"), cmpopts.IgnoreFields(RuleDefinition{}, "Policy"), cmpopts.IgnoreUnexported(eval.Rule{})) {
					assert.Fail(t, fmt.Sprintf("Diff: %s)", r.String()))
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				return assert.ErrorContains(t, err, "rule `foo` error: multiple definition with the same ID", fmt.Sprintf("Errors are: %+v", err.Errors))
			},
		},
		{
			name: "disabling a default rule via a different file",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "default.policy",
								Source:  PolicyProviderTypeDir,
								Version: "",
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-default/shadow\"",
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-default/file\"",
									},
								},
								Macros: nil,
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myRC.policy",
								Source: PolicyProviderTypeRC,
								Rules: []*RuleDefinition{
									{
										ID:       "foo",
										Disabled: true,
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/rc-custom/file\"",
									},
								},
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got *EvaluationSet, msgs ...interface{}) bool {
				gotNumberOfRules := len(got.RuleSets[DefaultRuleSetTagValue].rules)
				assert.Equal(t, 1, gotNumberOfRules)

				expectedRules := map[eval.RuleID]*Rule{
					"bar": {
						Rule: &eval.Rule{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						},
						Definition: &RuleDefinition{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						}},
				}

				var r DiffReporter
				if !cmp.Equal(expectedRules, got.RuleSets[DefaultRuleSetTagValue].rules, cmp.Reporter(&r), cmpopts.IgnoreFields(Rule{}, "Opts", "Model"), cmpopts.IgnoreFields(RuleDefinition{}, "Policy"), cmpopts.IgnoreUnexported(eval.Rule{})) {
					assert.Fail(t, fmt.Sprintf("Diff: %s)", r.String()))
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				return assert.ErrorContains(t, err, "rule `bar` error: multiple definition with the same ID", fmt.Sprintf("Errors are: %+v", err.Errors))
			},
		},
		{
			name: "disabling a default rule including ignored expression",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "default.policy",
								Source:  PolicyProviderTypeDir,
								Version: "",
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-default/shadow\"",
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-default/file\"",
									},
								},
								Macros: nil,
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myRC.policy",
								Source: PolicyProviderTypeRC,
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-custom/shadow\"",
										Disabled:   true,
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/rc-custom/file\"",
									},
								},
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got *EvaluationSet, msgs ...interface{}) bool {
				assert.Equal(t, 1, len(got.RuleSets))
				if _, ok := got.RuleSets[DefaultRuleSetTagValue]; !ok {
					t.Errorf("Missing %s rule set", DefaultRuleSetTagValue)
				}

				gotNumberOfRules := len(got.RuleSets[DefaultRuleSetTagValue].rules)
				assert.Equal(t, 1, gotNumberOfRules)

				expectedRules := map[eval.RuleID]*Rule{
					"bar": {
						Rule: &eval.Rule{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						},
						Definition: &RuleDefinition{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						}},
				}

				var r DiffReporter
				// TODO: Use custom cmp.Comparer instead of ignoring unexported fields
				if !cmp.Equal(expectedRules, got.RuleSets[DefaultRuleSetTagValue].rules, cmp.Reporter(&r),
					cmpopts.IgnoreFields(Rule{}, "Opts", "Model"), cmpopts.IgnoreFields(RuleDefinition{}, "Policy"),
					cmpopts.IgnoreFields(RuleSet{}, "opts", "evalOpts", "eventRuleBuckets", "fieldEvaluators", "model",
						"eventCtor", "listenersLock", "listeners", "globalVariables", "scopedVariables", "fields", "logger", "pool"),
					cmpopts.IgnoreUnexported(eval.Rule{})) {
					assert.Fail(t, fmt.Sprintf("Diff: %s)", r.String()))
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				return assert.ErrorContains(t, err, "rule `bar` error: multiple definition with the same ID", fmt.Sprintf("Errors are: %+v", err.Errors))
			},
		},
		{
			name: "disabling a default rule and creating a custom rule with same ID",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "default.policy",
								Source:  PolicyProviderTypeDir,
								Version: "",
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-default/shadow\"",
										Disabled:   true,
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-default/file\"",
									},
								},
								Macros: nil,
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myRC.policy",
								Source: PolicyProviderTypeRC,
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-custom/shadow\"",
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/rc-custom/file\"",
									},
								},
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got *EvaluationSet, msgs ...interface{}) bool {
				assert.Equal(t, 1, len(got.RuleSets))
				if _, ok := got.RuleSets[DefaultRuleSetTagValue]; !ok {
					t.Errorf("Missing %s rule set", DefaultRuleSetTagValue)
				}

				gotNumberOfRules := len(got.RuleSets[DefaultRuleSetTagValue].rules)
				assert.Equal(t, 1, gotNumberOfRules)

				expectedRules := map[eval.RuleID]*Rule{
					"bar": {
						Rule: &eval.Rule{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						},
						Definition: &RuleDefinition{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						}},
				}

				var r DiffReporter
				if !cmp.Equal(expectedRules, got.RuleSets[DefaultRuleSetTagValue].rules, cmp.Reporter(&r),
					cmpopts.IgnoreFields(Rule{}, "Opts", "Model"), cmpopts.IgnoreFields(RuleDefinition{}, "Policy"),
					cmpopts.IgnoreFields(RuleSet{}, "opts", "evalOpts", "eventRuleBuckets", "fieldEvaluators", "model",
						"eventCtor", "listenersLock", "listeners", "globalVariables", "scopedVariables", "fields", "logger", "pool"),
					cmpopts.IgnoreUnexported(eval.Rule{})) {
					assert.Fail(t, fmt.Sprintf("Diff: %s)", r.String()))
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				assert.Equal(t, 2, err.Len(), fmt.Sprintf("Errors are: %s", err.Errors))
				return assert.ErrorContains(t, err, "rule `bar` error: multiple definition with the same ID")
			},
		},
		{
			name: "combine:override",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "default.policy",
								Source:  PolicyProviderTypeDir,
								Version: "",
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-default/shadow\"",
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-default/file\"",
									},
								},
								Macros: nil,
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myRC.policy",
								Source: PolicyProviderTypeRC,
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-custom/shadow\"",
										Combine:    OverridePolicy,
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/rc-custom/file\"",
									},
								},
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got *EvaluationSet, msgs ...interface{}) bool {
				assert.Equal(t, 1, len(got.RuleSets))
				if _, ok := got.RuleSets[DefaultRuleSetTagValue]; !ok {
					t.Errorf("Missing %s rule set", DefaultRuleSetTagValue)
				}

				assert.Equal(t, 2, len(got.RuleSets[DefaultRuleSetTagValue].rules))

				expectedRules := map[eval.RuleID]*Rule{
					"foo": {
						Rule: &eval.Rule{
							ID:         "foo",
							Expression: "open.file.path == \"/etc/rc-custom/shadow\"",
						},
						Definition: &RuleDefinition{
							ID:         "foo",
							Expression: "open.file.path == \"/etc/rc-custom/shadow\"",
						}},
					"bar": {
						Rule: &eval.Rule{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						},
						Definition: &RuleDefinition{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/local-default/file\"",
						},
					},
				}

				var r DiffReporter
				// TODO: Use custom cmp.Comparer instead of ignoring unexported fields
				if !cmp.Equal(expectedRules, got.RuleSets[DefaultRuleSetTagValue].rules, cmp.Reporter(&r),
					cmpopts.IgnoreFields(Rule{}, "Opts", "Model"), cmpopts.IgnoreFields(RuleDefinition{}, "Policy"),
					cmpopts.IgnoreFields(RuleSet{}, "opts", "evalOpts", "eventRuleBuckets", "fieldEvaluators", "model",
						"eventCtor", "listenersLock", "listeners", "globalVariables", "scopedVariables", "fields", "logger", "pool"),
					cmpopts.IgnoreUnexported(eval.Rule{})) {
					assert.Fail(t, fmt.Sprintf("Diff: %s)", r.String()))
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				assert.Equal(t, 1, err.Len(), fmt.Sprintf("Errors are: %s", err.Errors))
				return assert.ErrorContains(t, err, "rule `bar` error: multiple definition with the same ID", fmt.Sprintf("Errors are: %+v", err.Errors))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			p := &PolicyLoader{
				Providers: tt.fields.Providers,
			}

			policyLoaderOpts := PolicyLoaderOpts{}
			es, _ := newTestEvaluationSet(tt.fields.TagValues)

			err := es.LoadPolicies(p, policyLoaderOpts)

			tt.want(t, tt.fields, es)
			tt.wantErr(t, err)
		})
	}
}

func TestEvaluationSet_LoadPolicies_PolicyPrecedence(t *testing.T) {
	type fields struct {
		Providers []PolicyProvider
		TagValues []eval.RuleSetTagValue
	}
	tests := []struct {
		name    string
		fields  fields
		want    func(t assert.TestingT, fields fields, got *EvaluationSet, msgs ...interface{}) bool
		wantErr func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool
	}{
		{
			name: "RC Default replaces Local Default and overrides all else, and RC Custom overrides Local Custom",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myLocal.policy",
								Source: PolicyProviderTypeDir,
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-custom/foo\"",
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-custom/bar\"",
									},
									{
										ID:         "baz",
										Expression: "open.file.path == \"/etc/local-custom/baz\"",
									},
									{
										ID:         "alpha",
										Expression: "open.file.path == \"/etc/local-custom/alpha\"",
									},
								},
							}, {
								Name:   DefaultPolicyName,
								Source: PolicyProviderTypeDir,
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-default/foo\"",
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-default/bar\"",
									},
									{
										ID:         "baz",
										Expression: "open.file.path == \"/etc/local-default/baz\"",
									},
									{
										ID:         "alpha",
										Expression: "open.file.path == \"/etc/local-default/alpha\"",
									},
								},
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myRC.policy",
								Source: PolicyProviderTypeRC,
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-custom/foo\"",
									},
									{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/rc-custom/bar\"",
									},
									{
										ID:         "baz",
										Expression: "open.file.path == \"/etc/rc-custom/baz\"",
									},
								},
							}, {
								Name:   DefaultPolicyName,
								Source: PolicyProviderTypeRC,
								Rules: []*RuleDefinition{
									{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-default/foo\"",
									},
								},
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got *EvaluationSet, msgs ...interface{}) bool {
				gotNumberOfRules := len(got.RuleSets[DefaultRuleSetTagValue].rules)
				assert.Equal(t, 4, gotNumberOfRules)

				expectedRules := map[eval.RuleID]*Rule{
					"foo": {
						Rule: &eval.Rule{
							ID:         "foo",
							Expression: "open.file.path == \"/etc/rc-default/foo\"",
						},
						Definition: &RuleDefinition{
							ID:         "foo",
							Expression: "open.file.path == \"/etc/rc-default/foo\"",
						}},
					"bar": {
						Rule: &eval.Rule{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/rc-custom/bar\"",
						},
						Definition: &RuleDefinition{
							ID:         "bar",
							Expression: "open.file.path == \"/etc/rc-custom/bar\"",
						}},
					"baz": {
						Rule: &eval.Rule{
							ID:         "baz",
							Expression: "open.file.path == \"/etc/rc-custom/baz\"",
						},
						Definition: &RuleDefinition{
							ID:         "baz",
							Expression: "open.file.path == \"/etc/rc-custom/baz\"",
						}},
					"alpha": {
						Rule: &eval.Rule{
							ID:         "alpha",
							Expression: "open.file.path == \"/etc/local-custom/alpha\"",
						},
						Definition: &RuleDefinition{
							ID:         "alpha",
							Expression: "open.file.path == \"/etc/local-custom/alpha\"",
						}},
				}

				var r DiffReporter

				// TODO: Use custom cmp.Comparer instead of ignoring unexported fields
				if !cmp.Equal(expectedRules, got.RuleSets[DefaultRuleSetTagValue].rules, cmp.Reporter(&r),
					cmpopts.IgnoreFields(Rule{}, "Opts", "Model"), cmpopts.IgnoreFields(RuleDefinition{}, "Policy"),
					cmpopts.IgnoreFields(RuleSet{}, "opts", "evalOpts", "eventRuleBuckets", "fieldEvaluators", "model",
						"eventCtor", "listenersLock", "listeners", "globalVariables", "scopedVariables", "fields", "logger", "pool"),
					cmpopts.IgnoreUnexported(eval.Rule{})) {
					assert.Fail(t, fmt.Sprintf("Diff: %s)", r.String()))
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				assert.Equal(t, err.Len(), 4, "Expected %d errors, got %d: %+v", 1, err.Len(), err)
				assert.ErrorContains(t, err, "rule `foo` error: multiple definition with the same ID", fmt.Sprintf("Errors are: %+v", err.Errors))
				assert.ErrorContains(t, err, "rule `bar` error: multiple definition with the same ID", fmt.Sprintf("Errors are: %+v", err.Errors))
				return assert.ErrorContains(t, err, "rule `baz` error: multiple definition with the same ID", fmt.Sprintf("Errors are: %+v", err.Errors))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PolicyLoader{
				Providers: tt.fields.Providers,
			}

			policyLoaderOpts := PolicyLoaderOpts{}
			es, _ := newTestEvaluationSet(tt.fields.TagValues)

			err := es.LoadPolicies(p, policyLoaderOpts)

			tt.want(t, tt.fields, es)
			tt.wantErr(t, err)
		})
	}
}

func TestEvaluationSet_LoadPolicies_RuleSetTags(t *testing.T) {
	type args struct {
		policy    *PolicyDef
		tagValues []eval.RuleSetTagValue
	}
	tests := []struct {
		name    string
		args    args
		want    func(t assert.TestingT, args args, got *EvaluationSet, msgs ...interface{}) bool
		wantErr func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool
	}{
		{
			name: "just threat score",
			args: args{
				policy: &PolicyDef{
					Rules: []*RuleDefinition{
						{
							ID:         "testA",
							Expression: `open.file.path == "/tmp/test"`,
						},
						{
							ID:         "testB",
							Expression: `open.file.path == "/tmp/test"`,
							Tags:       map[string]string{"ruleset": "threat_score"},
						},
						{
							ID:         "testC",
							Expression: `open.file.path == "/tmp/toto"`,
						},
					},
				},
				tagValues: []eval.RuleSetTagValue{"threat_score"},
			},
			want: func(t assert.TestingT, args args, got *EvaluationSet, msgs ...interface{}) bool {
				gotNumOfRules := len(got.RuleSets["threat_score"].rules)
				expected := 1
				assert.Equal(t, expected, gotNumOfRules)

				return assert.Equal(t, 1, len(got.RuleSets))
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				return assert.Nil(t, err, msgs)
			},
		},
		{
			name: "just probe evaluation",
			args: args{
				policy: &PolicyDef{
					Rules: []*RuleDefinition{
						{
							ID:         "testA",
							Expression: `open.file.path == "/tmp/test"`,
						},
						{
							ID:         "testB",
							Expression: `open.file.path == "/tmp/test"`,
							Tags:       map[string]string{"ruleset": DefaultRuleSetTagValue},
						},
						{
							ID:         "testC",
							Expression: `open.file.path == "/tmp/toto"`,
						},
					},
				},
				tagValues: []eval.RuleSetTagValue{DefaultRuleSetTagValue},
			},
			want: func(t assert.TestingT, args args, got *EvaluationSet, msgs ...interface{}) bool {
				gotNumberOfRules := len(got.RuleSets[DefaultRuleSetTagValue].rules)
				expected := 3
				assert.Equal(t, expected, gotNumberOfRules)

				return assert.Equal(t, 1, len(got.RuleSets))
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				return assert.Nil(t, err, msgs)
			},
		},
		{
			name: "mix of tags",
			args: args{
				policy: &PolicyDef{
					Rules: []*RuleDefinition{
						{
							ID:         "testA",
							Expression: `open.file.path == "/tmp/test"`,
						},
						{
							ID:         "testB",
							Expression: `open.file.path == "/tmp/test"`,
							Tags:       map[string]string{"ruleset": "threat_score"},
						},
						{
							ID:         "testC",
							Expression: `open.file.path == "/tmp/toto"`,
							Tags:       map[string]string{"ruleset": "special"},
						},
						{
							ID:         "testD",
							Expression: `open.file.path == "/tmp/toto"`,
							Tags:       map[string]string{"threat_score": "4", "ruleset": "special"},
						},
					},
				},
				tagValues: []eval.RuleSetTagValue{DefaultRuleSetTagValue, "threat_score", "special"},
			},
			want: func(t assert.TestingT, args args, got *EvaluationSet, msgs ...interface{}) bool {
				assert.Equal(t, len(args.tagValues), len(got.RuleSets))

				gotNumProbeEvalRules := len(got.RuleSets[DefaultRuleSetTagValue].rules)
				expected := 1
				assert.Equal(t, expected, gotNumProbeEvalRules)

				gotNumThreatScoreRules := len(got.RuleSets["threat_score"].rules)
				expectedNumThreatScoreRules := 1
				assert.Equal(t, expectedNumThreatScoreRules, gotNumThreatScoreRules)

				gotNumSpecialRules := len(got.RuleSets["special"].rules)
				expectedNumSpecialRules := 2
				assert.Equal(t, expectedNumSpecialRules, gotNumSpecialRules)

				return assert.Equal(t, 3, len(got.RuleSets))
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				return assert.Nil(t, err, msgs)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyLoaderOpts := PolicyLoaderOpts{}
			loader, es := loadPolicySetup(t, tt.args.policy, tt.args.tagValues)

			err := es.LoadPolicies(loader, policyLoaderOpts)
			tt.want(t, tt.args, es)
			tt.wantErr(t, err)
		})
	}
}

func TestNewEvaluationSet(t *testing.T) {
	ruleSet := newRuleSet()
	ruleSetWithThreatScoreTag := newRuleSet()
	ruleSetWithThreatScoreTag.setRuleSetTagValue("threat_score")

	type args struct {
		ruleSetsToInclude []*RuleSet
	}
	tests := []struct {
		name    string
		args    args
		want    func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "no rule sets",
			args: args{ruleSetsToInclude: []*RuleSet{}},
			want: func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool {
				return assert.Nil(t, got)
			},
			wantErr: func(t assert.TestingT, err error, msgs ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrNoRuleSetsInEvaluationSet, msgs)
			},
		},
		{
			name: "just probe evaluation ruleset",
			args: args{[]*RuleSet{ruleSet}},
			want: func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool {
				expected := &EvaluationSet{RuleSets: map[eval.RuleSetTagValue]*RuleSet{"probe_evaluation": ruleSet}}
				return assert.Equal(t, expected, got, msgs)
			},
			wantErr: func(t assert.TestingT, err error, msgs ...interface{}) bool {
				return assert.ErrorIs(t, err, nil, msgs)
			},
		},
		{
			name: "just non-probe evaluation ruleset",
			args: args{ruleSetsToInclude: []*RuleSet{ruleSetWithThreatScoreTag}},
			want: func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool {
				expected := &EvaluationSet{RuleSets: map[eval.RuleSetTagValue]*RuleSet{"threat_score": ruleSetWithThreatScoreTag}}
				return assert.Equal(t, expected, got, msgs)
			},
			wantErr: func(t assert.TestingT, err error, msgs ...interface{}) bool {
				return assert.ErrorIs(t, err, nil, msgs)
			},
		},
		{
			name: "multiple rule sets",
			args: args{ruleSetsToInclude: []*RuleSet{ruleSetWithThreatScoreTag, ruleSet}},
			want: func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool {
				expected := &EvaluationSet{RuleSets: map[eval.RuleSetTagValue]*RuleSet{"threat_score": ruleSetWithThreatScoreTag, "probe_evaluation": ruleSet}}
				return assert.Equal(t, expected, got, msgs)
			},
			wantErr: func(t assert.TestingT, err error, msgs ...interface{}) bool {
				return assert.ErrorIs(t, err, nil, msgs)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewEvaluationSet(tt.args.ruleSetsToInclude)
			tt.wantErr(t, err, fmt.Sprintf("NewEvaluationSet(%v)", tt.args.ruleSetsToInclude))
			tt.want(t, got, fmt.Sprintf("NewEvaluationSet(%v)", tt.args.ruleSetsToInclude))
		})
	}
}

// Test Utilities
func newTestEvaluationSet(tagValues []eval.RuleSetTagValue) (*EvaluationSet, error) {
	var ruleSetsToInclude []*RuleSet
	if len(tagValues) > 0 {
		for _, tagValue := range tagValues {
			rs := newRuleSet()
			rs.setRuleSetTagValue(tagValue)
			ruleSetsToInclude = append(ruleSetsToInclude, rs)
		}
	} else {
		rs := newRuleSet()
		ruleSetsToInclude = append(ruleSetsToInclude, rs)
	}

	return NewEvaluationSet(ruleSetsToInclude)
}

func loadPolicyIntoProbeEvaluationRuleSet(t *testing.T, testPolicy *PolicyDef, policyOpts PolicyLoaderOpts) (*EvaluationSet, *multierror.Error) {
	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir, false)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewPolicyLoader(provider)

	evaluationSet, _ := newTestEvaluationSet([]eval.RuleSetTagValue{})
	return evaluationSet, evaluationSet.LoadPolicies(loader, policyOpts)
}

func loadPolicySetup(t *testing.T, testPolicy *PolicyDef, tagValues []eval.RuleSetTagValue) (*PolicyLoader, *EvaluationSet) {
	tmpDir := t.TempDir()

	if err := savePolicy(filepath.Join(tmpDir, "test.policy"), testPolicy); err != nil {
		t.Fatal(err)
	}

	provider, err := NewPoliciesDirProvider(tmpDir, false)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewPolicyLoader(provider)

	evaluationSet, _ := newTestEvaluationSet(tagValues)
	return loader, evaluationSet
}

// The following is from https://pkg.go.dev/github.com/google/go-cmp@v0.5.9/cmp#example-Reporter
// DiffReporter is a simple custom reporter that only records differences
// detected during comparison.
type DiffReporter struct {
	path  cmp.Path
	diffs []string
}

func (r *DiffReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *DiffReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		vx, vy := r.path.Last().Values()
		r.diffs = append(r.diffs, fmt.Sprintf("%#v:\n\t-: %+v\n\t+: %+v\n", r.path, vx, vy))
	}
}

func (r *DiffReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

func (r *DiffReporter) String() string {
	return strings.Join(r.diffs, "\n")
}
