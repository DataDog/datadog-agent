// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
)

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
		want    func(t assert.TestingT, fields fields, got []*Policy, msgs ...interface{}) bool
		wantErr func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool
	}{
		{
			name: "RC Default replaces Local Default, and RC Custom overrides Local Custom",
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
										ID:         "baz",
										Expression: "open.file.path == \"/etc/local-default/baz\"",
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
										ID:         "alpha",
										Expression: "open.file.path == \"/etc/rc-custom/alpha\"",
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
									{
										ID:         "bravo",
										Expression: "open.file.path == \"/etc/rc-default/bravo\"",
									},
								},
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got []*Policy, msgs ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Name:   DefaultPolicyName,
						Source: PolicyProviderTypeRC,
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
					{
						Name:   "myRC.policy",
						Source: PolicyProviderTypeRC,
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
					{
						Name:    "myLocal.policy",
						Source:  PolicyProviderTypeDir,
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
						Macros: nil,
					},
				}

				defaultPolicyCount, lastSeenDefaultPolicyIdx := numAndLastIdxOfDefaultPolicies(expectedLoadedPolicies)

				assert.Equalf(t, 1, defaultPolicyCount, "There are more than 1 default policies")
				assert.Equalf(t, PolicyProviderTypeRC, got[lastSeenDefaultPolicyIdx].Source, "The default policy is not from RC")

				return assert.Equalf(t, expectedLoadedPolicies, got, "The loaded policies do not match the expected")
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "No default policy",
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
										ID:         "bar3",
										Expression: "open.file.path == \"/etc/rc-custom/bar\"",
									},
								},
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got []*Policy, msgs ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Name:   "myRC.policy",
						Source: PolicyProviderTypeRC,
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
					{
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
						},
					},
				}

				defaultPolicyCount, _ := numAndLastIdxOfDefaultPolicies(expectedLoadedPolicies)

				assert.Equalf(t, 0, defaultPolicyCount, "The count of default policies do not match")
				return assert.Equalf(t, expectedLoadedPolicies, got, "The loaded policies do not match the expected")
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
				return assert.Nil(t, err, "Expected no errors but got %+v", err)
			},
		},
		{
			name: "Broken policy yaml file from RC → packaged policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "myLocal.policy",
								Source:  PolicyProviderTypeDir,
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
								Macros: nil,
							}}, nil
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
			want: func(t assert.TestingT, fields fields, got []*Policy, msgs ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Name:    "myLocal.policy",
						Source:  PolicyProviderTypeDir,
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
						Macros: nil,
					},
				}

				defaultPolicyCount, _ := numAndLastIdxOfDefaultPolicies(expectedLoadedPolicies)

				assert.Equalf(t, 0, defaultPolicyCount, "The count of default policies do not match")
				return assert.Equalf(t, expectedLoadedPolicies, got, "The loaded policies do not match the expected")
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
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
							return []*Policy{{
								Name:    "myLocal.policy",
								Source:  PolicyProviderTypeDir,
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
								Macros: nil,
							}}, nil
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
			want: func(t assert.TestingT, fields fields, got []*Policy, msgs ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Name:    "myLocal.policy",
						Source:  PolicyProviderTypeDir,
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
						Macros: nil,
					},
				}

				return assert.Equalf(t, expectedLoadedPolicies, got, "The loaded policies do not match the expected")
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
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
							return []*Policy{{
								Name:    "myLocal.policy",
								Source:  PolicyProviderTypeDir,
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
								Macros: nil,
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myRC.policy",
								Source: PolicyProviderTypeRC,
								Rules:  nil,
							}}, nil
						},
					},
				},
			},
			want: func(t assert.TestingT, fields fields, got []*Policy, msgs ...interface{}) bool {
				expectedLoadedPolicies := []*Policy{
					{
						Name:   "myRC.policy",
						Source: PolicyProviderTypeRC,
						Rules:  nil, // TODO: Ensure this doesn't cause a problem with loading rules
					},
					{
						Name:    "myLocal.policy",
						Source:  PolicyProviderTypeDir,
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
						Macros: nil,
					},
				}

				return assert.Equalf(t, expectedLoadedPolicies, got, "The loaded policies do not match the expected")
			},
			wantErr: func(t assert.TestingT, err *multierror.Error, msgs ...interface{}) bool {
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

			tt.want(t, tt.fields, loadedPolicies)
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

func (d dummyDirProvider) LoadPolicies(_ []MacroFilter, _ []RuleFilter, _ bool) ([]*Policy, *multierror.Error) {
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

func (d dummyRCProvider) LoadPolicies(_ []MacroFilter, _ []RuleFilter, _ bool) ([]*Policy, *multierror.Error) {
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
