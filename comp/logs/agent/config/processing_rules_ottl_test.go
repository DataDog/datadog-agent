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

func TestOTTLConditions(t *testing.T) {
	for _, test := range []struct {
		name, pattern, content string
		match, wantErr         bool
	}{
		{"nested field", `attributes["a1"]["status"] == "error"`, `{"a1":{"status":"ok"},"decoy":{"status":"error"}}`, false, false},
		{"nested match", `attributes["a1"]["status"] == "error"`, `{"a1":{"status":"error"}}`, true, false},
		{"array", `attributes["events"][1]["status"] == 503`, `{"events":[{}, {"status":503}]}`, true, false},
		{"missing field", `attributes["absent"] == nil`, `{}`, true, false},
		{"missing parent", `attributes["absent"]["child"] == nil`, `{}`, true, false},
		{"null", `attributes["value"] == nil`, `{"value":null}`, true, false},
		{"boolean", `attributes["flag"] == true`, `{"flag":true}`, true, false},
		{"double", `attributes["value"] > 1.5`, `{"value":1.75}`, true, false},
		{"exponent", `attributes["value"] == 150.0`, `{"value":1.5e2}`, true, false},
		{"integer precision", `attributes["id"] == 9007199254740993`, `{"id":9007199254740993}`, true, false},
		{"integer mismatch", `attributes["id"] == 9007199254740992`, `{"id":9007199254740993}`, false, false},
		{"int64 boundary", `attributes["id"] == 9223372036854775807`, `{"id":9223372036854775807}`, true, false},
		{"integer overflow", `attributes["id"] > 0`, `{"id":9223372036854775808}`, false, true},
		{"special key", `attributes["a.b"]["x[y]"] == "ok"`, `{"a.b":{"x[y]":"ok"}}`, true, false},
		{"escaped string", `attributes["message"] == "hello\nworld"`, `{"message":"hello\nworld"}`, true, false},
		{"unicode", `attributes["name"] == "café"`, `{"name":"caf\u00e9"}`, true, false},
		{"dynamic key", `attributes[attributes["key"]] == "ok"`, `{"key":"status","status":"ok"}`, true, false},
		{"dynamic index", `attributes["items"][attributes["index"]] == "ok"`, `{"index":1,"items":["no","ok"]}`, true, false},
		{"invalid dynamic key", `attributes[attributes["key"]] == nil`, `{"key":true}`, false, true},
		{"negative index", `attributes["items"][-1] == nil`, `{"items":[1]}`, false, true},
		{"out of bounds", `attributes["items"][5] == nil`, `{"items":[1]}`, false, true},
		{"wrong index type", `attributes["items"]["0"] == 1`, `{"items":[1]}`, false, true},
		{"wrong object type", `attributes["items"][0] == 1`, `{"items":{"0":1}}`, false, true},
		{"object length", `Len(attributes["http"]) == 1`, `{"http":{"status":200}}`, true, false},
		{"array length", `Len(attributes["items"]) == 2`, `{"items":[1,2]}`, true, false},
		{"map type", `IsMap(attributes["http"])`, `{"http":{"status":200}}`, true, false},
		{"list type", `IsList(attributes["items"])`, `{"items":[1,2]}`, true, false},
		{"type guard", `IsString(attributes["status"]) and HasPrefix(attributes["status"], "err")`, `{"status":12}`, false, false},
		{"regex", `IsMatch(attributes["message"], "^hello")`, `{"message":"hello world"}`, true, false},
		{"conversion", `Int(attributes["status"]) >= 500`, `{"status":"503"}`, true, false},
		{"raw body", `HasSuffix(body, "world")`, `hello world`, true, false},
		{"log prefix", `log.attributes["status"] == "ok" and HasPrefix(log.body, "{")`, `{"status":"ok"}`, true, false},
		{"non-JSON", `attributes["level"] == "info"`, `hello world`, false, true},
		{"duplicate key", `attributes["level"] == "info"`, `{"level":"debug","level":"info"}`, true, false},
		{"escaped duplicate key", `attributes["level"] == "info"`, `{"level":"debug","le\u0076el":"info"}`, true, false},
		{"invalid escape", `attributes["level"] == "info"`, `{"level":"info","message":"\x"}`, false, true},
		{"invalid number", `attributes["level"] == "info"`, `{"level":"info","value":01}`, false, true},
		{"invalid exponent", `attributes["level"] == "info"`, `{"level":"info","value":1e}`, false, true},
		{"trailing data", `attributes["level"] == "info"`, `{"level":"info"} {}`, false, true},
		{"non-object", `attributes["level"] == nil`, `[1,2]`, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			condition, err := compileOTTLCondition(test.pattern)
			require.NoError(t, err)
			c := AcquireOTTLContext([]byte(test.content))
			defer ReleaseOTTLContext(c)
			match, err := condition.Eval(context.Background(), c)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.match, match)
			}
		})
	}
}

func TestOTTLRuleValidation(t *testing.T) {
	for _, pattern := range []string{"", `attributes["level"] ==`, `resource.attributes["level"] == "info"`, `attributes.level == "info"`, `Unknown(body)`, `set(body, "changed")`} {
		for _, ruleType := range []string{ExcludeAtMatchOTTL, IncludeAtMatchOTTL} {
			rule := &ProcessingRule{Name: "test", Type: ruleType, Pattern: pattern}
			require.Error(t, ValidateProcessingRules([]*ProcessingRule{rule}), pattern)
			require.Error(t, CompileProcessingRules([]*ProcessingRule{rule}), pattern)
		}
	}
	rule := &ProcessingRule{Name: "test", Type: IncludeAtMatchOTTL, Pattern: `attributes["level"] == "info"`}
	require.NoError(t, ValidateProcessingRules([]*ProcessingRule{rule}))
	require.NoError(t, CompileProcessingRules([]*ProcessingRule{rule}))
	require.NotNil(t, rule.OTTLCondition)
	require.Nil(t, rule.Regex)
}

func TestOTTLContextReset(t *testing.T) {
	condition, err := compileOTTLCondition(`attributes["level"] == "info"`)
	require.NoError(t, err)
	c := AcquireOTTLContext([]byte(`{"level":"info"}`))
	defer ReleaseOTTLContext(c)
	for _, content := range []string{`{"level":"info"}`, `invalid`, `{"level":"debug"}`, `{"level":"info"}`} {
		c.Reset([]byte(content))
		match, err := condition.Eval(context.Background(), c)
		if content == "invalid" {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, content == `{"level":"info"}`, match)
		}
	}
}

func TestOTTLConditionConcurrentUse(t *testing.T) {
	condition, err := compileOTTLCondition(`attributes["level"] == "info"`)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			content := []byte(`{"level":"info"}`)
			if i%2 != 0 {
				content = []byte(`{"level":"debug"}`)
			}
			for range 100 {
				c := AcquireOTTLContext(content)
				match, evalErr := condition.Eval(context.Background(), c)
				ReleaseOTTLContext(c)
				if evalErr != nil || match != (i%2 == 0) {
					t.Errorf("unexpected result: match=%v err=%v", match, evalErr)
					return
				}
			}
		})
	}
	wg.Wait()
}

// Exercise the configuration paths used by datadog.yaml and autodiscovery,
// rather than only constructing compiled processing rules directly.
func TestOTTLGlobalProcessingRules(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{"map", []map[string]any{{"type": IncludeAtMatchOTTL, "name": "keep_prod", "pattern": `attributes["env"] == "prod"`}}},
		{"JSON string", `[{"type":"include_at_match_ottl","name":"keep_prod","pattern":"attributes[\"env\"] == \"prod\""}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := coreconfig.NewMock(t)
			cfg.SetInTest("logs_config.processing_rules", test.value)
			rules, err := GlobalProcessingRules(cfg)
			require.NoError(t, err)
			require.Len(t, rules, 1)
			require.Equal(t, IncludeAtMatchOTTL, rules[0].Type)
			require.NotNil(t, rules[0].OTTLCondition)
			c := AcquireOTTLContext([]byte(`{"env":"prod"}`))
			defer ReleaseOTTLContext(c)
			matched, err := rules[0].OTTLCondition.Eval(context.Background(), c)
			require.NoError(t, err)
			require.True(t, matched)
		})
	}
}

func TestOTTLSourceProcessingRules(t *testing.T) {
	for name, parse := range map[string]func([]byte) ([]*LogsConfig, error){"YAML": ParseYAML, "JSON": ParseJSON} {
		t.Run(name, func(t *testing.T) {
			input := `logs:
  - type: file
    path: app.log
    log_processing_rules:
      - type: exclude_at_match_ottl
        name: drop_errors
        pattern: 'attributes["http"]["status"] >= 500'
`
			if name == "JSON" {
				input = `[{"type":"file","path":"app.log","log_processing_rules":[{"type":"exclude_at_match_ottl","name":"drop_errors","pattern":"attributes[\"http\"][\"status\"] >= 500"}]}]`
			}
			configs, err := parse([]byte(input))
			require.NoError(t, err)
			require.Len(t, configs, 1)
			require.NoError(t, configs[0].Validate())
			require.Len(t, configs[0].ProcessingRules, 1)
			condition := configs[0].ProcessingRules[0].OTTLCondition
			require.NotNil(t, condition)
			c := AcquireOTTLContext([]byte(`{"http":{"status":503}}`))
			defer ReleaseOTTLContext(c)
			matched, err := condition.Eval(context.Background(), c)
			require.NoError(t, err)
			require.True(t, matched)
		})
	}
}

func TestOTTLConfigurationRejectsInvalidCondition(t *testing.T) {
	cfg := coreconfig.NewMock(t)
	cfg.SetInTest("logs_config.processing_rules", []map[string]any{{"type": ExcludeAtMatchOTTL, "name": "invalid", "pattern": `attributes["http"] ==`}})
	_, err := GlobalProcessingRules(cfg)
	require.ErrorContains(t, err, "invalid OTTL condition")
	source := &LogsConfig{Type: FileType, Path: "app.log", ProcessingRules: []*ProcessingRule{{Type: IncludeAtMatchOTTL, Name: "invalid", Pattern: `attributes["http"] ==`}}}
	require.ErrorContains(t, source.Validate(), "invalid OTTL condition")
}
