// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package serializers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// The values matched in a dictionary, the environment variables of a process,
// are serialized only for the keys of `envs_with_value` written in the rule.
func TestRuleContextDictionaryValues(t *testing.T) {
	scrubber, err := utils.NewScrubber(nil, nil)
	require.NoError(t, err)
	scrubber.WithEnvsWithValue([]string{"PATH"})

	for _, test := range []struct {
		name     string
		expr     string
		field    string
		value    interface{}
		expected string
	}{
		{"key-not-listed", `exec.env["AWS_SECRET_ACCESS_KEY"] =~ r".+"`, "exec.env", "secret", "********"},
		{"key-listed", `exec.env["PATH"] == "/bin"`, "exec.env", "/bin", "/bin"},
		{"key-listed-spaced", `exec.env [ "PATH" ] == "/bin"`, "exec.env", "/bin", "/bin"},
		{"key-evaluated", `exec.env[exec.file.name] == "x"`, "exec.env", "secret", "********"},
		{"iterated", `process.ancestors.env["TOKEN"] =~ "ghp_*"`, "process.ancestors.env", []string{"ghp_a", "ghp_b"}, "[******** ********]"},
		{"iterated-listed", `process.ancestors.env["PATH"] == "/bin"`, "process.ancestors.env", []string{"/bin", "/usr/bin"}, "[/bin /usr/bin]"},
		{"not-a-dictionary", `exec.file.name == "ls"`, "exec.file.name", "ls", "ls"},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := model.NewFakeEvent()
			offset := strings.Index(test.expr, test.field)
			event.RuleContext.MatchingSubExprs = eval.MatchingSubExprs{{
				Offset: offset,
				ValueA: eval.MatchingValue{Offset: offset, Field: test.field, Value: test.value},
			}}
			rule := &rules.Rule{Rule: &eval.Rule{Expression: test.expr}}

			ruleContext := newRuleContext(event, rule, scrubber)
			require.Len(t, ruleContext.MatchingSubExprs, 1)
			assert.Equal(t, test.expected, ruleContext.MatchingSubExprs[0].Value)
			assert.Equal(t, test.field, ruleContext.MatchingSubExprs[0].Field)
		})
	}
}
