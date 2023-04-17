// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package rules

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"testing"
)

func newEvaluationSet(tagValues []eval.RuleSetTagValue) (*EvaluationSet, error) {
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

	evaluationSet, _ := newEvaluationSet([]eval.RuleSetTagValue{})
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

	evaluationSet, _ := newEvaluationSet(tagValues)
	return loader, evaluationSet
}

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
					DefaultRuleSetTagValue: &RuleSet{
						policies: []*Policy{
							&Policy{Name: "policy 1"},
							&Policy{Name: "policy 2"},
						}},
					"threat_score": &RuleSet{
						policies: []*Policy{
							&Policy{Name: "policy 3"},
							&Policy{Name: "policy 2"},
						}},
					"special": &RuleSet{
						policies: []*Policy{
							&Policy{Name: "policy 3"},
							&Policy{Name: "policy 2"},
						}},
				},
			},
			want: []*Policy{&Policy{Name: "policy 1"},
				&Policy{Name: "policy 2"}, &Policy{Name: "policy 3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &EvaluationSet{
				RuleSets: tt.fields.RuleSets,
			}
			assert.Equalf(t, tt.want, es.GetPolicies(), "GetPolicies()")
		})
	}
}

func TestEvaluationSet_LoadPolicies(t *testing.T) {
	type args struct {
		policy    *PolicyDef
		tagValues []eval.RuleSetTagValue
		opts      PolicyLoaderOpts
	}
	tests := []struct {
		name    string
		args    args
		want    func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool
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
			want: func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool {
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
			want: func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool {
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
					},
				},
				tagValues: []eval.RuleSetTagValue{DefaultRuleSetTagValue, "threat_score", "special"},
			},
			want: func(t assert.TestingT, got *EvaluationSet, msgs ...interface{}) bool {
				gotNumProbeEvalRules := len(got.RuleSets[DefaultRuleSetTagValue].rules)
				expected := 1
				assert.Equal(t, expected, gotNumProbeEvalRules)

				gotNumThreatScoreRules := len(got.RuleSets["threat_score"].rules)
				expectedNumThreatScoreRules := 1
				assert.Equal(t, expectedNumThreatScoreRules, gotNumThreatScoreRules)

				gotNumSpecialRules := len(got.RuleSets["special"].rules)
				expectedNumSpecialRules := 1
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
			tt.want(t, es)
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
			name: "no rulesets",
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
			name: "multiple rulesets",
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
