// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOTTLStructuralValidation(t *testing.T) {
	for _, kind := range []string{IncludeAtMatch, ExcludeAtMatch, MaskSequences} {
		t.Run(kind, func(t *testing.T) {
			rules := []*ProcessingRule{{Type: kind, Name: "test", OTTL: "invalid OTTL syntax"}}
			// Shared configuration checks structure without linking an OTTL parser.
			require.NoError(t, ValidateProcessingRules(rules))
			require.NoError(t, CompileProcessingRules(rules))
			require.Nil(t, rules[0].OTTLProgram)
			require.Nil(t, rules[0].Regex, "an ottl rule must not compile its expression as a regex")
			rules[0].OTTL = ""
			require.ErrorContains(t, ValidateProcessingRules(rules), "no pattern provided")
		})
	}
}

func TestOTTLFieldValidation(t *testing.T) {
	tests := []struct {
		name string
		rule ProcessingRule
		err  string
	}{
		{"pattern and ottl", ProcessingRule{Type: IncludeAtMatch, Name: "both", Pattern: "prod", OTTL: `attributes["env"] == "prod"`}, "sets both pattern and ottl"},
		{"placeholder with ottl", ProcessingRule{Type: MaskSequences, Name: "mask", ReplacePlaceholder: "***", OTTL: `set(attributes["password"], "***")`}, "replace_placeholder cannot be used with ottl"},
		{"multi_line", ProcessingRule{Type: MultiLine, Name: "ml", OTTL: `IsMatch(body, "^\\d")`}, "ottl is not supported for processing rule type multi_line"},
		{"exclude_truncated", ProcessingRule{Type: ExcludeTruncated, Name: "trunc", OTTL: `true`}, "ottl is not supported for processing rule type exclude_truncated"},
		{"missing type", ProcessingRule{Name: "untyped", OTTL: `true`}, "type must be set"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, ValidateProcessingRules([]*ProcessingRule{&test.rule}), test.err)
		})
	}
}

func TestOTTLFieldParsing(t *testing.T) {
	yamlConfig := []byte(`logs:
  - type: file
    path: app.log
    log_processing_rules:
      - type: include_at_match
        name: keep_prod
        ottl: 'attributes["env"] == "prod"'
      - type: exclude_at_match
        name: drop_debug
        pattern: 'level=debug'
`)
	configs, err := ParseYAML(yamlConfig)
	require.NoError(t, err)
	require.Len(t, configs[0].ProcessingRules, 2)
	require.Equal(t, `attributes["env"] == "prod"`, configs[0].ProcessingRules[0].OTTL)
	require.Empty(t, configs[0].ProcessingRules[0].Pattern)
	require.Equal(t, "level=debug", configs[0].ProcessingRules[1].Pattern)
	require.Empty(t, configs[0].ProcessingRules[1].OTTL)

	jsonConfig := []byte(`[{"type":"file","path":"app.log","log_processing_rules":[{"type":"mask_sequences","name":"redact","ottl":"set(attributes[\"password\"], \"***\")"}]}]`)
	configs, err = ParseJSON(jsonConfig)
	require.NoError(t, err)
	require.Equal(t, MaskSequences, configs[0].ProcessingRules[0].Type)
	require.Equal(t, `set(attributes["password"], "***")`, configs[0].ProcessingRules[0].OTTL)
}
