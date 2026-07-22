// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type processorTestCase struct {
	source        sources.LogSource
	input         []byte
	output        []byte
	shouldProcess bool
	matchCount    int64
	ruleType      string
}

// exclusions tests
// ----------------
var exclusionRuleType = "exclude_at_match"
var exclusionTests = []processorTestCase{
	{
		source:        newSource(exclusionRuleType, "", "world"),
		input:         []byte("hello"),
		output:        []byte("hello"),
		shouldProcess: true,
		matchCount:    0,
		ruleType:      exclusionRuleType,
	},
	{
		source:        newSource(exclusionRuleType, "", "world"),
		input:         []byte("world"),
		output:        []byte{},
		shouldProcess: false,
		matchCount:    1,
		ruleType:      exclusionRuleType,
	},
	{
		source:        newSource(exclusionRuleType, "", "world"),
		input:         []byte("a brand new world"),
		output:        []byte{},
		shouldProcess: false,
		matchCount:    1,
		ruleType:      exclusionRuleType,
	},
	{
		source:        newSource(exclusionRuleType, "", "$world"),
		input:         []byte("a brand new world"),
		output:        []byte("a brand new world"),
		shouldProcess: true,
		matchCount:    0,
		ruleType:      exclusionRuleType,
	},
}

func TestExclusion(t *testing.T) {
	p := &Processor{}
	assert := assert.New(t)

	// unstructured messages

	for idx, test := range exclusionTests {
		msg := newMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		assert.Equal(test.matchCount, msg.Origin.LogSource.ProcessingInfo.GetCount(test.ruleType+":"+ruleName), "match count should be equal for test %d", idx)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
		msg.Origin.LogSource.ProcessingInfo.Reset()
	}

	// structured messages

	for idx, test := range exclusionTests {
		msg := newStructuredMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		assert.Equal(test.matchCount, msg.Origin.LogSource.ProcessingInfo.GetCount(test.ruleType+":"+ruleName), "match count should be equal for test %d", idx)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
		msg.Origin.LogSource.ProcessingInfo.Reset()
	}
}

// inclusion tests
// ---------------

var inclusionRuleType = "include_at_match"
var inclusionTests = []processorTestCase{
	{
		source:        *sources.NewLogSource("", &config.LogsConfig{}),
		input:         []byte("hello"),
		output:        []byte("hello"),
		shouldProcess: false,
		matchCount:    0,
		ruleType:      inclusionRuleType,
	},
	{
		source:        *sources.NewLogSource("", &config.LogsConfig{}),
		input:         []byte("world"),
		output:        []byte("world"),
		shouldProcess: true,
		matchCount:    1,
		ruleType:      inclusionRuleType,
	},
	{
		source:        *sources.NewLogSource("", &config.LogsConfig{}),
		input:         []byte("a brand new world"),
		output:        []byte("a brand new world"),
		shouldProcess: true,
		matchCount:    1,
	},
	{
		source:        newSource(inclusionRuleType, "", "^world"),
		input:         []byte("a brand new world"),
		output:        []byte("a brand new world"),
		shouldProcess: false,
		matchCount:    1,
	},
}

func TestInclusion(t *testing.T) {
	p := &Processor{processingRules: []*config.ProcessingRule{newProcessingRule(inclusionRuleType, "", "world")}}
	assert := assert.New(t)

	// unstructured messages

	for idx, test := range inclusionTests {
		msg := newMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		assert.Equal(test.matchCount, msg.Origin.LogSource.ProcessingInfo.GetCount(inclusionRuleType+":"+ruleName), "match count should be equal for test %d", idx)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
		msg.Origin.LogSource.ProcessingInfo.Reset()
	}

	// structured messages

	for idx, test := range inclusionTests {
		msg := newStructuredMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		assert.Equal(test.matchCount, msg.Origin.LogSource.ProcessingInfo.GetCount(inclusionRuleType+":"+ruleName), "match count should be equal for test %d", idx)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
		msg.Origin.LogSource.ProcessingInfo.Reset()
	}

}

// exclusion with inclusion tests
// ------------------------------

var exclusionRule *config.ProcessingRule = newProcessingRule(exclusionRuleType, "", "^bob")
var inclusionRule *config.ProcessingRule = newProcessingRule(inclusionRuleType, "", ".*@datadoghq.com$")

var exclusionInclusionTests = []processorTestCase{
	{
		source:        *sources.NewLogSource("", &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{inclusionRule}}),
		input:         []byte("bob@datadoghq.com"),
		output:        []byte("bob@datadoghq.com"),
		shouldProcess: false,
		matchCount:    1,
		ruleType:      exclusionRuleType,
	},
	{
		source:        *sources.NewLogSource("", &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{inclusionRule}}),
		input:         []byte("bill@datadoghq.com"),
		output:        []byte("bill@datadoghq.com"),
		shouldProcess: true,
		matchCount:    1,
		ruleType:      inclusionRuleType,
	},
	{
		source:        *sources.NewLogSource("", &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{inclusionRule}}),
		input:         []byte("bob@amail.com"),
		output:        []byte("bob@amail.com"),
		shouldProcess: false,
		matchCount:    1,
		ruleType:      exclusionRuleType,
	},
	{
		source:        *sources.NewLogSource("", &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{inclusionRule}}),
		input:         []byte("bill@amail.com"),
		output:        []byte("bill@amail.com"),
		shouldProcess: false,
		matchCount:    0,
		ruleType:      inclusionRuleType,
	},
}

func TestExclusionWithInclusion(t *testing.T) {
	p := &Processor{processingRules: []*config.ProcessingRule{exclusionRule}}
	assert := assert.New(t)

	// unstructured messages

	for idx, test := range exclusionInclusionTests {
		msg := newMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		assert.Equal(test.matchCount, msg.Origin.LogSource.ProcessingInfo.GetCount(test.ruleType+":"+ruleName), "match count should be equal for test %d", idx)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
		msg.Origin.LogSource.ProcessingInfo.Reset()
	}

	// structured messages

	for idx, test := range exclusionInclusionTests {
		msg := newStructuredMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		assert.Equal(test.matchCount, msg.Origin.LogSource.ProcessingInfo.GetCount(test.ruleType+":"+ruleName), "match count should be equal for test %d", idx)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
		msg.Origin.LogSource.ProcessingInfo.Reset()
	}
}

// mask_sequences test cases
// -------------------------
var maskSequenceRule = "mask_sequences"
var masksTests = []processorTestCase{
	{
		source:        newSource(maskSequenceRule, "[masked_world]", "world"),
		input:         []byte("hello"),
		output:        []byte("hello"),
		shouldProcess: true,
		matchCount:    0,
		ruleType:      maskSequenceRule,
	},
	{
		source:        newSource(maskSequenceRule, "[masked_world]", "world"),
		input:         []byte("hello world!"),
		output:        []byte("hello [masked_world]!"),
		shouldProcess: true,
		matchCount:    1,
		ruleType:      maskSequenceRule,
	},
	{
		source:        newSource(maskSequenceRule, "[masked_user]", "User=\\w+@datadoghq.com"),
		input:         []byte("new test launched by User=beats@datadoghq.com on localhost"),
		output:        []byte("new test launched by [masked_user] on localhost"),
		shouldProcess: true,
		matchCount:    1,
		ruleType:      maskSequenceRule,
	},
	{
		source:        newSource(maskSequenceRule, "[masked_credit_card]", "(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\\d{3})\\d{11})"),
		input:         []byte("The credit card 4323124312341234 was used to buy some time"),
		output:        []byte("The credit card [masked_credit_card] was used to buy some time"),
		shouldProcess: true,
		matchCount:    1,
		ruleType:      maskSequenceRule,
	},
	{
		source:        newSource(maskSequenceRule, "${1}[masked_value]", "([Dd]ata_?values=)\\S+"),
		input:         []byte("New data added to Datavalues=123456 on prod"),
		output:        []byte("New data added to Datavalues=[masked_value] on prod"),
		shouldProcess: true,
		matchCount:    1,
		ruleType:      maskSequenceRule,
	},
	{
		source:        newSource(maskSequenceRule, "${1}[masked_value]", "([Dd]ata_?values=)\\S+"),
		input:         []byte("New data added to data_values=123456 on prod"),
		output:        []byte("New data added to data_values=[masked_value] on prod"),
		shouldProcess: true,
		matchCount:    1,
		ruleType:      maskSequenceRule,
	},
	{
		source:        newSource(maskSequenceRule, "${1}[masked_value]", "([Dd]ata_?values=)\\S+"),
		input:         []byte("New data added to data_values= on prod"),
		output:        []byte("New data added to data_values= on prod"),
		shouldProcess: true,
		matchCount:    0,
		ruleType:      maskSequenceRule,
	},
}

func TestMask(t *testing.T) {
	p := &Processor{}
	assert := assert.New(t)

	// unstructured messages

	for idx, maskTest := range masksTests {
		msg := newMessage(maskTest.input, &maskTest.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(maskTest.shouldProcess, shouldProcess)
		assert.Equal(maskTest.matchCount, msg.Origin.LogSource.ProcessingInfo.GetCount(maskSequenceRule+":"+ruleName), "match count should be equal for test %d", idx)
		if maskTest.shouldProcess {
			assert.Equal(maskTest.output, msg.GetContent())
		}
		msg.Origin.LogSource.ProcessingInfo.Reset()
	}

	// structured messages

	for idx, maskTest := range masksTests {
		msg := newStructuredMessage(maskTest.input, &maskTest.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(maskTest.shouldProcess, shouldProcess)
		assert.Equal(maskTest.matchCount, msg.Origin.LogSource.ProcessingInfo.GetCount(maskSequenceRule+":"+ruleName), "match count should be equal for test %d", idx)
		if maskTest.shouldProcess {
			assert.Equal(maskTest.output, msg.GetContent())
		}
		msg.Origin.LogSource.ProcessingInfo.Reset()
	}
}

func TestTruncate(t *testing.T) {
	p := &Processor{}
	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := newMessage([]byte("hello"), source, "")
	_ = p.applyRedactingRules(msg)
	assert.Equal(t, []byte("hello"), msg.GetContent())
}

func TestGetHostname(t *testing.T) {
	hostnameComponent, _ := hostnameinterface.NewMock("testHostnameFromEnvVar")
	p := &Processor{
		hostname: hostnameComponent,
	}
	m := message.NewMessage([]byte("hello"), nil, "", 0)
	assert.Equal(t, "testHostnameFromEnvVar", p.GetHostname(m))
}

// helpers
// -

var ruleName = "test"

func newProcessingRule(ruleType, replacePlaceholder, pattern string) *config.ProcessingRule {
	return &config.ProcessingRule{
		Type:               ruleType,
		Name:               ruleName,
		ReplacePlaceholder: replacePlaceholder,
		Placeholder:        []byte(replacePlaceholder),
		Pattern:            pattern,
		Regex:              regexp.MustCompile(pattern),
	}
}

func newSource(ruleType, replacePlaceholder, pattern string) sources.LogSource {
	return *sources.NewLogSource("", &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{newProcessingRule(ruleType, replacePlaceholder, pattern)}})
}

func newMessage(content []byte, source *sources.LogSource, status string) *message.Message {
	return message.NewMessageWithSource(content, status, source, 0)
}

func newStructuredMessage(content []byte, source *sources.LogSource, status string) *message.Message {
	structuredContent := message.BasicStructuredContent{
		Data: make(map[string]interface{}),
	}
	msg := message.NewStructuredMessage(&structuredContent, message.NewOrigin(source), status, 0)
	msg.SetContent(content)
	return msg
}

func newJQSource(ruleType, pattern string) sources.LogSource {
	rule := &config.ProcessingRule{
		Type:    ruleType,
		Name:    ruleName,
		Pattern: pattern,
	}
	if err := config.CompileProcessingRules([]*config.ProcessingRule{rule}); err != nil {
		panic(err)
	}
	return *sources.NewLogSource("", &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{rule}})
}

func TestJQExclusion(t *testing.T) {
	p := &Processor{}
	tests := []struct {
		name          string
		pattern       string
		input         []byte
		shouldProcess bool
	}{
		{
			name:          "drops matching JSON",
			pattern:       `select(.level == "debug")`,
			input:         []byte(`{"level":"debug","msg":"verbose"}`),
			shouldProcess: false,
		},
		{
			name:          "passes non-matching JSON",
			pattern:       `select(.level == "debug")`,
			input:         []byte(`{"level":"error","msg":"boom"}`),
			shouldProcess: true,
		},
		{
			name:          "passes non-JSON content unchanged",
			pattern:       `select(.level == "debug")`,
			input:         []byte("plain text log line"),
			shouldProcess: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newJQSource(config.ExcludeAtJQMatch, tt.pattern)
			msg := newMessage(tt.input, &src, "")
			assert.Equal(t, tt.shouldProcess, p.applyRedactingRules(msg))
		})
	}
}

func TestJQInclusion(t *testing.T) {
	p := &Processor{}
	tests := []struct {
		name          string
		pattern       string
		input         []byte
		shouldProcess bool
	}{
		{
			name:          "keeps matching JSON",
			pattern:       `select(.level == "error")`,
			input:         []byte(`{"level":"error","msg":"boom"}`),
			shouldProcess: true,
		},
		{
			name:          "drops non-matching JSON",
			pattern:       `select(.level == "error")`,
			input:         []byte(`{"level":"debug","msg":"verbose"}`),
			shouldProcess: false,
		},
		{
			name:          "passes non-JSON content unchanged",
			pattern:       `select(.level == "error")`,
			input:         []byte("plain text log line"),
			shouldProcess: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newJQSource(config.IncludeAtJQMatch, tt.pattern)
			msg := newMessage(tt.input, &src, "")
			assert.Equal(t, tt.shouldProcess, p.applyRedactingRules(msg))
		})
	}
}

func TestJQMask(t *testing.T) {
	p := &Processor{}

	t.Run("masks matching content", func(t *testing.T) {
		src := newJQSource(config.MaskJQTransform, `.message |= gsub("(?<num>[0-9]+)"; "[REDACTED-\(.num)]")`)
		msg := newMessage([]byte(`{"message":"user 123456 logged in"}`), &src, "")
		assert.True(t, p.applyRedactingRules(msg))
		assert.JSONEq(t, `{"message":"user [REDACTED-123456] logged in"}`, string(msg.GetContent()))
	})

	t.Run("fails closed and drops the message on non-JSON input", func(t *testing.T) {
		src := newJQSource(config.MaskJQTransform, `.message |= gsub("[0-9]+"; "X")`)
		msg := newMessage([]byte("not json"), &src, "")
		assert.False(t, p.applyRedactingRules(msg))
	})

	t.Run("fails closed and drops the message when the program produces no output", func(t *testing.T) {
		src := newJQSource(config.MaskJQTransform, `empty`)
		msg := newMessage([]byte(`{"message":"hello"}`), &src, "")
		assert.False(t, p.applyRedactingRules(msg))
	})

	t.Run("mask before exclude sees the masked content", func(t *testing.T) {
		var seen []byte
		maskRule := &config.ProcessingRule{
			Type:        config.MaskJQTransform,
			Name:        ruleName,
			JQTransform: func([]byte) ([]byte, error) { return []byte(`{"message":"masked"}`), nil },
		}
		excludeRule := &config.ProcessingRule{
			Type: config.ExcludeAtJQMatch,
			Name: ruleName,
			JQFilter: func(input []byte) (bool, error) {
				seen = input
				return false, nil
			},
		}
		src := *sources.NewLogSource("", &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{maskRule, excludeRule}})
		msg := newMessage([]byte(`{"message":"original"}`), &src, "")
		assert.True(t, p.applyRedactingRules(msg))
		assert.Equal(t, []byte(`{"message":"masked"}`), seen)
	})

	t.Run("masked output sorts object keys alphabetically", func(t *testing.T) {
		src := newJQSource(config.MaskJQTransform, `.`)
		msg := newMessage([]byte(`{"z":1,"a":2}`), &src, "")
		assert.True(t, p.applyRedactingRules(msg))
		assert.Equal(t, []byte(`{"a":2,"z":1}`), msg.GetContent())
	})
}

func BenchmarkMaskSequences(b *testing.B) {
	processor := &Processor{
		processingRules: []*config.ProcessingRule{
			{
				Type:               config.MaskSequences,
				Regex:              regexp.MustCompile("(?:api_key=[a-f0-9]{28})"),
				ReplacePlaceholder: "api_key=****************************",
			},
		},
	}

	msg := newMessage(nil, &sources.LogSource{
		Config: &config.LogsConfig{},
	}, "")

	b.Run("always matching", func(b *testing.B) {
		// what we benchmark here is the worse case scenario where the regex matches every time
		content := []byte("[1234] log message, api_key=1234567890123456789012345678")
		b.ResetTimer()

		for range b.N {
			msg.SetContent(content)
			processor.applyRedactingRules(msg)
		}
	})

	b.Run("never matching", func(b *testing.B) {
		// what we benchmark here is the best case scenario where the regex never matches
		content := []byte("nothing to see here")
		b.ResetTimer()

		for range b.N {
			msg.SetContent(content)
			processor.applyRedactingRules(msg)
		}
	})

}

func TestExcludeTruncated(t *testing.T) {
	p := &Processor{}
	assert := assert.New(t)

	ruleType := config.ExcludeTruncated
	source := newSource(ruleType, "", "")

	// A non-truncated message should be processed
	msg1 := newMessage([]byte("hello"), &source, "")
	msg1.IsTruncated = false
	shouldProcess1 := p.applyRedactingRules(msg1)
	assert.True(shouldProcess1)
	assert.Equal(int64(0), msg1.Origin.LogSource.ProcessingInfo.GetCount(ruleType+":"+ruleName))

	// A truncated message should not be processed
	msg2 := newMessage([]byte("hello"), &source, "")
	msg2.IsTruncated = true
	shouldProcess2 := p.applyRedactingRules(msg2)
	assert.False(shouldProcess2)
	assert.Equal(int64(1), msg2.Origin.LogSource.ProcessingInfo.GetCount(ruleType+":"+ruleName))
}

func TestRemapSource(t *testing.T) {
	assert := assert.New(t)

	t.Run("matching mapping remaps source", func(_ *testing.T) {
		source := sources.NewLogSource("", &config.LogsConfig{
			Source: "syslog_fallback",
			ProcessingRules: []*config.ProcessingRule{{
				Type: config.RemapSource,
				Name: "siem_remap",
				Matching: []*config.SourceMatchEntry{
					{Attribute: "siem.device_vendor", Value: "Security", NewSource: "arcsight"},
					{Attribute: "siem.device_product", Value: "palo alto", NewSource: "pan"},
				},
			}},
		})
		sc := &message.BasicStructuredContent{Data: map[string]interface{}{
			"message": "test",
			"siem": map[string]interface{}{
				"device_vendor": "Security",
			},
		}}
		msg := message.NewStructuredMessage(sc, message.NewOrigin(source), "", 0)

		p := &Processor{}
		shouldProcess := p.applyRedactingRules(msg)
		assert.True(shouldProcess)
		assert.Equal("arcsight", msg.Origin.Source())
	})

	t.Run("second mapping matches", func(_ *testing.T) {
		source := sources.NewLogSource("", &config.LogsConfig{
			Source: "syslog_fallback",
			ProcessingRules: []*config.ProcessingRule{{
				Type: config.RemapSource,
				Name: "siem_remap",
				Matching: []*config.SourceMatchEntry{
					{Attribute: "siem.device_vendor", Value: "Security", NewSource: "arcsight"},
					{Attribute: "siem.device_product", Value: "palo alto", NewSource: "pan"},
				},
			}},
		})
		sc := &message.BasicStructuredContent{Data: map[string]interface{}{
			"message": "test",
			"siem": map[string]interface{}{
				"device_product": "palo alto",
			},
		}}
		msg := message.NewStructuredMessage(sc, message.NewOrigin(source), "", 0)

		p := &Processor{}
		shouldProcess := p.applyRedactingRules(msg)
		assert.True(shouldProcess)
		assert.Equal("pan", msg.Origin.Source())
	})

	t.Run("no match falls back to config source", func(_ *testing.T) {
		source := sources.NewLogSource("", &config.LogsConfig{
			Source: "syslog_fallback",
			ProcessingRules: []*config.ProcessingRule{{
				Type: config.RemapSource,
				Name: "siem_remap",
				Matching: []*config.SourceMatchEntry{
					{Attribute: "siem.device_vendor", Value: "Security", NewSource: "arcsight"},
				},
			}},
		})
		sc := &message.BasicStructuredContent{Data: map[string]interface{}{
			"message": "test",
			"siem": map[string]interface{}{
				"device_vendor": "UnknownVendor",
			},
		}}
		msg := message.NewStructuredMessage(sc, message.NewOrigin(source), "", 0)

		p := &Processor{}
		shouldProcess := p.applyRedactingRules(msg)
		assert.True(shouldProcess)
		assert.Equal("syslog_fallback", msg.Origin.Source())
	})

	t.Run("unstructured message is a no-op", func(_ *testing.T) {
		source := sources.NewLogSource("", &config.LogsConfig{
			Source: "syslog_fallback",
			ProcessingRules: []*config.ProcessingRule{{
				Type: config.RemapSource,
				Name: "siem_remap",
				Matching: []*config.SourceMatchEntry{
					{Attribute: "siem.device_vendor", Value: "Security", NewSource: "arcsight"},
				},
			}},
		})
		msg := newMessage([]byte("plain text"), source, "")

		p := &Processor{}
		shouldProcess := p.applyRedactingRules(msg)
		assert.True(shouldProcess)
		assert.Equal("syslog_fallback", msg.Origin.Source())
	})

	t.Run("first match wins", func(_ *testing.T) {
		source := sources.NewLogSource("", &config.LogsConfig{
			Source: "syslog_fallback",
			ProcessingRules: []*config.ProcessingRule{{
				Type: config.RemapSource,
				Name: "siem_remap",
				Matching: []*config.SourceMatchEntry{
					{Attribute: "siem.format", Value: "CEF", NewSource: "cef_source"},
					{Attribute: "siem.device_vendor", Value: "Security", NewSource: "arcsight"},
				},
			}},
		})
		sc := &message.BasicStructuredContent{Data: map[string]interface{}{
			"message": "test",
			"siem": map[string]interface{}{
				"format":        "CEF",
				"device_vendor": "Security",
			},
		}}
		msg := message.NewStructuredMessage(sc, message.NewOrigin(source), "", 0)

		p := &Processor{}
		shouldProcess := p.applyRedactingRules(msg)
		assert.True(shouldProcess)
		assert.Equal("cef_source", msg.Origin.Source())
	})
}

func TestRemapSource_EscapedDotPath(t *testing.T) {
	assert := assert.New(t)

	t.Run("escaped dot matches dotted SD-ID key", func(_ *testing.T) {
		source := sources.NewLogSource("", &config.LogsConfig{
			Source: "fallback",
			ProcessingRules: []*config.ProcessingRule{{
				Type: config.RemapSource,
				Name: "sd_remap",
				Matching: []*config.SourceMatchEntry{
					{Attribute: `syslog.structured_data.my\.org@99999.status`, Value: "ok", NewSource: "matched_sd"},
				},
			}},
		})
		sc := &message.BasicStructuredContent{Data: map[string]interface{}{
			"message": "test",
			"syslog": map[string]interface{}{
				"structured_data": map[string]interface{}{
					"my.org@99999": map[string]interface{}{
						"status": "ok",
					},
				},
			},
		}}
		msg := message.NewStructuredMessage(sc, message.NewOrigin(source), "", 0)

		p := &Processor{}
		shouldProcess := p.applyRedactingRules(msg)
		assert.True(shouldProcess)
		assert.Equal("matched_sd", msg.Origin.Source())
	})

	t.Run("unescaped dot in dotted key does not match", func(_ *testing.T) {
		source := sources.NewLogSource("", &config.LogsConfig{
			Source: "fallback",
			ProcessingRules: []*config.ProcessingRule{{
				Type: config.RemapSource,
				Name: "sd_remap",
				Matching: []*config.SourceMatchEntry{
					{Attribute: "syslog.structured_data.my.org@99999.status", Value: "ok", NewSource: "should_not_match"},
				},
			}},
		})
		sc := &message.BasicStructuredContent{Data: map[string]interface{}{
			"message": "test",
			"syslog": map[string]interface{}{
				"structured_data": map[string]interface{}{
					"my.org@99999": map[string]interface{}{
						"status": "ok",
					},
				},
			},
		}}
		msg := message.NewStructuredMessage(sc, message.NewOrigin(source), "", 0)

		p := &Processor{}
		shouldProcess := p.applyRedactingRules(msg)
		assert.True(shouldProcess)
		assert.Equal("fallback", msg.Origin.Source())
	})
}
