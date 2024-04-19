// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package sds

// RulesConfig as sent by the Remote Configuration.
// Equivalent of the groups in the UI.
type RulesConfig struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Rules       []RuleConfig `json:"rules"`
	IsEnabled   bool         `json:"is_enabled"`
	Description string       `json:"description"`
}

// MatchAction defines what's the action to do when there is a match.
type MatchAction struct {
	Type           string `json:"type"`
	Placeholder    string `json:"placeholder"`
	Direction      string `json:"direction"`
	CharacterCount uint32 `json:"character_count"`
}

// StandardRuleConfig as sent by the Remote Configuration;
type StandardRuleConfig struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Definitions []StandardRuleDefinition `json:"definitions"`
}

// StandardRuleDefinition contains a versioned standard rule definition.
type StandardRuleDefinition struct {
	Version                 int                  `json:"version"`
	Pattern                 string               `json:"pattern"`
	Tags                    []string             `json:"tags"`
	DefaultIncludedKeywords []string             `json:"default_included_keywords"`
	SecondaryValidators     []SecondaryValidator `json:"secondary_validation"`
}

// SecondaryValidatorn definition.
type SecondaryValidator struct {
	Type string `json:"type"`
}

// StandardRulesConfig contains standard rules.
type StandardRulesConfig struct {
	Rules []StandardRuleConfig `json:"rules"`
}

// RuleConfig of rule as sent by the Remote Configuration.
type RuleConfig struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Tags             []string          `json:"tags"`
	Definition       RuleDefinition    `json:"definition"`
	MatchAction      MatchAction       `json:"match_action"`
	IncludedKeywords ProximityKeywords `json:"included_keywords"`
	IsEnabled        bool              `json:"is_enabled"`
}

// ProximityKeywords definition in RC config.
type ProximityKeywords struct {
	Keywords       []string `json:"keywords"`
	CharacterCount uint32   `json:"character_count"`
}

// RuleDefinition definition in RC config.
type RuleDefinition struct {
	StandardRuleID string `json:"standard_rule_id"`
	Pattern        string `json:"pattern"`
}

// OnlyEnabled returns a new RulesConfig object containing only enabled rules.
// Use this to filter out disabled rules.
func (r RulesConfig) OnlyEnabled() RulesConfig {
	// is the whole groupe disabled?
	if !r.IsEnabled {
		return RulesConfig{Rules: []RuleConfig{}}
	}

	rules := []RuleConfig{}
	for _, rule := range r.Rules {
		if rule.IsEnabled {
			rules = append(rules, rule)
		}
	}
	return RulesConfig{
		Rules: rules,
	}
}
