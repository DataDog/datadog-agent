// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
)

// compare all Policy fields but the `Def` field
var policyCmpOpts = []cmp.Option{
	cmp.AllowUnexported(Policy{}),
	cmpopts.IgnoreFields(Policy{}, "Def"),
}

// go test -v github.com/DataDog/datadog-agent/pkg/security/secl/rules --run="TestPolicyLoader_LoadPolicies"
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
									name:   "myLocal.policy",
									source: PolicyProviderTypeDir,
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
									name:   DefaultPolicyName,
									source: PolicyProviderTypeDir,
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
									name:   "myRC.policy",
									source: PolicyProviderTypeRC,
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
									name:   DefaultPolicyName,
									source: PolicyProviderTypeRC,
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
				expectedLoadedPolicies := fixupPoliciesRulesPolicy([]*Policy{
					{
						Name:   DefaultPolicyName,
						Source: PolicyProviderTypeRC,
						macros: map[string][]*PolicyMacro{},
						rules: map[string][]*PolicyRule{
							"foo": {
								{
									Def: &RuleDefinition{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-default/foo\"",
									},
									Accepted: true,
								},
							},
							"bravo": {
								{
									Def: &RuleDefinition{
										ID:         "bravo",
										Expression: "open.file.path == \"/etc/rc-default/bravo\"",
									},
									Accepted: true,
								},
							},
						},
					},
					{
						Name:   "myRC.policy",
						Source: PolicyProviderTypeRC,
						macros: map[string][]*PolicyMacro{},
						rules: map[string][]*PolicyRule{
							"foo": {
								{
									Def: &RuleDefinition{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-custom/foo\"",
									},
									Accepted: true,
								},
							},
							"alpha": {
								{
									Def: &RuleDefinition{
										ID:         "alpha",
										Expression: "open.file.path == \"/etc/rc-custom/alpha\"",
									},
									Accepted: true,
								},
							},
						},
					},
					{
						Name:   "myLocal.policy",
						Source: PolicyProviderTypeDir,
						macros: map[string][]*PolicyMacro{},
						rules: map[string][]*PolicyRule{
							"foo": {
								{
									Def: &RuleDefinition{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-custom/foo\"",
									},
									Accepted: true,
								},
							},
							"bar": {
								{
									Def: &RuleDefinition{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-custom/bar\"",
									},
									Accepted: true,
								},
							},
						},
					},
				})

				defaultPolicyCount, lastSeenDefaultPolicyIdx := numAndLastIdxOfDefaultPolicies(expectedLoadedPolicies)

				assert.Equalf(t, 1, defaultPolicyCount, "There are more than 1 default policies")
				assert.Equalf(t, PolicyProviderTypeRC, got[lastSeenDefaultPolicyIdx].Source, "The default policy is not from RC")

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
									name:   "myLocal.policy",
									source: PolicyProviderTypeDir,
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
									name:   "myRC.policy",
									source: PolicyProviderTypeRC,
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
				expectedLoadedPolicies := fixupPoliciesRulesPolicy([]*Policy{
					{
						Name:   "myRC.policy",
						Source: PolicyProviderTypeRC,
						macros: map[string][]*PolicyMacro{},
						rules: map[string][]*PolicyRule{
							"foo": {
								{
									Def: &RuleDefinition{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/rc-custom/foo\"",
									},
									Accepted: true,
								},
							},
							"bar3": {
								{
									Def: &RuleDefinition{
										ID:         "bar3",
										Expression: "open.file.path == \"/etc/rc-custom/bar\"",
									},
									Accepted: true,
								},
							},
						},
					},
					{
						Name:   "myLocal.policy",
						Source: PolicyProviderTypeDir,
						macros: map[string][]*PolicyMacro{},
						rules: map[string][]*PolicyRule{
							"foo": {
								{
									Def: &RuleDefinition{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-custom/foo\"",
									},
									Accepted: true,
								},
							},
							"bar": {
								{
									Def: &RuleDefinition{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-custom/bar\"",
									},
									Accepted: true,
								},
							},
						},
					},
				})

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
									name:   "myLocal.policy",
									source: PolicyProviderTypeDir,
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

							errs = multierror.Append(errs, &ErrPolicyLoad{Name: "myRC.policy", Err: fmt.Errorf(`yaml: unmarshal error`)})
							return nil, errs
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				expectedLoadedPolicies := fixupPoliciesRulesPolicy([]*Policy{
					{
						Name:   "myLocal.policy",
						Source: PolicyProviderTypeDir,
						macros: map[string][]*PolicyMacro{},
						rules: map[string][]*PolicyRule{
							"foo": {
								{
									Def: &RuleDefinition{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-custom/foo\"",
									},
									Accepted: true,
								},
							},
							"bar": {
								{
									Def: &RuleDefinition{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-custom/bar\"",
									},
									Accepted: true,
								},
							},
						},
					},
				})

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
					&ErrPolicyLoad{Name: "myRC.policy", Err: fmt.Errorf(`yaml: unmarshal error`)},
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
									name:   "myLocal.policy",
									source: PolicyProviderTypeDir,
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

							errs = multierror.Append(errs, &ErrPolicyLoad{Name: "myRC.policy", Err: fmt.Errorf(`EOF`)})
							return nil, errs
						},
					},
				},
			},
			want: func(t assert.TestingT, got []*Policy, _ ...interface{}) bool {
				expectedLoadedPolicies := fixupPoliciesRulesPolicy([]*Policy{
					{
						Name:   "myLocal.policy",
						Source: PolicyProviderTypeDir,
						macros: map[string][]*PolicyMacro{},
						rules: map[string][]*PolicyRule{
							"foo": {
								{
									Def: &RuleDefinition{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-custom/foo\"",
									},
									Accepted: true,
								},
							},
							"bar": {
								{
									Def: &RuleDefinition{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-custom/bar\"",
									},
									Accepted: true,
								},
							},
						},
					},
				})

				if !cmp.Equal(expectedLoadedPolicies, got, policyCmpOpts...) {
					t.Errorf("The loaded policies do not match the expected\nDiff:\n%s", cmp.Diff(expectedLoadedPolicies, got, policyCmpOpts...))
					return false
				}

				return true
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, _ ...interface{}) bool {
				return assert.Equal(t, err, &multierror.Error{
					Errors: []error{
						&ErrPolicyLoad{Name: "myRC.policy", Err: fmt.Errorf(`EOF`)},
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
									name:   "myLocal.policy",
									source: PolicyProviderTypeDir,
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
									name:   "myRC.policy",
									source: PolicyProviderTypeRC,
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
				expectedLoadedPolicies := fixupPoliciesRulesPolicy([]*Policy{
					{
						Name:   "myRC.policy",
						Source: PolicyProviderTypeRC,
						macros: map[string][]*PolicyMacro{},
						rules:  map[string][]*PolicyRule{},
					},
					{
						Name:   "myLocal.policy",
						Source: PolicyProviderTypeDir,
						macros: map[string][]*PolicyMacro{},
						rules: map[string][]*PolicyRule{
							"foo": {
								{
									Def: &RuleDefinition{
										ID:         "foo",
										Expression: "open.file.path == \"/etc/local-custom/foo\"",
									},
									Accepted: true,
								},
							},
							"bar": {
								{
									Def: &RuleDefinition{
										ID:         "bar",
										Expression: "open.file.path == \"/etc/local-custom/bar\"",
									},
									Accepted: true,
								},
							},
						},
					},
				})

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
}

// Utils

func numAndLastIdxOfDefaultPolicies(policies []*Policy) (int, int) {
	var defaultPolicyCount int
	var lastSeenDefaultPolicyIdx int
	for idx, policy := range policies {
		if policy.Name == DefaultPolicyName {
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

func (dummyDirProvider) SetOnNewPoliciesReadyCb(_ func()) {}

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

func (dummyRCProvider) SetOnNewPoliciesReadyCb(_ func()) {}

func (dummyRCProvider) Start() {}

func (dummyRCProvider) Close() error {
	return nil
}

func (dummyRCProvider) Type() string {
	return PolicyProviderTypeRC
}

type testPolicyDef struct {
	def    PolicyDef
	name   string
	source string
}

func testPolicyToPolicy(testPolicy *testPolicyDef) (*Policy, *multierror.Error) {
	policy, err := LoadPolicyFromDefinition(testPolicy.name, testPolicy.source, &testPolicy.def, nil, nil)
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

func fixupRulesPolicy(policy *Policy) *Policy {
	for _, rules := range policy.rules {
		for _, rule := range rules {
			rule.Policy = policy
		}
	}
	return policy
}

func fixupPoliciesRulesPolicy(policies []*Policy) []*Policy {
	for _, policy := range policies {
		fixupRulesPolicy(policy)
	}
	return policies
}
