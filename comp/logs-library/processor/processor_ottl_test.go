// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package processor

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/processor/ottl"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestOTTLProcessingRules(t *testing.T) {
	for _, ruleType := range []string{config.ExcludeAtMatch, config.IncludeAtMatch} {
		for _, content := range []string{`{"level":"debug"}`, `{"level":"info"}`, `not JSON`, `{"level":"debug"} trailing`} {
			t.Run(ruleType+"/"+content, func(t *testing.T) {
				rules := []*config.ProcessingRule{{Type: ruleType, Name: "filter", OTTL: `attributes["level"] == "debug"`}}
				require.NoError(t, ottl.CompileProcessingRules(rules))
				source := sources.NewLogSource("test", &config.LogsConfig{ProcessingRules: rules})
				for _, newMsg := range []bool{false, true} {
					msg := newMessage([]byte(content), source, "")
					if newMsg {
						msg = newStructuredMessage([]byte(content), source, "")
					}
					p := &Processor{}
					keep := true
					if content == `{"level":"debug"}` || content == `{"level":"info"}` {
						matched := content == `{"level":"debug"}`
						keep = matched == (ruleType == config.IncludeAtMatch)
					}
					require.Equal(t, keep, p.applyRedactingRules(msg))
					if keep {
						require.Equal(t, content, string(msg.GetContent()))
					}
				}
			})
		}
	}
}

func TestOTTLProcessingRuleOrder(t *testing.T) {
	global := []*config.ProcessingRule{{Type: config.IncludeAtMatch, Name: "before", OTTL: `attributes["message"] == "secret"`}}
	local := []*config.ProcessingRule{
		{Type: config.MaskSequences, Name: "mask", Pattern: "secret", ReplacePlaceholder: "redacted"},
		{Type: config.ExcludeAtMatch, Name: "after", OTTL: `attributes["message"] == "redacted"`},
	}
	require.NoError(t, ottl.CompileProcessingRules(global))
	require.NoError(t, ottl.CompileProcessingRules(local))
	source := sources.NewLogSource("test", &config.LogsConfig{ProcessingRules: local})
	p := &Processor{processingRules: global}
	msg := newMessage([]byte(`{"message":"secret"}`), source, "")
	require.False(t, p.applyRedactingRules(msg), "the last rule must see the masked content")
	for _, rule := range []*config.ProcessingRule{global[0], local[0], local[1]} {
		require.EqualValues(t, 1, source.ProcessingInfo.GetCount(rule.Type+":"+rule.Name))
	}
}

func TestOTTLProcessingMaskInvalidatesBody(t *testing.T) {
	rules := []*config.ProcessingRule{
		{Type: config.IncludeAtMatch, Name: "before", OTTL: `HasPrefix(body, "secret")`},
		{Type: config.MaskSequences, Name: "mask", Pattern: "secret", ReplacePlaceholder: "redacted"},
		{Type: config.IncludeAtMatch, Name: "after", OTTL: `HasPrefix(body, "redacted")`},
	}
	require.NoError(t, ottl.CompileProcessingRules(rules))
	source := sources.NewLogSource("test", &config.LogsConfig{})
	p := &Processor{processingRules: rules}
	msg := newMessage([]byte("secret message"), source, "")
	require.True(t, p.applyRedactingRules(msg))
	require.Equal(t, "redacted message", string(msg.GetContent()))
}

// BenchmarkOTTLProcessor measures the actual global-plus-source rule dispatch,
// including context pooling and error handling, without encoding/network costs.
func BenchmarkOTTLProcessor(b *testing.B) {
	for _, language := range []string{"regex", "ottl"} {
		for _, count := range []int{1, 5, 20} {
			b.Run(fmt.Sprintf("%s/%s/rules=%d", config.ExcludeAtMatch, language, count), func(b *testing.B) {
				rules := make([]*config.ProcessingRule, count)
				for i := range rules {
					rules[i] = &config.ProcessingRule{Type: config.ExcludeAtMatch, Name: "filter", Pattern: `"level"\s*:\s*"debug"`}
					if language == "ottl" {
						rules[i] = &config.ProcessingRule{Type: config.ExcludeAtMatch, Name: "filter", OTTL: `attributes["level"] == "debug"`}
					}
				}
				if err := ottl.CompileProcessingRules(rules); err != nil {
					b.Fatal(err)
				}
				p := &Processor{processingRules: rules[:1]}
				source := sources.NewLogSource("test", &config.LogsConfig{ProcessingRules: rules[1:]})
				msg := newMessage([]byte(`{"level":"info","message":"request completed","http":{"status":200}}`), source, "")
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if !p.applyRedactingRules(msg) {
						b.Fatal("unexpectedly dropped message")
					}
				}
			})
		}
	}
}

// The synchronous processor now shares rule evaluation with the asynchronous
// processor. Exercise filtering, masking invalidation, and final encoding there.
func TestSyncProcessorOTTLProcessingRules(t *testing.T) {
	global := []*config.ProcessingRule{{Type: config.IncludeAtMatch, Name: "production", OTTL: `attributes["env"] == "prod"`}}
	local := []*config.ProcessingRule{
		{Type: config.MaskSequences, Name: "mask", Pattern: "secret", ReplacePlaceholder: "redacted"},
		{Type: config.IncludeAtMatch, Name: "after_mask", OTTL: `attributes["message"] != "secret"`},
		{Type: config.ExcludeAtMatch, Name: "debug", OTTL: `attributes["level"] == "debug"`},
	}
	require.NoError(t, ottl.CompileProcessingRules(global))
	require.NoError(t, ottl.CompileProcessingRules(local))
	source := sources.NewLogSource("test", &config.LogsConfig{ProcessingRules: local})
	processor := NewSyncProcessor(global, JSONEncoder, nil)

	for _, test := range []struct {
		name, input, output string
		keep                bool
	}{
		{"masked and encoded", `{"env":"prod","message":"secret","level":"info"}`, `{"env":"prod","message":"redacted","level":"info"}`, true},
		{"global exclusion", `{"env":"dev","message":"secret","level":"info"}`, "", false},
		{"source exclusion", `{"env":"prod","message":"secret","level":"debug"}`, "", false},
		{"errors continue through masking", "secret non-JSON", "redacted non-JSON", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			msg := newMessage([]byte(test.input), source, "")
			keep, err := processor.Process(msg)
			require.NoError(t, err)
			require.Equal(t, test.keep, keep)
			if keep {
				var encoded encodedLog
				require.NoError(t, json.Unmarshal(msg.GetContent(), &encoded))
				require.Equal(t, test.output, encoded.Message)
			}
		})
	}
}

func TestOTTLMaskProcessingRuleOrder(t *testing.T) {
	global := []*config.ProcessingRule{
		{Type: config.MaskSequences, Name: "global_mask", OTTL: `set(attributes["credentials"]["password"], "***")`},
		{Type: config.IncludeAtMatch, Name: "read_mask", OTTL: `attributes["credentials"]["password"] == "***"`},
	}
	local := []*config.ProcessingRule{
		{Type: config.MaskSequences, Name: "legacy_mask", Pattern: `\*\*\*`, ReplacePlaceholder: "redacted"},
		{Type: config.MaskSequences, Name: "source_mask", OTTL: `replace_pattern(attributes["message"], "token", "safe")`},
		{Type: config.IncludeAtMatch, Name: "legacy_filter", Pattern: `"message":"safe"`},
		{Type: config.IncludeAtMatch, Name: "read_both", OTTL: `attributes["credentials"]["password"] == "redacted" and attributes["message"] == "safe"`},
	}
	require.NoError(t, ottl.CompileProcessingRules(global))
	require.NoError(t, ottl.CompileProcessingRules(local))
	input := `{"credentials":{"password":"secret"},"message":"token"}`
	output := `{"credentials":{"password":"redacted"},"message":"safe"}`

	for _, mode := range []string{"unstructured", "structured", "synchronous"} {
		t.Run(mode, func(t *testing.T) {
			source := sources.NewLogSource("test", &config.LogsConfig{ProcessingRules: local})
			msg := newMessage([]byte(input), source, "")
			if mode == "structured" {
				msg = newStructuredMessage([]byte(input), source, "")
			}
			if mode == "synchronous" {
				keep, err := NewSyncProcessor(global, JSONEncoder, nil).Process(msg)
				require.NoError(t, err)
				require.True(t, keep)
				var encoded encodedLog
				require.NoError(t, json.Unmarshal(msg.GetContent(), &encoded))
				require.Equal(t, output, encoded.Message)
			} else {
				require.True(t, (&Processor{processingRules: global}).applyRedactingRules(msg))
				require.Equal(t, output, string(msg.GetContent()))
			}
			for _, rule := range append(append([]*config.ProcessingRule{}, global...), local...) {
				require.EqualValues(t, 1, source.ProcessingInfo.GetCount(rule.Type+":"+rule.Name))
			}
		})
	}
}

func TestOTTLMaskProcessingErrors(t *testing.T) {
	for _, test := range []struct {
		name, pattern, input, output string
		keep, recorded               bool
	}{
		{"malformed JSON drops", `set(attributes["password"], "***")`, "not JSON", "", false, true},
		{"failed where drops", `set(body, "***") where attributes["env"] == "prod"`, "not JSON", "", false, true},
		{"invalid array index drops", `set(attributes["items"][1], "***")`, `{"items":[]}`, "", false, true},
		{"raw body masks", `replace_pattern(body, "secret", "***")`, "secret text", "*** text", true, true},
		{"no match", `replace_pattern(attributes["message"], "secret", "***")`, ` { "message": "safe" } `, ` { "message": "safe" } `, true, false},
		{"false condition", `set(attributes["message"], "***") where false`, ` { "message": "secret" } `, ` { "message": "secret" } `, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rules := []*config.ProcessingRule{{Name: "mask", Type: config.MaskSequences, OTTL: test.pattern}}
			require.NoError(t, ottl.CompileProcessingRules(rules))
			for _, synchronous := range []bool{false, true} {
				source := sources.NewLogSource("test", &config.LogsConfig{ProcessingRules: rules})
				msg := newMessage([]byte(test.input), source, "")
				if synchronous {
					keep, err := NewSyncProcessor(nil, JSONEncoder, nil).Process(msg)
					require.NoError(t, err)
					require.Equal(t, test.keep, keep)
					if keep {
						var encoded encodedLog
						require.NoError(t, json.Unmarshal(msg.GetContent(), &encoded))
						require.Equal(t, test.output, encoded.Message)
					}
				} else {
					require.Equal(t, test.keep, (&Processor{}).applyRedactingRules(msg))
					if test.keep {
						require.Equal(t, test.output, string(msg.GetContent()))
					}
				}
				expected := 0
				if test.recorded {
					expected = 1
				}
				require.EqualValues(t, expected, source.ProcessingInfo.GetCount(config.MaskSequences+":mask"))
			}
		})
	}
}

func TestUncompiledOTTLRulesCannotBypassProcessing(t *testing.T) {
	for _, kind := range []string{config.IncludeAtMatch, config.ExcludeAtMatch, config.MaskSequences} {
		t.Run(kind, func(t *testing.T) {
			rules := []*config.ProcessingRule{{Name: "unprepared", Type: kind, OTTL: "true"}}
			source := sources.NewLogSource("test", &config.LogsConfig{ProcessingRules: rules})
			require.False(t, (&Processor{}).applyRedactingRules(newMessage([]byte("secret"), source, "")))
			require.Contains(t, source.Status().GetError(), "unprepared")
			keep, err := NewSyncProcessor(nil, JSONEncoder, nil).Process(newMessage([]byte("secret"), source, ""))
			require.ErrorContains(t, err, "unprepared")
			require.False(t, keep)
		})
	}
}
