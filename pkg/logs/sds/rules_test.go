// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package sds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testdataStandardRules() map[string]StandardRuleConfig {
	return map[string]StandardRuleConfig{
		"0": {
			ID:          "0",
			Name:        "Zero",
			Description: "Zero desc",
			Definitions: []StandardRuleDefinition{{
				Pattern: "zero",
				Tags:    []string{"tag:zero"},
			}},
		},
		"1": {
			ID:          "1",
			Name:        "One",
			Description: "One desc",
			Definitions: []StandardRuleDefinition{{
				Pattern: "one",
				Tags:    []string{"tag:one"},
			}},
		},
		"2": {
			ID:          "2",
			Name:        "Two",
			Description: "Two desc",
			Definitions: []StandardRuleDefinition{{
				Pattern: "two",
				Tags:    []string{"tag:two"},
			}},
		},
	}
}

//nolint:staticcheck
func TestGetByID(t *testing.T) {
	require := require.New(t)
	rules := testdataStandardRules()

	require.Contains(rules, "2", "rule two exists, should be returned")
	two := rules["2"]
	lastDef, err := two.LastSupportedVersion()
	require.NoError(err)
	require.Equal(two.ID, "2", "not the good rule")
	require.Equal(two.Name, "Two", "not the good rule")
	require.Equal(two.Description, "Two desc", "not the good rule")
	require.Equal(lastDef.Pattern, "two", "not the good rule")

	require.Contains(rules, "0", "rule zero exists, should be returned")
	zero := rules["0"]
	require.Equal(zero.Name, "Zero", "not the good rule")

	require.NotContains(rules, "meh", "rule doesn't exist, nothing should be returned")
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
	require := require.New(t)
	rules := testdataRulesConfig()

	onlyEnabled := rules.OnlyEnabled()
	require.Len(onlyEnabled.Rules, 1, "only one rule should be enabled.")
	require.Equal(onlyEnabled.Rules[0].Name, "One", "only One should be enabled")

	// disable the whole group
	rules.IsEnabled = false
	onlyEnabled = rules.OnlyEnabled()
	require.Len(onlyEnabled.Rules, 0, "the group is disabled, no rules should be returned")
}
