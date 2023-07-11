package rules

import (
	"fmt"
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

func TestPolicyLoader_LoadPolicies(t *testing.T) {
	type fields struct {
		Providers []PolicyProvider
	}
	type args struct {
		opts PolicyLoaderOpts
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []*Policy
		want1  *multierror.Error
	}{
		{
			name: "RC default policy overrides packaged default policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "default.policy",
								Source: PolicySourceDir,
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "default.policy",
								Source: PolicySourceRC,
							}}, nil
						},
					},
				},
			},
			want: []*Policy{
				{
					Name:    "default.policy",
					Source:  "remote-config",
					Version: "",
					Rules:   nil,
					Macros:  nil,
				},
			},
			want1: nil,
		},
		{
			name: "No default policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "my.policy",
								Source: PolicySourceDir,
							}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{
								Name:   "my2.policy",
								Source: PolicySourceRC,
							}}, nil
						},
					},
				},
			},
			want: []*Policy{
				{
					Name:    "my2.policy",
					Source:  PolicySourceRC,
					Version: "",
					Rules:   nil,
					Macros:  nil,
				},
				{
					Name:    "my.policy",
					Source:  PolicySourceDir,
					Version: "",
					Rules:   nil,
					Macros:  nil,
				},
			},
			want1: nil,
		},
		//{
		//	name: "Scoped API key not present → packaged policy",
		//	fields: fields{
		//		Providers: []PolicyProvider{dummyDirProvider{}, dummyRCProvider{}},
		//	},
		//	want: []*Policy{
		//		{Name: "policy 1"},
		//		{Name: "policy 2"},
		//	},
		//	want1: nil,
		//},
		{
			name: "Broken policy yaml file from RC → packaged policy",
			fields: fields{
				Providers: []PolicyProvider{
					dummyDirProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							return []*Policy{{Name: "policy from directory"}}, nil
						},
					},
					dummyRCProvider{
						dummyLoadPoliciesFunc: func() ([]*Policy, *multierror.Error) {
							var errs *multierror.Error
							errs = multierror.Append(errs, ErrPolicyLoad{Err: fmt.Errorf("couldn't decode policy file")})

							return []*Policy{}, errs
						},
					}},
			},
			want: []*Policy{
				{
					Name:    "policy from directory",
					Source:  "",
					Version: "",
					Rules:   nil,
					Macros:  nil,
				},
			},
			want1: &multierror.Error{
				Errors: []error{
					ErrPolicyLoad{
						Name: "",
						Err:  fmt.Errorf("couldn't decode policy file"),
					},
				},
				ErrorFormat: nil,
			},
		},
		//{
		//	name: "Empty policy yaml file → packaged policy",
		//	fields: fields{
		//		Providers: []PolicyProvider{dummyDirProvider{}, dummyRCProvider{}},
		//	},
		//want: []*Policy{
		//	{
		//		Name:    "policy from directory",
		//		Source:  "",
		//		Version: "",
		//		Rules:   nil,
		//		Macros:  nil,
		//	},
		//	{
		//		Name:    "policy from RC",
		//		Source:  "",
		//		Version: "",
		//		Rules:   nil,
		//		Macros:  nil,
		//	},
		//},
		//	want1: nil,
		//},
		//{
		//	name: "Empty rules → packaged policy",
		//	fields: fields{
		//		Providers: []PolicyProvider{dummyDirProvider{}, dummyRCProvider{}},
		//	},
		//	want: []*Policy{
		//		{Name: "policy 1"},
		//		{Name: "policy 2"},
		//	},
		//	want1: nil,
		//},
		//{
		//	name: "Empty RC response → packaged policy",
		//	fields: fields{
		//		Providers: []PolicyProvider{dummyDirProvider{}, dummyRCProvider{}},
		//	},
		//	want: []*Policy{
		//		{Name: "policy 1"},
		//		{Name: "policy 2"},
		//	},
		//	want1: nil,
		//},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PolicyLoader{
				Providers: tt.fields.Providers,
			}
			got, got1 := p.LoadPolicies(tt.args.opts)
			assert.Equalf(t, tt.want, got, "LoadPolicies(%v)", tt.args.opts)
			assert.Equalf(t, tt.want1, got1, "LoadPolicies(%v)", tt.args.opts)
		})
	}
}
