//go:build sds

//nolint:revive
package sds

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestCreateScanner(t *testing.T) {
	standardRules := []byte(`
        {"priority":1,"is_enabled":true,"rules":[
            {
                "id":"zero-0",
                "description":"zero desc",
                "name":"zero",
                "pattern":"zero"
            },{
                "id":"one-1",
                "description":"one desc",
                "name":"one",
                "pattern":"one"
            },{
                "id":"two-2",
                "description":"two desc",
                "name":"two",
                "pattern":"two"
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

	s := CreateScanner()

	if s == nil {
		t.Error("the scanner should not be nil after a creation")
	}

	err := s.Reconfigure(ReconfigureOrder{
		Type:   StandardRules,
		Config: standardRules,
	})

	if err != nil {
		t.Errorf("configuring the standard rules should not fail: %v", err)
	}

	// now that we have some definitions, we can configure the scanner
	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	if err != nil {
		t.Errorf("this one shouldn't fail, all rules are disabled but it's OK as long as there are no rules in the scanner")
	}

	if s == nil {
		t.Errorf("the scanner should not become a nil object")
	}

	if len(s.configuredRules) > 0 {
		t.Errorf("No rules should be configured, they're all disabled. Got (%v) rules configured instead.", len(s.configuredRules))
	}

	// enable 2 of the 3 rules
	// ------

	agentConfig = bytes.Replace(agentConfig, []byte("\"is_enabled\":false"), []byte("\"is_enabled\":true"), 2)

	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	if err != nil {
		t.Errorf("this one should not fail since two rules are enabled: %v", err)
	}

	if s.Scanner == nil {
		t.Errorf("the Scanner should've been created, it should not be nil")
	}

	if s.Scanner.Rules == nil {
		t.Errorf("the Scanner should use rules")
	}

	if len(s.Scanner.Rules) != 2 {
		t.Errorf("the Scanner should use two rules, has (%d) instead", len(s.Scanner.Rules))
	}

	if len(s.configuredRules) != 2 {
		t.Errorf("only two rules should be part of this scanner. len == %d", len(s.configuredRules))
	}

	// order matters, it's ok to test rules by [] access
	if s.configuredRules[0].Name != "zero" {
		t.Error("incorrect rules selected for configuration")
	}
	if s.configuredRules[1].Name != "one" {
		t.Error("incorrect rules selected for configuration")
	}

	// compare rules returned by GetRuleByIdx

	zero, err := s.GetRuleByIdx(0)
	if err != nil {
		t.Errorf("GetRuleByIdx on 0 should not fail: %v", err)
	}
	if s.configuredRules[0].ID != zero.ID {
		t.Error("incorrect rule returned")
	}

	one, err := s.GetRuleByIdx(1)
	if err != nil {
		t.Errorf("GetRuleByIdx on 1 should not fail: %v", err)
	}
	if s.configuredRules[1].ID != one.ID {
		t.Error("incorrect rule returned")
	}

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

	if err != nil {
		t.Errorf("this one should not fail since one rule is enabled: %v", err)
	}

	if len(s.configuredRules) != 1 {
		t.Errorf("only one rules should be part of this scanner. len == %d", len(s.configuredRules))
	}

	// order matters, it's ok to test rules by [] access
	if s.configuredRules[0].Name != "one" {
		t.Error("incorrect rule selected for configuration")
	}

	rule, err := s.GetRuleByIdx(0)
	if err != nil {
		t.Error("incorrect rule returned")
	}
	if rule.ID != s.configuredRules[0].ID || rule.Name != "one" {
		t.Error("the scanner hasn't been configured with the good rule")
	}

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

	if len(s.configuredRules) != 0 {
		t.Errorf("The group is disabled, no rules should be configured. len == %d", len(s.configuredRules))
	}
}

func TestIsReady(t *testing.T) {
	standardRules := []byte(`
        {"priority":1,"rules":[
            {
                "id":"zero-0",
                "description":"zero desc",
                "name":"zero",
                "pattern":"zero"
            },{
                "id":"one-1",
                "description":"one desc",
                "name":"one",
                "pattern":"one"
            },{
                "id":"two-2",
                "description":"two desc",
                "name":"two",
                "pattern":"two"
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

	s := CreateScanner()

	if s == nil {
		t.Error("the scanner should not be nil after a creation")
	}

	if s.IsReady() != false {
		t.Error("at this stage, the scanner should not be considered ready, no definitions received")
	}

	err := s.Reconfigure(ReconfigureOrder{
		Type:   StandardRules,
		Config: standardRules,
	})

	if err != nil {
		t.Errorf("configuring the definitions should not fail: %v", err)
	}

	if s.IsReady() != false {
		t.Error("at this stage, the scanner should not be considered ready, no user config received")
	}

	// now that we have some definitions, we can configure the scanner
	err = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	if s.IsReady() != true {
		t.Error("at this stage, the scanner should be considered ready")
	}
}

// TestScan validates that everything fits and works. It's not validating
// the scanning feature itself, which is done in the library.
func TestScan(t *testing.T) {
	standardRules := []byte(`
        {"priority":1,"rules":[
            {
                "id":"zero-0",
                "description":"zero desc",
                "name":"zero",
                "pattern":"zero"
            },{
                "id":"one-1",
                "description":"one desc",
                "name":"one",
                "pattern":"one"
            },{
                "id":"two-2",
                "description":"two desc",
                "name":"two",
                "pattern":"two"
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

	s := CreateScanner()
	if s == nil {
		t.Error("the returned scanner should not be nil")
	}
	_ = s.Reconfigure(ReconfigureOrder{
		Type:   StandardRules,
		Config: standardRules,
	})
	_ = s.Reconfigure(ReconfigureOrder{
		Type:   AgentConfig,
		Config: agentConfig,
	})

	if s.IsReady() != true {
		t.Error("at this stage, the scanner should be considered ready")
	}

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
			event:      "",
			matchCount: 0,
		},
	}

	for k, v := range tests {
		msg := message.Message{}
		matched, processed, err := s.Scan([]byte(k), &msg)
		if err != nil {
			t.Errorf("scanning these event should not fail. err received: %v", err)
		}
		if matched && processed == nil {
			t.Errorf("incorrect result: nil processed event returned")
		}
		if matched != v.matched {
			t.Errorf("unexpected match/non-match: expected %v got %v", v.matched, matched)
		}
		if string(processed) != v.event {
			t.Errorf("incorrect result, expected \"%v\" got \"%v\"", v.event, string(processed))
		}
	}
}
