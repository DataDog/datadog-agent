// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sds

//nolint:revive
package sds

import (
	"bytes"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	sds "github.com/DataDog/dd-sensitive-data-scanner/sds-go/go"
	"github.com/stretchr/testify/require"
)

func TestCreateScanner(t *testing.T) {
	require := require.New(t)

	standardRules := []byte(`
        {"priority":1,"is_enabled":true,"rules":[
            {
                "id":"zero-0",
                "description":"zero desc",
                "name":"zero",
                "definitions": [{"version":1, "pattern":"zero"}]
            },{
                "id":"one-1",
                "description":"one desc",
                "name":"one",
                "definitions": [{"version":1, "pattern":"one"}]
            },{
                "id":"two-2",
                "description":"two desc",
                "name":"two",
                "definitions": [{"version":1, "pattern":"two"}]
            }
        ]}
    `)
	agentConfig := []byte(`
        {"is_enabled":true,"rules":[
            {
                "id": "random000",
                "name":"zero",
                "definition":{"standard_rule_id":"zero-0"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            },{
                "id": "random111",
                "name":"one",
                "definition":{"standard_rule_id":"one-1"},
                "match_action":{"type":"Hash"},
                "is_enabled":false
            },{
                "id": "random222",
                "name":"two",
                "definition":{"standard_rule_id":"two-2"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            }
        ]}
    `)

	// scanner creation
	// -----

	s := CreateScanner(0)

	require.NotNil(s, "the scanner should not be nil after a creation")

	err := s.Reconfigure(ReconfigureOrder{
		Type:   StandardRules,
		Config: standardRules,
	})

	require.NoError(err, "configuring the standard rules should not fail")

	// now that we have some definitions, we can configure the scanner
	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	require.NoError(err, "this one shouldn't fail, all rules are disabled but it's OK as long as there are no rules in the scanner")

	require.NotNil(s, "the scanner should not become a nil object")

	if s != nil && len(s.configuredRules) > 0 {
		t.Errorf("No rules should be configured, they're all disabled. Got (%v) rules configured instead.", len(s.configuredRules))
	}

	// enable 2 of the 3 rules
	// ------

	agentConfig = bytes.Replace(agentConfig, []byte("\"is_enabled\":false"), []byte("\"is_enabled\":true"), 2)

	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	require.NoError(err, "this one should not fail since two rules are enabled: %v", err)

	require.NotNil(s.Scanner, "the Scanner should've been created, it should not be nil")
	require.NotNil(s.Scanner.Rules, "the Scanner should use rules")

	require.Len(s.Scanner.Rules, 2, "the Scanner should use two rules")
	require.Len(s.configuredRules, 2, "only two rules should be part of this scanner.")

	// order matters, it's ok to test rules by [] access
	require.Equal(s.configuredRules[0].Name, "zero", "incorrect rules selected for configuration")
	require.Equal(s.configuredRules[1].Name, "one", "incorrect rules selected for configuration")

	// compare rules returned by GetRuleByIdx

	zero, err := s.GetRuleByIdx(0)
	require.NoError(err, "GetRuleByIdx on 0 should not fail")
	require.Equal(s.configuredRules[0].ID, zero.ID, "incorrect rule returned")
	one, err := s.GetRuleByIdx(1)
	require.NoError(err, "GetRuleByIdx on 1 should not fail")
	require.Equal(s.configuredRules[1].ID, one.ID, "incorrect rule returned")

	// disable the rule zero
	// only "one" is left enabled
	// -----

	agentConfig = []byte(`
        {"is_enabled":true,"rules":[
            {
                "id": "random000",
                "name":"zero",
                "definition":{"standard_rule_id":"zero-0"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            },{
                "id": "random111",
                "name":"one",
                "definition":{"standard_rule_id":"one-1"},
                "match_action":{"type":"Hash"},
                "is_enabled":true
            },{
                "id": "random222",
                "name":"two",
                "definition":{"standard_rule_id":"two-2"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            }
        ]}
    `)

	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	require.NoError(err, "this one should not fail since one rule is enabled")
	require.Len(s.configuredRules, 1, "only one rules should be part of this scanner")

	// order matters, it's ok to test rules by [] access
	require.Equal(s.configuredRules[0].Name, "one", "incorrect rule selected for configuration")

	rule, err := s.GetRuleByIdx(0)
	require.NoError(err, "incorrect rule returned")
	require.Equal(rule.ID, s.configuredRules[0].ID, "the scanner hasn't been configured with the good rule")
	require.Equal(rule.Name, "one", "the scanner hasn't been configured with the good rule")

	// disable the whole group

	agentConfig = []byte(`
        {"is_enabled":false,"rules":[
            {
                "id": "random000",
                "name":"zero",
                "definition":{"standard_rule_id":"zero-0"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":true
            },{
                "id": "random111",
                "name":"one",
                "definition":{"standard_rule_id":"one-1"},
                "match_action":{"type":"Hash"},
                "is_enabled":true
            },{
                "id": "random222",
                "name":"two",
                "definition":{"standard_rule_id":"two-2"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            }
        ]}
    `)

	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	require.NoError(err, "no error should happen")
	require.Len(s.configuredRules, 0, "The group is disabled, no rules should be configured.")
}

// TestEmptyConfiguration validates that the scanner is destroyed when receiving
// an empty configuration.
func TestEmptyConfiguration(t *testing.T) {
	require := require.New(t)

	standardRules := []byte(`
        {"priority":1,"is_enabled":true,"rules":[
            {
                "id":"zero-0",
                "description":"zero desc",
                "name":"zero",
                "definitions": [{"version":1, "pattern":"zero"}]
            },{
                "id":"one-1",
                "description":"one desc",
                "name":"one",
                "definitions": [{"version":1, "pattern":"one"}]
            },{
                "id":"two-2",
                "description":"two desc",
                "name":"two",
                "definitions": [{"version":1, "pattern":"two"}]
            }
        ]}
    `)
	agentConfig := []byte(`
        {"is_enabled":true,"rules":[
            {
                "id": "random000",
                "name":"zero",
                "definition":{"standard_rule_id":"zero-0"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":true
            }
        ]}
    `)

	s := CreateScanner(0)

	require.NotNil(s, "the scanner should not be nil after a creation")

	err := s.Reconfigure(ReconfigureOrder{
		Type:   StandardRules,
		Config: standardRules,
	})

	require.NoError(err, "configuring the standard rules should not fail")

	// configure with one rule

	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	require.NoError(err, "this one should not fail since one rule is enabled")
	require.Len(s.configuredRules, 1, "only one rules should be part of this scanner")
	require.NotNil(s.Scanner)

	// empty reconfiguration

	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: []byte("{}"),
	})

	require.NoError(err)
	require.Len(s.configuredRules, 0)
	require.Nil(s.Scanner)
}

func TestIsReady(t *testing.T) {
	require := require.New(t)

	standardRules := []byte(`
        {"priority":1,"rules":[
            {
                "id":"zero-0",
                "description":"zero desc",
                "name":"zero",
                "definitions": [{"version":1, "pattern":"zero"}]
            },{
                "id":"one-1",
                "description":"one desc",
                "name":"one",
                "definitions": [{"version":1, "pattern":"one"}]
            },{
                "id":"two-2",
                "description":"two desc",
                "name":"two",
                "definitions": [{"version":1, "pattern":"two"}]
            }
        ]}
    `)
	agentConfig := []byte(`
        {"is_enabled":true,"rules":[
            {
                "id":"random-0000000",
                "definition":{"standard_rule_id":"zero-0"},
                "name":"zero",
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":true
            },{
                "id":"random-111",
                "definition":{"standard_rule_id":"one-1"},
                "name":"one",
                "match_action":{"type":"Hash"},
                "is_enabled":true
            }
        ]}
    `)

	// scanner creation
	// -----

	s := CreateScanner(0)

	require.NotNil(s, "the scanner should not be nil after a creation")
	require.False(s.IsReady(), "at this stage, the scanner should not be considered ready, no definitions received")

	err := s.Reconfigure(ReconfigureOrder{
		Type:   StandardRules,
		Config: standardRules,
	})

	require.NoError(err, "configuring the definitions should not fail")
	require.False(s.IsReady(), "at this stage, the scanner should not be considered ready, no user config received")

	// now that we have some definitions, we can configure the scanner
	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	require.True(s.IsReady(), "at this stage, the scanner should be considered ready")
}

// TestScan validates that everything fits and works. It's not validating
// the scanning feature itself, which is done in the library.
func TestScan(t *testing.T) {
	require := require.New(t)

	standardRules := []byte(`
        {"priority":1,"rules":[
            {
                "id":"zero-0",
                "description":"zero desc",
                "name":"zero",
                "definitions": [{"version":1, "pattern":"zero"}]
            },{
                "id":"one-1",
                "description":"one desc",
                "name":"one",
                "definitions": [{"version":1, "pattern":"one"}]
            },{
                "id":"two-2",
                "description":"two desc",
                "name":"two",
                "definitions": [{"version":1, "pattern":"two"}]
            }
        ]}
    `)
	agentConfig := []byte(`
        {"is_enabled":true,"rules":[
            {
                "id":"random-00000",
                "definition":{"standard_rule_id":"zero-0"},
                "name":"zero",
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":true
            },{
                "id":"random-11111",
                "definition":{"standard_rule_id":"one-1"},
                "name":"one",
                "match_action":{"type":"Redact","placeholder":"[REDACTED]"},
                "is_enabled":true
            }
        ]}
    `)

	// scanner creation
	// -----

	s := CreateScanner(0)
	require.NotNil(s, "the returned scanner should not be nil")

	_ = s.Reconfigure(ReconfigureOrder{
		Type:   StandardRules,
		Config: standardRules,
	})
	_ = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	require.True(s.IsReady(), "at this stage, the scanner should be considered ready")
	type result struct {
		matched    bool
		event      string
		matchCount int
	}

	tests := map[string]result{
		"one two three go!": {
			matched:    true,
			event:      "[REDACTED] two three go!",
			matchCount: 1,
		},
		"after zero comes one, after one comes two, and the rest is history": {
			matched:    true,
			event:      "after [redacted] comes [REDACTED], after [REDACTED] comes two, and the rest is history",
			matchCount: 3,
		},
		"and so we go": {
			matched:    false,
			event:      "and so we go",
			matchCount: 0,
		},
	}

	for k, v := range tests {
		msg := message.Message{}
		matched, processed, err := s.Scan([]byte(k), &msg)
		require.NoError(err, "scanning these event should not fail.")
		require.False(matched && processed == nil, "incorrect result: nil processed event returned")
		require.Equal(matched, v.matched, "unexpected match/non-match")
		require.Equal(string(processed), v.event, "incorrect result")
	}
}

// TestCloseCycleScan validates that the close cycle works well (not blocking, not racing).
// by trying hard to reproduce a possible race on close.
func TestCloseCycleScan(t *testing.T) {
	require := require.New(t)

	standardRules := []byte(`
        {"priority":1,"rules":[
            {
                "id":"zero-0",
                "description":"zero desc",
                "name":"zero",
                "definitions": [{"version":1, "pattern":"zero"}]
            }
        ]}
    `)
	agentConfig := []byte(`
        {"is_enabled":true,"rules":[
            {
                "id":"random-00000",
                "definition":{"standard_rule_id":"zero-0"},
                "name":"zero",
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":true
            },{
                "id":"random-11111",
                "definition":{"standard_rule_id":"zero-0"},
                "name":"one",
                "match_action":{"type":"Redact","placeholder":"[REDACTED]"},
                "is_enabled":true
            }
        ]}
    `)

	// scanner creation
	// -----

	for i := 0; i < 10; i++ {
		s := CreateScanner(0)
		require.NotNil(s, "the returned scanner should not be nil")

		_ = s.Reconfigure(ReconfigureOrder{
			Type:   StandardRules,
			Config: standardRules,
		})
		_ = s.Reconfigure(ReconfigureOrder{
			Type:   AgentConfig,
			Config: agentConfig,
		})

		require.True(s.IsReady(), "at this stage, the scanner should be considered ready")
		type result struct {
			matched    bool
			event      string
			matchCount int
		}

		tests := map[string]result{
			"one two three go!": {
				matched:    true,
				event:      "[REDACTED] two three go!",
				matchCount: 1,
			},
			"after zero comes one, after one comes two, and the rest is history": {
				matched:    true,
				event:      "after [redacted] comes [REDACTED], after [REDACTED] comes two, and the rest is history",
				matchCount: 3,
			},
			"and so we go": {
				matched:    false,
				event:      "and so we go",
				matchCount: 0,
			},
		}

		// this test is about being over-cautious, making sure the Scan method
		// will never cause a race when calling the Delete method at the same time.
		// It can't happen with the current implementation / concurrency pattern
		// used in processor.go, but I'm being over-cautious because if it happens
		// in the future because of someone changing the processor implementation,
		// it could lead to a panic and a hard crash of the Agent.

		go func() {
			for {
				for k := range tests {
					msg := message.Message{}
					s.Scan([]byte(k), &msg)
				}
			}
		}()

		time.Sleep(100 * time.Millisecond)
		s.Delete()
	}
}

func TestInterpretRC(t *testing.T) {
	require := require.New(t)

	stdRc := StandardRuleConfig{
		ID:          "0",
		Name:        "Zero",
		Description: "Zero desc",
		Definitions: []StandardRuleDefinition{{
			Version: 1,
			Pattern: "rule pattern 1",
		}},
	}

	rc := RuleConfig{
		Name:        "test",
		Description: "desc",
		Definition: RuleDefinition{
			StandardRuleID: "0",
		},
		Tags: []string{"tag:test"},
		MatchAction: MatchAction{
			Type:        matchActionRCRedact,
			Placeholder: "[redacted]",
		},
	}

	rule, err := interpretRCRule(rc, stdRc, StandardRulesDefaults{})
	require.NoError(err)

	require.Equal(rule.Id, "Zero")
	require.Equal(rule.Pattern, "rule pattern 1")
	require.Equal(rule.SecondaryValidator, sds.SecondaryValidator(""))

	// add a version with a required capability
	stdRc.Definitions = append(stdRc.Definitions, StandardRuleDefinition{
		Version:              2,
		Pattern:              "second pattern",
		RequiredCapabilities: []string{RCSecondaryValidationLuhnChecksum},
	})

	rule, err = interpretRCRule(rc, stdRc, StandardRulesDefaults{})
	require.NoError(err)

	require.Equal(rule.Id, "Zero")
	require.Equal(rule.Pattern, "second pattern")
	require.Equal(rule.SecondaryValidator, sds.LuhnChecksum)

	// add a third version with an unknown required capability
	// it should fallback on using the version 2
	// also, make sure the version ain't ordered properly
	stdRc.Definitions = []StandardRuleDefinition{
		{
			Version:              2,
			Pattern:              "second pattern",
			RequiredCapabilities: []string{RCSecondaryValidationLuhnChecksum},
		},
		{
			Version:              1,
			Pattern:              "first pattern",
			RequiredCapabilities: nil,
		},
		{
			Version:              3,
			Pattern:              "third pattern",
			RequiredCapabilities: []string{"unsupported"},
		},
	}

	rule, err = interpretRCRule(rc, stdRc, StandardRulesDefaults{})
	require.NoError(err)

	require.Equal(rule.Id, "Zero")
	require.Equal(rule.Pattern, "second pattern")
	require.Equal(rule.SecondaryValidator, sds.LuhnChecksum)

	// make sure we use the keywords proximity feature if any's configured
	// in the std rule definition 	stdRc.Definitions = []StandardRuleDefinition{
	stdRc.Definitions = []StandardRuleDefinition{
		{
			Version:                 2,
			Pattern:                 "second pattern",
			RequiredCapabilities:    []string{RCSecondaryValidationLuhnChecksum},
			DefaultIncludedKeywords: []string{"hello"},
		},
		{
			Version:              1,
			Pattern:              "first pattern",
			RequiredCapabilities: nil,
		},
	}

	rule, err = interpretRCRule(rc, stdRc, StandardRulesDefaults{IncludedKeywordsCharCount: 10})
	require.NoError(err)

	require.Equal(rule.Id, "Zero")
	require.Equal(rule.Pattern, "second pattern")
	require.Equal(rule.SecondaryValidator, sds.LuhnChecksum)
	require.NotNil(rule.ProximityKeywords)
	require.Equal(rule.ProximityKeywords.LookAheadCharacterCount, uint32(10))
	require.Equal(rule.ProximityKeywords.IncludedKeywords, []string{"hello"})

	// make sure we use the user provided information first
	// even if there is some in the std rule
	rc.IncludedKeywords = ProximityKeywords{
		Keywords:       []string{"custom"},
		CharacterCount: 42,
	}

	rule, err = interpretRCRule(rc, stdRc, StandardRulesDefaults{IncludedKeywordsCharCount: 10})
	require.NoError(err)

	require.Equal(rule.Id, "Zero")
	require.Equal(rule.Pattern, "second pattern")
	require.Equal(rule.SecondaryValidator, sds.LuhnChecksum)
	require.NotNil(rule.ProximityKeywords)
	require.Equal(rule.ProximityKeywords.LookAheadCharacterCount, uint32(42))
	require.Equal(rule.ProximityKeywords.IncludedKeywords, []string{"custom"})
}
