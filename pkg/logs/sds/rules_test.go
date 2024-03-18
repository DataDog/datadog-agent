//nolint:revive
package sds

import (
	"testing"
)

func testdataStandardRules() StandardRulesConfig {
	return StandardRulesConfig{
		Rules: []StandardRuleConfig{
			{
				ID:          "0",
				Name:        "Zero",
				Description: "Zero desc",
				Pattern:     "zero",
				Tags:        []string{"tag:zero"},
			},
			{
				ID:          "1",
				Name:        "One",
				Description: "One desc",
				Pattern:     "one",
				Tags:        []string{"tag:one"},
			},
			{
				ID:          "2",
				Name:        "Two",
				Description: "Two desc",
				Pattern:     "two",
				Tags:        []string{"tag:two"},
			},
		},
	}
}

//nolint:staticcheck
func TestGetByID(t *testing.T) {
	rules := testdataStandardRules()

	two := rules.GetByID("2")
	if two == nil {
		t.Error("rule two exists, should be returned")
	}
	if two.ID != "2" {
		t.Error("not the good rule")
	}
	if two.Name != "Two" {
		t.Error("not the good rule")
	}
	if two.Description != "Two desc" {
		t.Error("not the good rule")
	}
	if two.Pattern != "two" {
		t.Error("not the good rule")
	}

	zero := rules.GetByID("0")
	if zero == nil {
		t.Error("rule zero exists, should be returned")
	}
	if zero.Name != "Zero" {
		t.Error("not the good rule")
	}

	unknown := rules.GetByID("meh")
	if unknown != nil {
		t.Error("rule doesn't exist, nothing should be returned")
	}
}

func testdataRulesConfig() RulesConfig {
	return RulesConfig{
		IsEnabled: true,
		Rules: []RuleConfig{
			{
				ID:          "0",
				Name:        "Zero",
				Description: "Zero desc",
				IsEnabled:   false,
			},
			{
				ID:          "1",
				Name:        "One",
				Description: "One desc",
				IsEnabled:   true,
			},
			{
				ID:          "2",
				Name:        "Two",
				Description: "Two desc",
				IsEnabled:   false,
			},
		},
	}
}

func TestOnlyEnabled(t *testing.T) {
	rules := testdataRulesConfig()

	onlyEnabled := rules.OnlyEnabled()
	if len(onlyEnabled.Rules) != 1 {
		t.Errorf("only one rule should be enabled. Expected (%v), got (%v)", 1, len(onlyEnabled.Rules))
	}

	if onlyEnabled.Rules[0].Name != "One" {
		t.Error("only One should enabled")
	}

	// disable the whole group
	rules.IsEnabled = false
	onlyEnabled = rules.OnlyEnabled()
	if len(onlyEnabled.Rules) > 0 {
		t.Error("the group is disabled, no rules should be returned")
	}
}
