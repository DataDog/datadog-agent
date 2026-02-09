// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// compare all Policy fields but the `Def` field
var policyCmpOpts = []cmp.Option{
	cmp.AllowUnexported(Policy{}),
	cmpopts.IgnoreFields(Policy{}, "Def"),
}

// go test -tags test -v github.com/DataDog/datadog-agent/pkg/security/secl/rules --run="TestPolicyLoader_LoadPolicies"
func TestPolicyLoader_LoadPolicies(t *testing.T) {
	type fields struct {
		Providers []PolicyProvider
	}
	type args struct {
		opts PolicyLoaderOpts
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    func(t assert.TestingT, got []*Policy, msgs ...interface{}) bool
		wantErr func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool
	}{
		{
			name: "RC Default replaces Local Default, and RC Custom overrides Local Custom",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "myLocal.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/local-custom/foo\"",
											},
											{
												ID:         "bar",
												Expression: "open.file.path == \"/etc/local-custom/bar\"",
											},
										},
									},
								},
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeDir,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/local-default/foo\"",
											},
											{
												ID:         "baz",
												Expression: "open.file.path == \"/etc/local-default/baz\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "myRC.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/rc-custom/foo\"",
											},
											{
												ID:         "alpha",
												Expression: "open.file.path == \"/etc/rc-custom/alpha\"",
											},
										},
									},
								},
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/rc-default/foo\"",
											},
											{
												ID:         "bravo",
												Expression: "open.file.path == \"/etc/rc-default/bravo\"",
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Info: PolicyInfo{
							Name:         DefaultPolicyName,
							Source:       PolicyProviderTypeRC,
							InternalType: DefaultPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "foo",
									Expression: "open.file.path == \"/etc/rc-default/foo\"",
								},
								Policy: PolicyInfo{
									Name:         DefaultPolicyName,
									Source:       PolicyProviderTypeRC,
									InternalType: DefaultPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         DefaultPolicyName,
									Source:       PolicyProviderTypeRC,
									InternalType: DefaultPolicyType,
								}},
								Accepted: true,
							},
							{
								Def: &RuleDefinition{
									ID:         "bravo",
									Expression: "open.file.path == \"/etc/rc-default/bravo\"",
								},
								Policy: PolicyInfo{
									Name:         DefaultPolicyName,
									Source:       PolicyProviderTypeRC,
									InternalType: DefaultPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         DefaultPolicyName,
									Source:       PolicyProviderTypeRC,
									InternalType: DefaultPolicyType,
								}},
								Accepted: true,
							},
						},
					},
					{
						Info: PolicyInfo{
							Name:         "myRC.policy",
							Source:       PolicyProviderTypeRC,
							InternalType: CustomPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "foo",
									Expression: "open.file.path == \"/etc/rc-custom/foo\"",
								},
								Policy: PolicyInfo{
									Name:         "myRC.policy",
									Source:       PolicyProviderTypeRC,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myRC.policy",
									Source:       PolicyProviderTypeRC,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
							{
								Def: &RuleDefinition{
									ID:         "alpha",
									Expression: "open.file.path == \"/etc/rc-custom/alpha\"",
								},
								Policy: PolicyInfo{
									Name:         "myRC.policy",
									Source:       PolicyProviderTypeRC,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myRC.policy",
									Source:       PolicyProviderTypeRC,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
						},
					},
					{
						Info: PolicyInfo{
							Name:         "myLocal.policy",
							Source:       PolicyProviderTypeDir,
							InternalType: CustomPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "foo",
									Expression: "open.file.path == \"/etc/local-custom/foo\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
							{
								Def: &RuleDefinition{
									ID:         "bar",
									Expression: "open.file.path == \"/etc/local-custom/bar\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
						},
					},
				}

				defaultPolicyCount, lastSeenDefaultPolicyIdx := numAndLastIdxOfDefaultPolicies(expectedLoadedPolicies)

				assert.Equalf(t, 1, defaultPolicyCount, "There are more than 1 default policies")
				assert.Equalf(t, PolicyProviderTypeRC, got[lastSeenDefaultPolicyIdx].Info.Source, "The default policy is not from RC")

				if !cmp.Equal(expectedLoadedPolicies, got, policyCmpOpts...) {
					t.Errorf("The loaded policies do not match the expected\nDiff:\n%s", cmp.Diff(expectedLoadedPolicies, got, policyCmpOpts...))
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "No default policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "myLocal.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/local-custom/foo\"",
											},
											{
												ID:         "bar",
												Expression: "open.file.path == \"/etc/local-custom/bar\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "myRC.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/rc-custom/foo\"",
											},
											{
												ID:         "bar3",
												Expression: "open.file.path == \"/etc/rc-custom/bar\"",
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Info: PolicyInfo{
							Name:         "myRC.policy",
							Source:       PolicyProviderTypeRC,
							InternalType: CustomPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "foo",
									Expression: "open.file.path == \"/etc/rc-custom/foo\"",
								},
								Policy: PolicyInfo{
									Name:         "myRC.policy",
									Source:       PolicyProviderTypeRC,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myRC.policy",
									Source:       PolicyProviderTypeRC,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
							{
								Def: &RuleDefinition{
									ID:         "bar3",
									Expression: "open.file.path == \"/etc/rc-custom/bar\"",
								},
								Policy: PolicyInfo{
									Name:         "myRC.policy",
									Source:       PolicyProviderTypeRC,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myRC.policy",
									Source:       PolicyProviderTypeRC,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
						},
					},
					{
						Info: PolicyInfo{
							Name:         "myLocal.policy",
							Source:       PolicyProviderTypeDir,
							InternalType: CustomPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "foo",
									Expression: "open.file.path == \"/etc/local-custom/foo\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
							{
								Def: &RuleDefinition{
									ID:         "bar",
									Expression: "open.file.path == \"/etc/local-custom/bar\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
						},
					},
				}

				defaultPolicyCount, _ := numAndLastIdxOfDefaultPolicies(expectedLoadedPolicies)
				assert.Equalf(t, 0, defaultPolicyCount, "The count of default policies do not match")

				if !cmp.Equal(expectedLoadedPolicies, got, policyCmpOpts...) {
					t.Errorf("The loaded policies do not match the expected\nDiff:\n%s", cmp.Diff(expectedLoadedPolicies, got, policyCmpOpts...))
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "Broken policy yaml file from RC → packaged policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "myLocal.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Version: "",
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/local-custom/foo\"",
											},
											{
												ID:         "bar",
												Expression: "open.file.path == \"/etc/local-custom/bar\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							var errs *multierror.Error

							errs = multierror.Append(errs, &ErrPolicyLoad{Name: "myRC.policy", Source: PolicyProviderTypeRC, Err: errors.New(`yaml: unmarshal error`)})
							return nil, errs
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Info: PolicyInfo{
							Name:         "myLocal.policy",
							Source:       PolicyProviderTypeDir,
							InternalType: CustomPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "foo",
									Expression: "open.file.path == \"/etc/local-custom/foo\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
							{
								Def: &RuleDefinition{
									ID:         "bar",
									Expression: "open.file.path == \"/etc/local-custom/bar\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
						},
					},
				}

				defaultPolicyCount, _ := numAndLastIdxOfDefaultPolicies(expectedLoadedPolicies)
				assert.Equalf(t, 0, defaultPolicyCount, "The count of default policies do not match")

				if !cmp.Equal(expectedLoadedPolicies, got, policyCmpOpts...) {
					t.Errorf("The loaded policies do not match the expected\nDiff:\n%s", cmp.Diff(expectedLoadedPolicies, got, policyCmpOpts...))
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Equal(t, err, &multierror.Error{Errors: []error{
					&ErrPolicyLoad{Name: "myRC.policy", Source: PolicyProviderTypeRC, Err: errors.New(`yaml: unmarshal error`)},
				}}, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "Empty RC policy yaml file → local policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "myLocal.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Version: "",
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/local-custom/foo\"",
											},
											{
												ID:         "bar",
												Expression: "open.file.path == \"/etc/local-custom/bar\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							var errs *multierror.Error

							errs = multierror.Append(errs, &ErrPolicyLoad{Name: "myRC.policy", Source: PolicyProviderTypeRC, Err: errors.New(`EOF`)})
							return nil, errs
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Info: PolicyInfo{
							Name:         "myLocal.policy",
							Source:       PolicyProviderTypeDir,
							InternalType: CustomPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "foo",
									Expression: "open.file.path == \"/etc/local-custom/foo\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
							{
								Def: &RuleDefinition{
									ID:         "bar",
									Expression: "open.file.path == \"/etc/local-custom/bar\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
						},
					},
				}

				if !cmp.Equal(expectedLoadedPolicies, got, policyCmpOpts...) {
					t.Errorf("The loaded policies do not match the expected\nDiff:\n%s", cmp.Diff(expectedLoadedPolicies, got, policyCmpOpts...))
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Equal(t, err, &multierror.Error{
					Errors: []error{
						&ErrPolicyLoad{Name: "myRC.policy", Source: PolicyProviderTypeRC, Err: errors.New(`EOF`)},
					}})
			},
		},
		{
			name: "Empty rules → packaged policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "myLocal.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Version: "",
										Rules: []*RuleDefinition{
											{
												ID:         "foo",
												Expression: "open.file.path == \"/etc/local-custom/foo\"",
											},
											{
												ID:         "bar",
												Expression: "open.file.path == \"/etc/local-custom/bar\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "myRC.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Version: "",
										Rules:   nil,
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Info: PolicyInfo{
							Name:         "myRC.policy",
							Source:       PolicyProviderTypeRC,
							InternalType: CustomPolicyType,
						},
					},
					{
						Info: PolicyInfo{
							Name:         "myLocal.policy",
							Source:       PolicyProviderTypeDir,
							InternalType: CustomPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "foo",
									Expression: "open.file.path == \"/etc/local-custom/foo\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
							{
								Def: &RuleDefinition{
									ID:         "bar",
									Expression: "open.file.path == \"/etc/local-custom/bar\"",
								},
								Policy: PolicyInfo{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "myLocal.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
						},
					},
				}

				if !cmp.Equal(expectedLoadedPolicies, got, policyCmpOpts...) {
					t.Errorf("The loaded policies do not match the expected\nDiff:\n%s", cmp.Diff(expectedLoadedPolicies, got, policyCmpOpts...))
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
	}

	overridesTestCases := []struct {
		name   string
		fields fields
		args   args
		want   func(t assert.TestingT, got map[eval.RuleID]*Rule, msgs ...interface{}) bool
	}{
		{
			name: "P0.DR enabled, P1.DR enabled => P0.DR enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"",
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
							Policy: PolicyInfo{
								Name:         DefaultPolicyName,
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         DefaultPolicyName,
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR enabled, P1.DR disabled => P0.DR enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
							Policy: PolicyInfo{
								Name:         DefaultPolicyName,
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         DefaultPolicyName,
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)

			},
		},
		{
			name: "P0.DR disabled, P1.DR enabled => P1.DR enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"", Disabled: false},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR disabled, P1.DR disabled => P0.DR disabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR enabled, P1.CR disabled => P1.CR disabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR0 enabled, P0.DR1 enabled, P1.CR0 disabled, P1.CR1 disabled => P1.CR0 disabled, P1.CR1 disabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
											{ID: "rule_2", Expression: "exec.file.path == \"/etc/default/bar\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"", Disabled: true},
											{ID: "rule_2", Expression: "exec.file.path == \"/etc/default/bar\"", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR0 enabled, P1.DR0 enabled, P2.CR0 disabled => P0.DR0 enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P0.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
											{ID: "rule_2", Expression: "exec.file.path == \"/etc/default/bar\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
											{ID: "rule_2", Expression: "exec.file.path == \"/etc/default/bar\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P2.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_2", Expression: "exec.file.path == \"/etc/default/bar\"", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
							Policy: PolicyInfo{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							}},
							Accepted: true,
						},
					},
					"rule_2": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_2", Expression: "exec.file.path == \"/etc/default/bar\""},
							Policy: PolicyInfo{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR0 enabled, P1.DR0 enabled, P2.CR0 disabled, P3.CR0 disabled => R0 disabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P0.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
											{ID: "rule_2", Expression: "exec.file.path == \"/etc/default/bar\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P2.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P3.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_2": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_2", Expression: "exec.file.path == \"/etc/default/bar\""},
							Policy: PolicyInfo{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR0 disabled, P1.DR0 disabled, P2.CR0 enabled => P2.CRO enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P0.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P2.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
							Policy: PolicyInfo{
								Name:         "P2.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P2.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR disabled, P1.CR disabled => CR disabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR disabled, P1.CR enabled => P1.CR enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\"", Disabled: false},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\""},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR enabled, P1.CR enabled => P0.CR enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
							Policy: PolicyInfo{
								Name:         DefaultPolicyName,
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         DefaultPolicyName,
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							}},
							Accepted: true,
						},
					},
				}

				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.CR enabled, P1.CR enabled => P0.CR enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P0.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\""},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\""},
							Policy: PolicyInfo{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}

				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.CR disabled, P1.CR enabled => P1.CR enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P0.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\"", Disabled: false},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\""},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.CR enabled, P1.CR disabled => P0.CR enabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P0.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\"", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\""},
							Policy: PolicyInfo{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.CR disabled, P1.CR disabled => P0.CR disabled",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P0.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Disabled: true},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Disabled: true},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR no action , P1.DR 1 action => P1.DR + 1 action",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\""},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"",
												Combine: OverridePolicy,
												OverrideOptions: OverrideOptions{
													Fields: []OverrideField{OverrideActionFields},
												},
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR2",
														},
													},
												},
											},
										},
									},
								}})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{
								ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"",
								Combine: OverridePolicy,
								Actions: []*ActionDefinition{
									{
										Kill: &KillDefinition{
											Signal: "SIGUSR2",
										},
									},
								},
							},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: DefaultPolicyType,
							}},
							Accepted: true,
						},
					},
				}

				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR 1 action, P1.CR 1 action => P1.CR + 2 actions",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR1",
														},
													},
												},
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Combine:    OverridePolicy,
												OverrideOptions: OverrideOptions{
													Fields: []OverrideField{OverrideActionFields},
												},
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR2",
														},
													},
												},
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{
								ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"",
								Combine: OverridePolicy,
								Actions: []*ActionDefinition{
									{
										Kill: &KillDefinition{
											Signal: "SIGUSR1",
										},
									},
									{
										Kill: &KillDefinition{
											Signal: "SIGUSR2",
										},
									},
								},
							},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR 1 action, P1.CR no action => P1.DR 1 action",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR1",
														},
													},
												},
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Combine:    OverridePolicy,
												OverrideOptions: OverrideOptions{
													Fields: []OverrideField{OverrideActionFields},
												},
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{
								ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"",
								Combine: OverridePolicy,
								Actions: []*ActionDefinition{
									{
										Kill: &KillDefinition{
											Signal: "SIGUSR1",
										},
									},
								},
							},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.CR 1 action, P1.CR no action => P1.CR + 1 action",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P0.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/custom/foo\"",
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR1",
														},
													},
												},
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/custom/foo\"",
												Combine:    OverridePolicy,
												OverrideOptions: OverrideOptions{
													Fields: []OverrideField{OverrideActionFields},
												},
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{
								ID: "rule_1", Expression: "exec.file.path == \"/etc/custom/foo\"",
								Combine: OverridePolicy,
								Actions: []*ActionDefinition{
									{
										Kill: &KillDefinition{
											Signal: "SIGUSR1",
										},
									},
								},
							},
							Policy: PolicyInfo{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P0.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR disabled, P1.CR 1, enabled, P2 1, disabled => P1.CR",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Disabled:   true,
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Disabled:   false,
											},
										},
									},
								},
								{
									name:       "P2.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Disabled:   true,
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{
								ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"",
							},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR enabled, P1.CR 1 actionA, enabled, P2 1 actionB, disabled => P1.CR + 1 actionA",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Disabled:   false,
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR1",
														},
													},
												},
												Combine: OverridePolicy,
												OverrideOptions: OverrideOptions{
													Fields: []OverrideField{OverrideActionFields},
												},
											},
										},
									},
								},
								{
									name:       "P2.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Disabled:   true,
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR2",
														},
													},
												},
												Combine: OverridePolicy,
												OverrideOptions: OverrideOptions{
													Fields: []OverrideField{OverrideActionFields},
												},
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{
								ID: "rule_1", Expression: "exec.file.path == \"/etc/default/foo\"",
								Actions: []*ActionDefinition{
									{
										Kill: &KillDefinition{
											Signal: "SIGUSR1",
										},
									},
								},
								Combine: OverridePolicy,
							},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
		{
			name: "P0.DR enabled 1 Action A, P1.CR  same Action A + Action B => P1.CR + 1 Action A (not duplicated) + Action B",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							policies, _ := testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       DefaultPolicyName,
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR1",
														},
													},
												},
											},
										},
									},
								},
								{
									name:       "P1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule_1",
												Expression: "exec.file.path == \"/etc/default/foo\"",
												Combine:    OverridePolicy,
												OverrideOptions: OverrideOptions{
													Fields: []OverrideField{OverrideActionFields},
												},
												Actions: []*ActionDefinition{
													{
														Kill: &KillDefinition{
															Signal: "SIGUSR1",
														},
													}, {
														Kill: &KillDefinition{
															Signal: "SIGUSR2",
														},
													},
												},
											},
										},
									},
								},
							})
							return policies, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, got map[eval.RuleID]*Rule, _ ...interface{}) bool {
				expected := map[eval.RuleID]*Rule{
					"rule_1": {
						PolicyRule: &PolicyRule{
							Def: &RuleDefinition{
								ID:         "rule_1",
								Expression: "exec.file.path == \"/etc/default/foo\"",
								Combine:    OverridePolicy,
								Actions: []*ActionDefinition{
									{
										Kill: &KillDefinition{
											Signal: "SIGUSR1",
										},
									},
									{
										Kill: &KillDefinition{
											Signal: "SIGUSR2",
										},
									},
								},
							},
							Policy: PolicyInfo{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							},
							UsedBy: []PolicyInfo{{
								Name:         "P1.policy",
								Source:       PolicyProviderTypeRC,
								InternalType: CustomPolicyType,
							}},
							Accepted: true,
						},
					},
				}
				return checkOverrideResult(t, expected, got)
			},
		},
	}

	replacePolicyTestCases := []struct {
		name    string
		fields  fields
		args    args
		want    func(t assert.TestingT, got []*Policy, msgs ...interface{}) bool
		wantErr func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool
	}{
		{
			name: "RC Custom policy replaces RC Default policy by ReplacePolicyID",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "local-custom.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule1",
												Expression: "open.file.path == \"/etc/local/rule1\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "rc-default.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule2",
												Expression: "open.file.path == \"/etc/rc-default/rule2\"",
											},
										},
									},
								},
								{
									name:       "rc-custom.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										ReplacePolicyID: "rc-default.policy",
										Rules: []*RuleDefinition{
											{
												ID:         "rule3",
												Expression: "open.file.path == \"/etc/rc-custom/rule3\"",
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				expectedPolicies := []*Policy{
					{
						Info: PolicyInfo{
							Name:            "rc-custom.policy",
							Source:          PolicyProviderTypeRC,
							InternalType:    CustomPolicyType,
							ReplacePolicyID: "rc-default.policy",
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "rule3",
									Expression: "open.file.path == \"/etc/rc-custom/rule3\"",
								},
								Policy: PolicyInfo{
									Name:            "rc-custom.policy",
									Source:          PolicyProviderTypeRC,
									InternalType:    CustomPolicyType,
									ReplacePolicyID: "rc-default.policy",
								},
								UsedBy: []PolicyInfo{{
									Name:            "rc-custom.policy",
									Source:          PolicyProviderTypeRC,
									InternalType:    CustomPolicyType,
									ReplacePolicyID: "rc-default.policy",
								}},
								Accepted: true,
							},
						},
					},
					{
						Info: PolicyInfo{
							Name:         "local-custom.policy",
							Source:       PolicyProviderTypeDir,
							InternalType: CustomPolicyType,
						},
						Rules: []*PolicyRule{
							{
								Def: &RuleDefinition{
									ID:         "rule1",
									Expression: "open.file.path == \"/etc/local/rule1\"",
								},
								Policy: PolicyInfo{
									Name:         "local-custom.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								},
								UsedBy: []PolicyInfo{{
									Name:         "local-custom.policy",
									Source:       PolicyProviderTypeDir,
									InternalType: CustomPolicyType,
								}},
								Accepted: true,
							},
						},
					},
				}

				if len(got) != len(expectedPolicies) {
					t.Errorf("Expected %d policies, got %d", len(expectedPolicies), len(got))
					return false
				}

				// Check that rc-default.policy was replaced and no longer exists
				for _, policy := range got {
					if policy.Info.Name == "rc-default.policy" {
						t.Errorf("Policy 'rc-default.policy' should have been replaced but still exists")
						return false
					}
				}

				if !cmp.Equal(expectedPolicies, got, policyCmpOpts...) {
					t.Errorf("The loaded policies do not match the expected\nDiff:\n%s", cmp.Diff(expectedPolicies, got, policyCmpOpts...))
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "RC Custom policy with ReplacePolicyID but target RC default policy doesn't exist",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "existing-local.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule1",
												Expression: "open.file.path == \"/etc/local/rule1\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "rc-custom.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										ReplacePolicyID: "non-existent-rc-default.policy",
										Rules: []*RuleDefinition{
											{
												ID:         "rule2",
												Expression: "open.file.path == \"/etc/rc/rule2\"",
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				// Should have both policies since the target RC default policy doesn't exist
				if len(got) != 2 {
					t.Errorf("Expected 2 policies, got %d", len(got))
					return false
				}

				policyNames := make(map[string]bool)
				for _, policy := range got {
					policyNames[policy.Info.Name] = true
				}

				if !policyNames["existing-local.policy"] {
					t.Errorf("Expected 'existing-local.policy' to be present")
					return false
				}

				if !policyNames["rc-custom.policy"] {
					t.Errorf("Expected 'rc-custom.policy' to be present")
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "Multiple RC Custom policies with different ReplacePolicyIDs targeting RC Default policies",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "keep-local.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule3",
												Expression: "open.file.path == \"/etc/keep/rule3\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "rc-default-1.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule1",
												Expression: "open.file.path == \"/etc/rc-default1/rule1\"",
											},
										},
									},
								},
								{
									name:       "rc-default-2.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule2",
												Expression: "open.file.path == \"/etc/rc-default2/rule2\"",
											},
										},
									},
								},
								{
									name:       "rc-custom-1.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										ReplacePolicyID: "rc-default-1.policy",
										Rules: []*RuleDefinition{
											{
												ID:         "rule4",
												Expression: "open.file.path == \"/etc/rc-custom1/rule4\"",
											},
										},
									},
								},
								{
									name:       "rc-custom-2.policy",
									source:     PolicyProviderTypeRC,
									policyType: CustomPolicyType,
									def: PolicyDef{
										ReplacePolicyID: "rc-default-2.policy",
										Rules: []*RuleDefinition{
											{
												ID:         "rule5",
												Expression: "open.file.path == \"/etc/rc-custom2/rule5\"",
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				if len(got) != 3 {
					t.Errorf("Expected 3 policies, got %d", len(got))
					return false
				}

				policyNames := make(map[string]bool)
				for _, policy := range got {
					policyNames[policy.Info.Name] = true
				}

				// Should have the RC custom policies and the local policy
				expectedPolicies := []string{"rc-custom-1.policy", "rc-custom-2.policy", "keep-local.policy"}
				for _, expected := range expectedPolicies {
					if !policyNames[expected] {
						t.Errorf("Expected policy '%s' to be present", expected)
						return false
					}
				}

				// Should not have the replaced RC default policies
				replacedPolicies := []string{"rc-default-1.policy", "rc-default-2.policy"}
				for _, replaced := range replacedPolicies {
					if policyNames[replaced] {
						t.Errorf("Policy '%s' should have been replaced but still exists", replaced)
						return false
					}
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "ReplacePolicyID from non-RC provider should be ignored",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "local-with-replace.policy",
									source:     PolicyProviderTypeDir,
									policyType: CustomPolicyType,
									def: PolicyDef{
										ReplacePolicyID: "rc-default.policy", // This should be ignored since it's not from RC
										Rules: []*RuleDefinition{
											{
												ID:         "rule2",
												Expression: "open.file.path == \"/etc/local-replace/rule2\"",
											},
										},
									},
								},
							})
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "rc-default.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule1",
												Expression: "open.file.path == \"/etc/rc-default/rule1\"",
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				if len(got) != 2 {
					t.Errorf("Expected 2 policies, got %d", len(got))
					return false
				}

				policyNames := make(map[string]bool)
				for _, policy := range got {
					policyNames[policy.Info.Name] = true
				}

				// Both policies should still exist since ReplacePolicyID is ignored for non-RC
				if !policyNames["rc-default.policy"] {
					t.Errorf("Expected 'rc-default.policy' to be present (should not be replaced by local policy)")
					return false
				}

				if !policyNames["local-with-replace.policy"] {
					t.Errorf("Expected 'local-with-replace.policy' to be present")
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "RC Default policy with ReplacePolicyID should be ignored",
			fields: fields{
				Providers: []PolicyProvider{
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return testPoliciesToPolicies([]*testPolicyDef{
								{
									name:       "rc-default-target.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										Rules: []*RuleDefinition{
											{
												ID:         "rule1",
												Expression: "open.file.path == \"/etc/rc-target/rule1\"",
											},
										},
									},
								},
								{
									name:       "rc-default-with-replace.policy",
									source:     PolicyProviderTypeRC,
									policyType: DefaultPolicyType,
									def: PolicyDef{
										ReplacePolicyID: "rc-default-target.policy", // This should be ignored since only custom policies can replace
										Rules: []*RuleDefinition{
											{
												ID:         "rule2",
												Expression: "open.file.path == \"/etc/rc-default-replace/rule2\"",
											},
										},
									},
								},
							})
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				if len(got) != 2 {
					t.Errorf("Expected 2 policies, got %d", len(got))
					return false
				}

				policyNames := make(map[string]bool)
				for _, policy := range got {
					policyNames[policy.Info.Name] = true
				}

				// Both policies should exist since default policies can't use ReplacePolicyID
				if !policyNames["rc-default-target.policy"] {
					t.Errorf("Expected 'rc-default-target.policy' to be present (should not be replaced)")
					return false
				}

				if !policyNames["rc-default-with-replace.policy"] {
					t.Errorf("Expected 'rc-default-with-replace.policy' to be present")
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PolicyLoader{
				Providers: tt.fields.Providers,
			}
			loadedPolicies, errs := p.LoadPolicies(tt.args.opts)

			tt.want(t, loadedPolicies)
			tt.wantErr(t, errs)
		})
	}

	for _, tt := range overridesTestCases {
		t.Run(tt.name, func(t *testing.T) {
			ruleOpts, evalOpts := NewBothOpts(map[eval.EventType]bool{"*": true})
			rs := NewRuleSet(&model.Model{}, func() eval.Event { return model.NewFakeEvent() }, ruleOpts, evalOpts)
			p := &PolicyLoader{
				Providers: tt.fields.Providers,
			}
			rs.LoadPolicies(p, tt.args.opts)
			tt.want(t, rs.GetRuleMap())
		})
	}

	for _, tt := range replacePolicyTestCases {
		t.Run(tt.name, func(t *testing.T) {
			p := &PolicyLoader{
				Providers: tt.fields.Providers,
			}
			loadedPolicies, errs := p.LoadPolicies(tt.args.opts)

			tt.want(t, loadedPolicies)
			tt.wantErr(t, errs)
		})
	}

}

// Utils
func numAndLastIdxOfDefaultPolicies(policies []*Policy) (int, int) {
	var defaultPolicyCount int
	var lastSeenDefaultPolicyIdx int
	for idx, policy := range policies {
		if policy.Info.Name == DefaultPolicyName {
			defaultPolicyCount++
			lastSeenDefaultPolicyIdx = idx
		}
	}

	return defaultPolicyCount, lastSeenDefaultPolicyIdx
}

type dummyDirProvider struct {
	dummyLoadPoliciesFunc func() ([]*Policy, *multierror.Error)
}

func (d dummyDirProvider) LoadPolicies(_ []MacroFilter, _ []RuleFilter) ([]*Policy, *multierror.Error) {
	return d.dummyLoadPoliciesFunc()
}

func (dummyDirProvider) SetOnNewPoliciesReadyCb(_ func(silent bool)) {}

func (dummyDirProvider) Start() {}

func (dummyDirProvider) Close() error {
	return nil
}

func (dummyDirProvider) Type() string {
	return PolicyProviderTypeDir
}

type dummyRCProvider struct {
	dummyLoadPoliciesFunc func() ([]*Policy, *multierror.Error)
}

func (d dummyRCProvider) LoadPolicies(_ []MacroFilter, _ []RuleFilter) ([]*Policy, *multierror.Error) {
	return d.dummyLoadPoliciesFunc()
}

func (dummyRCProvider) SetOnNewPoliciesReadyCb(_ func(silent bool)) {}

func (dummyRCProvider) Start() {}

func (dummyRCProvider) Close() error {
	return nil
}

func (dummyRCProvider) Type() string {
	return PolicyProviderTypeRC
}

type testPolicyDef struct {
	def        PolicyDef
	name       string
	source     string
	policyType InternalPolicyType
}

func testPolicyToPolicy(testPolicy *testPolicyDef) (*Policy, *multierror.Error) {
	info := &PolicyInfo{
		Name:         testPolicy.name,
		Source:       testPolicy.source,
		InternalType: testPolicy.policyType,
	}
	policy, err := LoadPolicyFromDefinition(info, &testPolicy.def, nil, nil)
	if err != nil {
		return nil, multierror.Append(nil, err)
	}
	return policy, nil
}

func testPoliciesToPolicies(testPolicies []*testPolicyDef) ([]*Policy, *multierror.Error) {
	var policies []*Policy
	var errs *multierror.Error

	for _, testPolicy := range testPolicies {
		p, err := testPolicyToPolicy(testPolicy)
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}

		policies = append(policies, p)
	}

	return policies, errs
}

func checkOverrideResult(t assert.TestingT, expected map[eval.RuleID]*Rule, got map[eval.RuleID]*Rule) bool {
	assert.Equal(t, len(expected), len(got))

	for ruleID, r := range expected {
		res := assert.NotNil(t, got[ruleID]) &&
			assert.Equal(t, r.PolicyRule.Def, got[ruleID].PolicyRule.Def) &&
			assert.Equal(t, r.PolicyRule.Policy.Name, got[ruleID].Policy.Name) &&
			assert.Equal(t, r.PolicyRule.Policy.Source, got[ruleID].Policy.Source) &&
			assert.Equal(t, r.PolicyRule.Policy.InternalType, got[ruleID].Policy.InternalType) &&
			assert.Equal(t, r.PolicyRule.Accepted, got[ruleID].PolicyRule.Accepted)
		if !res {
			return res
		}
	}
	return true
}
