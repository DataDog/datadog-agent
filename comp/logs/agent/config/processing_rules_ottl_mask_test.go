// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
)

func TestOTTLMaskStatements(t *testing.T) {
	for _, test := range []struct {
		name, input, pattern, output string
		changed, wantErr             bool
	}{
		{"nested regex", `{"user":{"message":"id=123"}}`, `replace_pattern(attributes["user"]["message"], "[0-9]+", "***")`, `{"user":{"message":"id=***"}}`, true, false},
		{"array element", `{"items":["secret","keep"]}`, `replace_match(attributes["items"][0], "sec*", "***")`, `{"items":["***","keep"]}`, true, false},
		{"dynamic field", `{"key":"token","token":"secret"}`, `set(attributes[attributes["key"]], "***")`, `{"key":"token","token":"***"}`, true, false},
		{"dynamic index", `{"index":1,"items":["keep","secret"]}`, `set(attributes["items"][attributes["index"]], "***")`, `{"index":1,"items":["keep","***"]}`, true, false},
		{"redact object", `{"credentials":{"password":"secret"}}`, `set(attributes["credentials"], "***")`, `{"credentials":"***"}`, true, false},
		{"capture group", `{"message":"user=123"}`, `replace_pattern(attributes["message"], "(user=)([0-9]+)", "$1***")`, `{"message":"user=***"}`, true, false},
		{"conditional match", `{"env":"prod","password":"secret"}`, `set(attributes["password"], "***") where attributes["env"] == "prod"`, `{"env":"prod","password":"***"}`, true, false},
		{"false condition", ` { "env": "dev", "password": "secret" } `, `set(attributes["password"], "***") where attributes["env"] == "prod"`, ` { "env": "dev", "password": "secret" } `, false, false},
		{"no match", ` { "message": "keep" } `, `replace_pattern(attributes["message"], "secret", "***")`, ` { "message": "keep" } `, false, false},
		{"same value", ` { "password": "***" } `, `set(attributes["password"], "***")`, ` { "password": "***" } `, false, false},
		{"missing target", `{}`, `replace_pattern(attributes["missing"], ".", "*")`, `{}`, false, false},
		{"raw body", "password=secret", `replace_pattern(body, "secret", "***")`, "password=***", true, false},
		{"log alias", `{"password":"secret"}`, `replace_match(log.attributes["password"], "*", "***")`, `{"password":"***"}`, true, false},
		{"whole body", "old", `set(log.body, "new")`, "new", true, false},
		{"non-string replace skips", `{"value":123}`, `replace_pattern(attributes["value"], ".", "*")`, `{"value":123}`, false, false},
		{"number redaction", `{"id":9007199254740993}`, `set(attributes["id"], "***")`, `{"id":"***"}`, true, false},
		{"preserve unrelated integers", `{"id":9007199254740993,"huge":9223372036854775808,"password":"secret"}`, `set(attributes["password"], "***")`, `{"id":9007199254740993,"huge":9223372036854775808,"password":"***"}`, true, false},
		{"null assignment is local", `{"a":true,"b":true,"c":null,"d":null}`, `set(attributes["c"], "masked")`, `{"a":true,"b":true,"c":"masked","d":null}`, true, false},
		{"boolean assignment is local", `{"a":true,"b":true}`, `set(attributes["a"], false)`, `{"a":false,"b":true}`, true, false},
		{"null redaction", `{"password":"secret"}`, `set(attributes["password"], nil)`, `{"password":null}`, true, false},
		{"integer assignment", `{"id":"secret"}`, `set(attributes["id"], 9007199254740993)`, `{"id":9007199254740993}`, true, false},
		{"double assignment", `{"id":"secret"}`, `set(attributes["id"], 1.0)`, `{"id":1.0}`, true, false},
		{"new scalar field", `{}`, `set(attributes["password"], "***")`, `{"password":"***"}`, true, false},
		{"already redacted duplicate", `{"password":"secret","password":"***"}`, `set(attributes["password"], "***")`, `{"password":"***"}`, true, false},
		{"no-match duplicate", `{"password":"secret","password":"safe"}`, `replace_pattern(attributes["password"], "secret", "***")`, `{"password":"safe"}`, true, false},
		{"false condition with duplicates", `{"password":"secret","password":"safe"}`, `set(attributes["password"], "***") where false`, `{"password":"secret","password":"safe"}`, false, false},
		{"duplicate key", `{"password":"first","password":"second"}`, `set(attributes["password"], "***")`, `{"password":"***"}`, true, false},
		{"escaped duplicate", `{"password":"first","pass\u0077ord":"second"}`, `set(attributes["password"], "***")`, `{"password":"***"}`, true, false},
		{"duplicate parent", `{"user":{"password":"first"},"user":{"password":"second"}}`, `set(attributes["user"]["password"], "***")`, `{"user":{"password":"***"}}`, true, false},
		{"escaping", `{"message":"old"}`, `set(attributes["message"], "line\n\"quoted\"\\")`, `{"message":"line\n\"quoted\"\\"}`, true, false},
		{"malformed JSON", "not JSON", `set(attributes["password"], "***")`, "not JSON", false, true},
		{"invalid escape", `{"password":"secret","other":"\x"}`, `set(attributes["password"], "***")`, `{"password":"secret","other":"\x"}`, false, true},
		{"missing parent", `{}`, `set(attributes["missing"]["password"], "***")`, `{}`, false, true},
		{"array bounds", `{"items":[]}`, `set(attributes["items"][0], "***")`, `{"items":[]}`, false, true},
		{"non-finite number", `{"x":1}`, `set(attributes["x"], Double("NaN"))`, `{"x":1}`, false, true},
		{"unsupported assignment", `{"x":"secret","obj":{}}`, `set(attributes["x"], attributes["obj"])`, `{"x":"secret","obj":{}}`, false, true},
		{"body requires string", "old", `set(body, 1)`, "old", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			statement, err := compileOTTLMaskStatement(test.pattern)
			require.NoError(t, err)
			c := AcquireOTTLContext([]byte(test.input))
			defer ReleaseOTTLContext(c)
			changed, err := c.ApplyMask(context.Background(), statement)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.changed, changed)
			require.Equal(t, test.output, string(c.Content()))
		})
	}
}

func TestOTTLMaskSharesParseAndDefersEncoding(t *testing.T) {
	input := []byte(` { "a": "secret", "b": "token" } `)
	c := AcquireOTTLContext(input)
	defer ReleaseOTTLContext(c)
	condition, err := compileOTTLCondition(`attributes["a"] == "secret"`)
	require.NoError(t, err)
	match, err := condition.Eval(context.Background(), c)
	require.NoError(t, err)
	require.True(t, match)
	root := c.root

	for _, pattern := range []string{`set(attributes["a"], "A")`, `set(attributes["b"], "B")`} {
		statement, err := compileOTTLMaskStatement(pattern)
		require.NoError(t, err)
		changed, err := c.ApplyMask(context.Background(), statement)
		require.NoError(t, err)
		require.True(t, changed)
		require.Same(t, root, c.root)
		require.Equal(t, input, c.content, "adjacent attribute masks must not serialize between rules")
		require.True(t, c.jsonDirty)
	}
	condition, err = compileOTTLCondition(`attributes["a"] == "A" and String(attributes["b"]) == "B" and Len(attributes) == 2`)
	require.NoError(t, err)
	match, err = condition.Eval(context.Background(), c)
	require.NoError(t, err)
	require.True(t, match)
	output := c.Content()
	require.Equal(t, `{"a":"A","b":"B"}`, string(output))
	require.False(t, c.jsonDirty)
	require.Same(t, &output[0], &c.Content()[0])

	statement, err := compileOTTLMaskStatement(`replace_pattern(body, "A", "changed")`)
	require.NoError(t, err)
	changed, err := c.ApplyMask(context.Background(), statement)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, c.parsed, "a body edit invalidates the JSON tree")
	condition, err = compileOTTLCondition(`attributes["a"] == "changed" and attributes["b"] == "B"`)
	require.NoError(t, err)
	match, err = condition.Eval(context.Background(), c)
	require.NoError(t, err)
	require.True(t, match)
}

func TestOTTLMaskOutputSurvivesContextReuse(t *testing.T) {
	statement, err := compileOTTLMaskStatement(`set(attributes["password"], "***")`)
	require.NoError(t, err)
	c := AcquireOTTLContext([]byte(`{"password":"secret"}`))
	changed, err := c.ApplyMask(context.Background(), statement)
	require.NoError(t, err)
	require.True(t, changed)
	output := c.Content()
	for range 10 {
		c.Reset([]byte(`{"password":"other","padding":"new message"}`))
		_, err := c.ApplyMask(context.Background(), statement)
		require.NoError(t, err)
		_ = c.Content()
	}
	ReleaseOTTLContext(c)
	require.Equal(t, `{"password":"***"}`, string(output))
}

func TestOTTLMaskStatementConcurrentUse(t *testing.T) {
	statement, err := compileOTTLMaskStatement(`replace_pattern(attributes["message"], "secret", "***")`)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				c := AcquireOTTLContext([]byte(`{"message":"secret"}`))
				changed, err := c.ApplyMask(context.Background(), statement)
				output := string(c.Content())
				ReleaseOTTLContext(c)
				if err != nil || !changed || output != `{"message":"***"}` {
					t.Errorf("unexpected masking result: changed=%v err=%v output=%s", changed, err, output)
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestOTTLMaskConfiguration(t *testing.T) {
	pattern := `set(attributes["password"], "***")`
	cfg := coreconfig.NewMock(t)
	cfg.SetInTest("logs_config.processing_rules", []map[string]any{{"type": MaskOTTLTransform, "name": "redact", "pattern": pattern}})
	global, err := GlobalProcessingRules(cfg)
	require.NoError(t, err)
	require.NotNil(t, global[0].OTTLStatement)

	sources, err := ParseYAML([]byte("logs:\n  - type: file\n    path: app.log\n    log_processing_rules:\n      - type: mask_ottl\n        name: redact\n        pattern: '" + pattern + "'\n"))
	require.NoError(t, err)
	require.NoError(t, sources[0].Validate())
	require.NotNil(t, sources[0].ProcessingRules[0].OTTLStatement)

	for _, pattern := range []string{"", `attributes["password"] == "secret"`, `replace_pattern(body, "[", "***")`, `delete_key(attributes, "password")`, `set(body, "x") where replace_pattern(body, ".", "*")`} {
		rules := []*ProcessingRule{{Name: "invalid", Type: MaskOTTLTransform, Pattern: pattern}}
		require.Error(t, ValidateProcessingRules(rules), pattern)
		require.Error(t, CompileProcessingRules(rules), pattern)
	}
}
