package rules

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRuleTagFilter(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "no_tag",
				Expression: `open.file.path == "/tmp/test"`,
			},
			{
				ID:         "default_tag",
				Expression: `open.file.path == "/tmp/test"`,
				Tags:       map[string]string{"ruleset": DefaultRuleSetTagValue},
			},
			{
				ID:         "threat_score",
				Expression: `open.file.path != "/tmp/test"`,
				Tags:       map[string]string{"ruleset": "threat_score"},
			},
			{
				ID:         "special",
				Expression: `open.file.path != "/tmp/test"`,
				Tags:       map[string]string{"ruleset": "special"},
			},
		},
	}

	policyOpts := PolicyLoaderOpts{
		RuleFilters: []RuleFilter{
			&RuleTagFilter{
				key:   "ruleset",
				value: "threat_score",
			},
		},
	}

	es, err := loadPolicyIntoProbeEvaluationRuleSet(t, testPolicy, policyOpts)
	assert.Equal(t, len(es.RuleSets), 1)
	assert.Contains(t, es.RuleSets, DefaultRuleSetTagValue)
	assert.NotContains(t, es.RuleSets, "threat_score")

	probeEvaluationRuleSet := es.RuleSets[DefaultRuleSetTagValue]

	assert.Nil(t, err)
	assert.Contains(t, probeEvaluationRuleSet.rules, "no_tag")
	assert.Contains(t, probeEvaluationRuleSet.rules, "default_tag")
	assert.NotContains(t, probeEvaluationRuleSet.rules, "threat_score")
	assert.Contains(t, probeEvaluationRuleSet.rules, "special")
}

func TestRuleIDFilter(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "test1",
				Expression: `open.file.path == "/tmp/test"`,
			},
			{
				ID:         "test2",
				Expression: `open.file.path != "/tmp/test"`,
			},
		},
	}

	policyOpts := PolicyLoaderOpts{
		RuleFilters: []RuleFilter{
			&RuleIDFilter{
				ID: "test2",
			},
		},
	}

	es, err := loadPolicyIntoProbeEvaluationRuleSet(t, testPolicy, policyOpts)
	rs := es.RuleSets[DefaultRuleSetTagValue]
	assert.Nil(t, err)

	assert.NotContains(t, rs.rules, "test1")
	assert.Contains(t, rs.rules, "test2")
}
