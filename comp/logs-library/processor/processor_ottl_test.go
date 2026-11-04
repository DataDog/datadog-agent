// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestOTTLProcessingRules(t *testing.T) {
	for _, ruleType := range []string{config.ExcludeAtMatchOTTL, config.IncludeAtMatchOTTL} {
		for _, content := range []string{`{"level":"debug"}`, `{"level":"info"}`, `not JSON`, `{"level":"debug"} trailing`} {
			t.Run(ruleType+"/"+content, func(t *testing.T) {
				rules := []*config.ProcessingRule{{Type: ruleType, Name: "filter", Pattern: `attributes["level"] == "debug"`}}
				require.NoError(t, config.CompileProcessingRules(rules))
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
						keep = matched == (ruleType == config.IncludeAtMatchOTTL)
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
	global := []*config.ProcessingRule{{Type: config.IncludeAtMatchOTTL, Name: "before", Pattern: `attributes["message"] == "secret"`}}
	local := []*config.ProcessingRule{
		{Type: config.MaskSequences, Name: "mask", Pattern: "secret", ReplacePlaceholder: "redacted"},
		{Type: config.ExcludeAtMatchOTTL, Name: "after", Pattern: `attributes["message"] == "redacted"`},
	}
	require.NoError(t, config.CompileProcessingRules(global))
	require.NoError(t, config.CompileProcessingRules(local))
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
		{Type: config.IncludeAtMatchOTTL, Name: "before", Pattern: `HasPrefix(body, "secret")`},
		{Type: config.MaskSequences, Name: "mask", Pattern: "secret", ReplacePlaceholder: "redacted"},
		{Type: config.IncludeAtMatchOTTL, Name: "after", Pattern: `HasPrefix(body, "redacted")`},
	}
	require.NoError(t, config.CompileProcessingRules(rules))
	source := sources.NewLogSource("test", &config.LogsConfig{})
	p := &Processor{processingRules: rules}
	msg := newMessage([]byte("secret message"), source, "")
	require.True(t, p.applyRedactingRules(msg))
	require.Equal(t, "redacted message", string(msg.GetContent()))
}

// BenchmarkOTTLProcessor measures the actual global-plus-source rule dispatch,
// including context pooling and error handling, without encoding/network costs.
func BenchmarkOTTLProcessor(b *testing.B) {
	for _, ruleType := range []string{config.ExcludeAtMatch, config.ExcludeAtMatchOTTL} {
		for _, count := range []int{1, 5, 20} {
			b.Run(fmt.Sprintf("%s/rules=%d", ruleType, count), func(b *testing.B) {
				pattern := `"level"\s*:\s*"debug"`
				if ruleType == config.ExcludeAtMatchOTTL {
					pattern = `attributes["level"] == "debug"`
				}
				rules := make([]*config.ProcessingRule, count)
				for i := range rules {
					rules[i] = &config.ProcessingRule{Type: ruleType, Name: "filter", Pattern: pattern}
				}
				if err := config.CompileProcessingRules(rules); err != nil {
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
	global := []*config.ProcessingRule{{Type: config.IncludeAtMatchOTTL, Name: "production", Pattern: `attributes["env"] == "prod"`}}
	local := []*config.ProcessingRule{
		{Type: config.MaskSequences, Name: "mask", Pattern: "secret", ReplacePlaceholder: "redacted"},
		{Type: config.IncludeAtMatchOTTL, Name: "after_mask", Pattern: `attributes["message"] != "secret"`},
		{Type: config.ExcludeAtMatchOTTL, Name: "debug", Pattern: `attributes["level"] == "debug"`},
	}
	require.NoError(t, config.CompileProcessingRules(global))
	require.NoError(t, config.CompileProcessingRules(local))
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
