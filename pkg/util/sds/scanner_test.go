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

	"github.com/stretchr/testify/require"
)

func TestCreateScanner(t *testing.T) {
	require := require.New(t)

	config := []byte(`
        {"is_enabled":true,"rules":[
            {
                "id": "random000",
                "name":"zero",
                "definition":{"pattern":"zero"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            },{
                "id": "random111",
                "name":"one",
                "definition":{"pattern":"one"},
                "match_action":{"type":"Hash"},
                "is_enabled":false
            },{
                "id": "random222",
                "name":"two",
                "definition":{"pattern":"two"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            }
        ]}
    `)

	// scanner creation
	// -----

	s := CreateScanner()

	require.NotNil(s, "the scanner should not be nil after a creation")

	err := s.Reconfigure(ReconfigureOrder{
		Type:   DatadogRules,
		Config: config,
	})

	require.NoError(err, "this one shouldn't fail, all rules are disabled but it's OK as long as there are no rules in the scanner")

	require.NotNil(s, "the scanner should not become a nil object")

	if s != nil && len(s.configuredRules) > 0 {
		t.Errorf("No rules should be configured, they're all disabled. Got (%v) rules configured instead.", len(s.configuredRules))
	}

	// enable 2 of the 3 rules
	// ------

	config = bytes.Replace(config, []byte("\"is_enabled\":false"), []byte("\"is_enabled\":true"), 2)

	err = s.Reconfigure(ReconfigureOrder{
		Type:   DatadogRules,
		Config: config,
	})

	require.NoError(err, "this one should not fail since two rules are enabled: %v", err)

	require.NotNil(s.Scanner, "the Scanner should've been created, it should not be nil")
	require.NotNil(s.Scanner.RuleConfigs, "the Scanner should use rules")

	require.Len(s.Scanner.RuleConfigs, 2, "the Scanner should use two rules")
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

	config = []byte(`
        {"is_enabled":true,"rules":[
            {
                "id": "random000",
                "name":"zero",
                "definition":{"pattern":"zero"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            },{
                "id": "random111",
                "name":"one",
                "definition":{"pattern":"one"},
                "match_action":{"type":"Hash"},
                "is_enabled":true
            },{
                "id": "random222",
                "name":"two",
                "definition":{"pattern":"two"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            }
        ]}
    `)

	err = s.Reconfigure(ReconfigureOrder{
		Type:   DatadogRules,
		Config: config,
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

	config = []byte(`
        {"is_enabled":false,"rules":[
            {
                "id": "random000",
                "name":"zero",
                "definition":{"pattern":"zero"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":true
            },{
                "id": "random111",
                "name":"one",
                "definition":{"pattern":"one"},
                "match_action":{"type":"Hash"},
                "is_enabled":true
            },{
                "id": "random222",
                "name":"two",
                "definition":{"pattern":"two"},
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":false
            }
        ]}
    `)

	err = s.Reconfigure(ReconfigureOrder{
		Type:   DatadogRules,
		Config: config,
	})

	require.NoError(err, "no error should happen")
	require.Len(s.configuredRules, 0, "The group is disabled, no rules should be configured.")
}

func TestIsReady(t *testing.T) {
	require := require.New(t)

	config := []byte(`
        {"is_enabled":true,"rules":[
            {
                "id":"random-0000000",
                "definition":{"pattern":"zero"},
                "name":"zero",
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":true
            },{
                "id":"random-111",
                "definition":{"pattern":"one"},
                "name":"one",
                "match_action":{"type":"Hash"},
                "is_enabled":true
            }
        ]}
    `)

	// scanner creation
	// -----

	s := CreateScanner()

	require.NotNil(s, "the scanner should not be nil after a creation")
	require.False(s.IsReady(), "at this stage, the scanner should not be considered ready, no config received")

	// now that the scanner is created, we can configure it
	err := s.Reconfigure(ReconfigureOrder{
		Type:   DatadogRules,
		Config: config,
	})

	require.NoError(err, "configuring the scanner should not fail")
	require.True(s.IsReady(), "at this stage, the scanner should be considered ready")
}

// TestScan validates that everything fits and works. It's not validating
// the scanning feature itself, which is done in the library.
func TestScan(t *testing.T) {
	require := require.New(t)

	config := []byte(`
        {"is_enabled":true,"rules":[
            {
                "id":"random-00000",
                "definition":{"pattern":"zero"},
                "name":"zero",
                "match_action":{"type":"Redact","placeholder":"[redacted]"},
                "is_enabled":true
            },{
                "id":"random-11111",
                "definition":{"pattern":"one"},
                "name":"one",
                "match_action":{"type":"Redact","placeholder":"[REDACTED]"},
                "is_enabled":true
            }
        ]}
    `)

	// scanner creation
	// -----

	s := CreateScanner()
	require.NotNil(s, "the returned scanner should not be nil")

	_ = s.Reconfigure(ReconfigureOrder{
		Type:   DatadogRules,
		Config: config,
	})

	require.True(s.IsReady(), "at this stage, the scanner should be considered ready")
	type result struct {
		matched bool
		event   string
	}

	tests := map[string]result{
		"one two three go!": {
			matched: true,
			event:   "[REDACTED] two three go!",
		},
		"after zero comes one, after one comes two, and the rest is history": {
			matched: true,
			event:   "after [redacted] comes [REDACTED], after [REDACTED] comes two, and the rest is history",
		},
		"and so we go": {
			matched: false,
			event:   "",
		},
	}

	for k, v := range tests {
		matched, processed, err := s.Scan([]byte(k))
		require.NoError(err, "scanning these event should not fail.")
		require.False(matched && processed == nil, "incorrect result: nil processed event returned")
		require.Equal(matched, v.matched, "unexpected match/non-match")
		require.Equal(string(processed), v.event, "incorrect result")
	}
}
