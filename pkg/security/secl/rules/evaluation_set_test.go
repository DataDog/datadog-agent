package rules

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
	"testing"
)

func newEvaluationSet() (*EvaluationSet, error) {
	rs := newRuleSet()
	return NewEvaluationSet([]*RuleSet{rs})
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
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &EvaluationSet{
				RuleSets: tt.fields.RuleSets,
			}
			assert.Equalf(t, tt.want, ps.GetPolicies(), "GetPolicies()")
		})
	}
}

func TestEvaluationSet_LoadPolicies(t *testing.T) {
	type fields struct {
		RuleSets map[eval.RuleSetTagValue]*RuleSet
	}
	type args struct {
		loader *PolicyLoader
		opts   PolicyLoaderOpts
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *multierror.Error
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &EvaluationSet{
				RuleSets: tt.fields.RuleSets,
			}
			assert.Equalf(t, tt.want, es.LoadPolicies(tt.args.loader, tt.args.opts), "LoadPolicies(%v, %v)", tt.args.loader, tt.args.opts)
		})
	}
}

func TestNewEvaluationSet(t *testing.T) {
	ruleSet := newRuleSet()
	ruleSetWithThreatScoreTag := newRuleSet()
	ruleSetWithThreatScoreTag.SetRuleSetTagValue("threat_score")

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
