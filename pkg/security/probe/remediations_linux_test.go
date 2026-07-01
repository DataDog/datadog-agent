// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func newTestRule(id string, tags ...string) *rules.Rule {
	return &rules.Rule{
		PolicyRule: &rules.PolicyRule{},
		Rule: &eval.Rule{
			ID:   eval.RuleID(id),
			Tags: tags,
		},
	}
}

func TestGetRemediationKeyFromAction(t *testing.T) {
	tests := []struct {
		name     string
		rule     *rules.Rule
		action   *rules.Action
		expected string
	}{
		{
			name: "kill action without remediation tag",
			rule: newTestRule("test_rule"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
						Scope:  "process",
					},
				},
			},
			expected: KillKeyPrefix + "test_rule" + "process" + "SIGKILL",
		},
		{
			name: "kill action with remediation tag true",
			rule: newTestRule("test_rule", "remediation_rule:true"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{
					Kill: &rules.KillDefinition{
						Signal: "SIGTERM",
						Scope:  "container",
					},
				},
			},
			expected: RemediationKeyPrefix + KillKeyPrefix + "test_rule" + "container" + "SIGTERM",
		},
		{
			name: "kill action with remediation tag false",
			rule: newTestRule("test_rule", "remediation_rule:false"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
						Scope:  "process",
					},
				},
			},
			expected: KillKeyPrefix + "test_rule" + "process" + "SIGKILL",
		},
		{
			name: "network isolation action without remediation tag",
			rule: newTestRule("net_rule"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{
					NetworkFilter: &rules.NetworkFilterDefinition{
						BPFFilter: "tcp port 80",
					},
				},
			},
			expected: func() string {
				hash := sha256.Sum256([]byte("tcp port 80"))
				return NetworkIsolationKeyPrefix + "net_rule" + hex.EncodeToString(hash[:])
			}(),
		},
		{
			name: "network isolation action with remediation tag",
			rule: newTestRule("net_rule", "remediation_rule:true"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{
					NetworkFilter: &rules.NetworkFilterDefinition{
						BPFFilter: "udp port 53",
					},
				},
			},
			expected: func() string {
				hash := sha256.Sum256([]byte("udp port 53"))
				return RemediationKeyPrefix + NetworkIsolationKeyPrefix + "net_rule" + hex.EncodeToString(hash[:])
			}(),
		},
		{
			name: "action with neither kill nor network filter",
			rule: newTestRule("other_rule"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{},
			},
			expected: "",
		},
		{
			name: "action with neither kill nor network filter but with remediation tag",
			rule: newTestRule("other_rule", "remediation_rule:true"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{},
			},
			expected: RemediationKeyPrefix,
		},
		{
			name: "kill action with multiple tags including remediation",
			rule: newTestRule("multi_tag_rule", "team:security", "remediation_rule:true", "env:prod"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
						Scope:  "cgroup",
					},
				},
			},
			expected: RemediationKeyPrefix + KillKeyPrefix + "multi_tag_rule" + "cgroup" + "SIGKILL",
		},
		{
			name: "different rules produce different keys",
			rule: newTestRule("rule_a"),
			action: &rules.Action{
				Def: &rules.ActionDefinition{
					Kill: &rules.KillDefinition{
						Signal: "SIGKILL",
						Scope:  "process",
					},
				},
			},
			expected: KillKeyPrefix + "rule_a" + "process" + "SIGKILL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRemediationKeyFromAction(tt.rule, tt.action)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRemediationKeyFromActionUniqueness(t *testing.T) {
	ruleA := newTestRule("rule_a")
	ruleB := newTestRule("rule_b")

	killAction := &rules.Action{
		Def: &rules.ActionDefinition{
			Kill: &rules.KillDefinition{
				Signal: "SIGKILL",
				Scope:  "process",
			},
		},
	}

	keyA := getRemediationKeyFromAction(ruleA, killAction)
	keyB := getRemediationKeyFromAction(ruleB, killAction)
	assert.NotEqual(t, keyA, keyB, "different rule IDs should produce different keys")

	killSigterm := &rules.Action{
		Def: &rules.ActionDefinition{
			Kill: &rules.KillDefinition{
				Signal: "SIGTERM",
				Scope:  "process",
			},
		},
	}
	keySigkill := getRemediationKeyFromAction(ruleA, killAction)
	keySigterm := getRemediationKeyFromAction(ruleA, killSigterm)
	assert.NotEqual(t, keySigkill, keySigterm, "different signals should produce different keys")

	killContainer := &rules.Action{
		Def: &rules.ActionDefinition{
			Kill: &rules.KillDefinition{
				Signal: "SIGKILL",
				Scope:  "container",
			},
		},
	}
	keyProcess := getRemediationKeyFromAction(ruleA, killAction)
	keyContainer := getRemediationKeyFromAction(ruleA, killContainer)
	assert.NotEqual(t, keyProcess, keyContainer, "different scopes should produce different keys")

	netAction1 := &rules.Action{
		Def: &rules.ActionDefinition{
			NetworkFilter: &rules.NetworkFilterDefinition{
				BPFFilter: "tcp port 80",
			},
		},
	}
	netAction2 := &rules.Action{
		Def: &rules.ActionDefinition{
			NetworkFilter: &rules.NetworkFilterDefinition{
				BPFFilter: "tcp port 443",
			},
		},
	}
	keyNet1 := getRemediationKeyFromAction(ruleA, netAction1)
	keyNet2 := getRemediationKeyFromAction(ruleA, netAction2)
	assert.NotEqual(t, keyNet1, keyNet2, "different BPF filters should produce different keys")
}

func TestGetRemediationKeyFromActionIdempotent(t *testing.T) {
	rule := newTestRule("stable_rule", "remediation_rule:true")
	action := &rules.Action{
		Def: &rules.ActionDefinition{
			Kill: &rules.KillDefinition{
				Signal: "SIGKILL",
				Scope:  "process",
			},
		},
	}

	key1 := getRemediationKeyFromAction(rule, action)
	key2 := getRemediationKeyFromAction(rule, action)
	assert.Equal(t, key1, key2, "same inputs should always produce the same key")
}
