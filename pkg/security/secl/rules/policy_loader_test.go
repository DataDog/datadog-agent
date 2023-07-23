// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"testing"
)

type dummyDirProvider struct {
	dummyLoadPoliciesFunc func() ([]*Policy, *multierror.Error)
}

func (d dummyDirProvider) LoadPolicies(_ []MacroFilter, _ []RuleFilter) ([]*Policy, *multierror.Error) {
	return d.dummyLoadPoliciesFunc()
}

func (dummyDirProvider) SetOnNewPoliciesReadyCb(f func()) {
	return
}

func (dummyDirProvider) Start() {
	return
}

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

func (dummyRCProvider) SetOnNewPoliciesReadyCb(f func()) {
	return
}

func (dummyRCProvider) Start() {
	return
}

func (dummyRCProvider) Close() error {
	return nil
}

func (dummyRCProvider) Type() string {
	return PolicyProviderTypeRC
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
		name                   string
		fields                 fields
		args                   args
		want                   []*Policy
		wantErrors             *multierror.Error
		wantNumDefaultPolicies int
	}{
		{
			name: "RC Default replaces Local Default, and RC Custom overrides Local Custom",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myLocal.policy",
								Source: PolicySourceDir,
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
								Source: PolicySourceDir,
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
								Source: PolicySourceRC,
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
								Source: PolicySourceRC,
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
			want: []*Policy{
				{
					Name:   DefaultPolicyName,
					Source: PolicySourceRC,
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
					Source: PolicySourceRC,
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
					Source:  PolicySourceDir,
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
			},
			wantErrors:             nil,
			wantNumDefaultPolicies: 1,
		},
		{
			name: "No default policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "myLocal.policy",
								Source: PolicySourceDir,
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
								Source: PolicySourceRC,
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
			want: []*Policy{
				{
					Name:   "myRC.policy",
					Source: PolicySourceRC,
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
					Source: PolicySourceDir,
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
			wantErrors:             nil,
			wantNumDefaultPolicies: 0,
		},
		{
			name: "Broken policy yaml file from RC → packaged policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "myLocal.policy",
								Source:  PolicySourceDir,
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
			want: []*Policy{
				{
					Name:    "myLocal.policy",
					Source:  PolicySourceDir,
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
			},
			wantErrors: &multierror.Error{Errors: []error{
				&ErrPolicyLoad{Name: "myRC.policy", Err: fmt.Errorf(`yaml: unmarshal error`)},
			}},
			wantNumDefaultPolicies: 0,
		},
		{
			name: "Empty RC policy yaml file → local policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "myLocal.policy",
								Source:  PolicySourceDir,
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
			want: []*Policy{
				{
					Name:    "myLocal.policy",
					Source:  PolicySourceDir,
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
			},
			wantErrors: &multierror.Error{Errors: []error{
				&ErrPolicyLoad{Name: "myRC.policy", Err: fmt.Errorf(`EOF`)},
			}},
			wantNumDefaultPolicies: 0,
		},
		{
			name: "Empty rules → packaged policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:    "myLocal.policy",
								Source:  PolicySourceDir,
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
								Source: PolicySourceRC,
								Rules:  nil,
							}}, nil
						},
					},
				},
			},
			want: []*Policy{
				{
					Name:   "myRC.policy",
					Source: PolicySourceRC,
					Rules:  nil, // TODO: Ensure this doesn't cause a problem with loading rules
				},
				{
					Name:    "myLocal.policy",
					Source:  PolicySourceDir,
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
			},
			wantErrors:             nil,
			wantNumDefaultPolicies: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PolicyLoader{
				Providers: tt.fields.Providers,
			}
			loadedPolicies, errs := p.LoadPolicies(tt.args.opts)

			var defaultPolicyCount int
			var lastSeenDefaultPolicyIdx int
			for idx, policy := range loadedPolicies {
				if policy.Name == DefaultPolicyName {
					defaultPolicyCount++
					lastSeenDefaultPolicyIdx = idx
				}
			}

			assert.Equalf(t, tt.wantNumDefaultPolicies, defaultPolicyCount, "There are more than 1 default policies")
			assert.Equalf(t, PolicySourceRC, loadedPolicies[lastSeenDefaultPolicyIdx].Source, "The default policy is not from RC")

			assert.Equalf(t, tt.want, loadedPolicies, "The loaded policies do not match the expected")
			assert.Equalf(t, tt.wantErrors, errs, "The errors do not match the expected")
		})
	}
}
