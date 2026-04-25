// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
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
	rule := &config.ProcessingRule{
		Type:               ruleType,
		Name:               ruleName,
		ReplacePlaceholder: replacePlaceholder,
		Pattern:            pattern,
	}
	rules := []*config.ProcessingRule{rule}
	if err := config.CompileProcessingRules(rules); err != nil {
		panic(err)
	}
	return rule
}

// newProcessingRuleBatch creates and compiles a batch of rules together.
func newProcessingRuleBatch(specs []struct{ ruleType, replacePlaceholder, pattern string }) []*config.ProcessingRule {
	rules := make([]*config.ProcessingRule, len(specs))
	for i, s := range specs {
		rules[i] = &config.ProcessingRule{
			Type:               s.ruleType,
			Name:               fmt.Sprintf("%s_%d", ruleName, i),
			ReplacePlaceholder: s.replacePlaceholder,
			Pattern:            s.pattern,
		}
	}
	if err := config.CompileProcessingRules(rules); err != nil {
		panic(err)
	}
	return rules
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

func BenchmarkMaskSequences(b *testing.B) {
	rule := newProcessingRule(config.MaskSequences, "api_key=****************************", "api_key=[a-f0-9]{28}")
	processor := &Processor{
		processingRules: []*config.ProcessingRule{rule},
	}

	benchSource := sources.NewLogSource("", &config.LogsConfig{})
	msg := newMessage(nil, benchSource, "")

	b.Run("always matching", func(b *testing.B) {
		content := []byte("[1234] log message, api_key=1234567890123456789012345678")
		b.ResetTimer()

		for range b.N {
			msg.SetContent(content)
			processor.applyRedactingRules(msg)
		}
	})

	b.Run("never matching", func(b *testing.B) {
		content := []byte("nothing to see here")
		b.ResetTimer()

		for range b.N {
			msg.SetContent(content)
			processor.applyRedactingRules(msg)
		}
	})
}

func BenchmarkProcessMessage(b *testing.B) {
	logLine := []byte(`2025-03-09T12:00:00.000Z INFO  [main] com.example.app - Request processed successfully for user_id=abc123 api_key=1234567890123456789012345678 in 42ms`)

	source := sources.NewLogSource("bench", &config.LogsConfig{})

	makeProcessor := func(rules []*config.ProcessingRule, enc Encoder) *Processor {
		outputChan := make(chan *message.Message, 1)
		monitor := metrics.NewNoopPipelineMonitor("bench")
		return &Processor{
			inputChan:                 make(chan *message.Message, 1),
			outputChan:                outputChan,
			processingRules:           rules,
			encoder:                   enc,
			done:                      make(chan struct{}),
			diagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
			pipelineMonitor:           monitor,
			utilization:               monitor.MakeUtilizationMonitor("processor", "bench"),
		}
	}

	noopEnc := &noopEncoder{}

	b.Run("no_rules", func(b *testing.B) {
		p := makeProcessor(nil, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("exclude_at_match/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "FATAL"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("exclude_at_match/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "INFO"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("exclude_at_match_alternation/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "(?:DEBUG|TRACE|VERBOSE)"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("exclude_at_match_alternation/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "(?:INFO|WARN|ERROR)"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("include_at_match/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.IncludeAtMatch, "", "INFO"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("include_at_match/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.IncludeAtMatch, "", "FATAL"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("mask_sequences/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("mask_sequences/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED]", "(?:password|secret)=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	// Content contains the literal prefix "api_key=" but the value after it
	// is not 28 hex chars, so the full regex misses. This exercises the path
	// where isMatchingLiteralPrefix passes but the engine must scan and reject.
	prefixMissLogLine := []byte(`2025-03-09T12:00:00.000Z INFO  [main] com.example.app - Request processed for api_key=NOTAHEXVALUE in 42ms`)

	b.Run("mask_sequences/prefix_found_regex_miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(prefixMissLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	creditCardPattern := `(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})`

	b.Run("mask_complex/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED_CC]", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	ccLogLine := []byte(`2025-03-09T12:00:00.000Z INFO  [main] com.example.app - Payment processed for card=4323124312341234 user_id=abc123 in 42ms`)

	b.Run("mask_complex/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED_CC]", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(ccLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("combined_rules", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "DEBUG"),
			newProcessingRule(config.IncludeAtMatch, "", "INFO|WARN|ERROR"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
			newProcessingRule(config.MaskSequences, "[MASKED_USER]", `(?:user_id|account_id)=\w+`),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("combined_rules/raw_encoder", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "DEBUG"),
			newProcessingRule(config.IncludeAtMatch, "", "INFO|WARN|ERROR"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
			newProcessingRule(config.MaskSequences, "[MASKED_USER]", `(?:user_id|account_id)=\w+`),
		}
		p := makeProcessor(rules, RawEncoder)
		p.hostname, _ = hostnameinterface.NewMock("benchhost")
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("combined_rules/json_encoder", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "DEBUG"),
			newProcessingRule(config.IncludeAtMatch, "", "INFO|WARN|ERROR"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
			newProcessingRule(config.MaskSequences, "[MASKED_USER]", `(?:user_id|account_id)=\w+`),
		}
		p := makeProcessor(rules, JSONEncoder)
		p.hostname, _ = hostnameinterface.NewMock("benchhost")
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("combined_rules/proto_encoder", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "DEBUG"),
			newProcessingRule(config.IncludeAtMatch, "", "INFO|WARN|ERROR"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
			newProcessingRule(config.MaskSequences, "[MASKED_USER]", `(?:user_id|account_id)=\w+`),
		}
		p := makeProcessor(rules, ProtoEncoder)
		p.hostname, _ = hostnameinterface.NewMock("benchhost")
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("8_exclude_rules_no_literal_prefix/miss", func(b *testing.B) {
		specs := make([]struct{ ruleType, replacePlaceholder, pattern string }, 8)
		for i := range 8 {
			specs[i] = struct{ ruleType, replacePlaceholder, pattern string }{config.ExcludeAtMatch, "", fmt.Sprintf("(?:NOMATCH_%d|NEVER_%d)", i, i)}
		}
		rules := newProcessingRuleBatch(specs)
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("8_exclude_rules_no_literal_prefix/hit_last", func(b *testing.B) {
		specs := make([]struct{ ruleType, replacePlaceholder, pattern string }, 8)
		for i := range 7 {
			specs[i] = struct{ ruleType, replacePlaceholder, pattern string }{config.ExcludeAtMatch, "", fmt.Sprintf("(?:NOMATCH_%d|NEVER_%d)", i, i)}
		}
		specs[7] = struct{ ruleType, replacePlaceholder, pattern string }{config.ExcludeAtMatch, "", "(?:INFO|WARN)"}
		rules := newProcessingRuleBatch(specs)
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("8_exclude_rules_literal/miss", func(b *testing.B) {
		specs := make([]struct{ ruleType, replacePlaceholder, pattern string }, 8)
		for i := range 8 {
			specs[i] = struct{ ruleType, replacePlaceholder, pattern string }{config.ExcludeAtMatch, "", fmt.Sprintf("NOMATCH_%d", i)}
		}
		rules := newProcessingRuleBatch(specs)
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("8_exclude_rules_literal/hit_last", func(b *testing.B) {
		specs := make([]struct{ ruleType, replacePlaceholder, pattern string }, 8)
		for i := range 7 {
			specs[i] = struct{ ruleType, replacePlaceholder, pattern string }{config.ExcludeAtMatch, "", fmt.Sprintf("NOMATCH_%d", i)}
		}
		specs[7] = struct{ ruleType, replacePlaceholder, pattern string }{config.ExcludeAtMatch, "", "INFO"}
		rules := newProcessingRuleBatch(specs)
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("8_mask_rules_no_literal_prefix/miss", func(b *testing.B) {
		var rules []*config.ProcessingRule
		for i := range 8 {
			rules = append(rules, newProcessingRule(config.MaskSequences, "[REDACTED]", fmt.Sprintf("(?:secret_%d|token_%d)=[a-f0-9]+", i, i)))
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("8_mask_rules_no_literal_prefix/all_hit", func(b *testing.B) {
		content := []byte("k0=abc123 k1=abc123 k2=abc123 k3=abc123 k4=abc123 k5=abc123 k6=abc123 k7=abc123")
		var rules []*config.ProcessingRule
		for i := range 8 {
			rules = append(rules, newProcessingRule(config.MaskSequences, "[REDACTED]", fmt.Sprintf("(?:k%d)=[a-z0-9]+", i)))
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(content), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	// Two-rule heavy-match workflows
	b.Run("2_excludes_alternation/both_miss", func(b *testing.B) {
		specs := []struct{ ruleType, replacePlaceholder, pattern string }{
			{config.ExcludeAtMatch, "", "(?:DEBUG|TRACE|VERBOSE)"},
			{config.ExcludeAtMatch, "", "(?:healthcheck|readiness|liveness)"},
		}
		rules := newProcessingRuleBatch(specs)
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("2_excludes_alternation/first_hits", func(b *testing.B) {
		specs := []struct{ ruleType, replacePlaceholder, pattern string }{
			{config.ExcludeAtMatch, "", "(?:INFO|WARN|ERROR)"},
			{config.ExcludeAtMatch, "", "(?:healthcheck|readiness|liveness)"},
		}
		rules := newProcessingRuleBatch(specs)
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("exclude_plus_mask/exclude_hits", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "(?:INFO|WARN|ERROR)"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("exclude_plus_mask/exclude_misses", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "(?:DEBUG|TRACE|VERBOSE)"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("include_plus_mask/include_hits", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.IncludeAtMatch, "", "(?:INFO|WARN|ERROR)"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("include_plus_mask/include_misses", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.IncludeAtMatch, "", "(?:DEBUG|TRACE|VERBOSE)"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("per_source_rules", func(b *testing.B) {
		sourceWithRules := sources.NewLogSource("bench", &config.LogsConfig{
			ProcessingRules: []*config.ProcessingRule{
				newProcessingRule(config.ExcludeAtMatch, "", "DEBUG"),
				newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
			},
		})
		globalRules := []*config.ProcessingRule{
			newProcessingRule(config.IncludeAtMatch, "", "INFO|WARN|ERROR"),
			newProcessingRule(config.MaskSequences, "[MASKED_USER]", "user_id=\\w+"),
		}
		p := makeProcessor(globalRules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), sourceWithRules, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	// Non-literal regex exclude patterns (fall through to regexp engine)
	b.Run("exclude_regex/wildcard/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "kube-probe/.*"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("exclude_regex/wildcard/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "com\\.example\\..*"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("exclude_regex/case_insensitive/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "(?i)debug"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("exclude_regex/case_insensitive/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "(?i)info"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("exclude_regex/complex_alternation/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "kube-probe/.*|/healthz?|GET /ready"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("exclude_regex/complex_alternation/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "kube-probe/.*|processed successfully|GET /ready"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
		}
	})

	// Large message benchmarks (~500KB)
	// Padding is placed before the matchable content so the regex/bytes.Contains
	// engine must scan the full buffer before finding (or not finding) a match.
	largeLogLine := func() []byte {
		var buf bytes.Buffer
		buf.WriteString("2025-03-09T12:00:00.000Z INFO  [main] com.example.app - Start of large message. ")
		padding := strings.Repeat("abcdefghij", 50000) // 500KB of filler
		buf.WriteString(padding)
		buf.WriteString(" api_key=1234567890123456789012345678 END_OF_LOG")
		return buf.Bytes()
	}()

	b.Run("large_500KB/no_rules", func(b *testing.B) {
		p := makeProcessor(nil, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/exclude_literal/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "FATAL"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/exclude_literal/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "END_OF_LOG"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("large_500KB/exclude_alternation/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "(?:FATAL|CRITICAL|PANIC)"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/exclude_alternation/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "(?:FATAL|END_OF_LOG|PANIC)"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("large_500KB/exclude_regex/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "kube-probe/.*"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/exclude_regex/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "com\\.example\\..*"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
		}
	})

	// Complex-pattern exclude/include on large content — the credit card
	// regex has no literal prefix and many alternations, so the NFA must
	// backtrack at every position in the 500KB buffer.
	b.Run("large_500KB/exclude_complex/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	largeCCExcludeLogLine := func() []byte {
		var buf bytes.Buffer
		buf.WriteString("2025-03-09T12:00:00.000Z INFO  [main] com.example.app - Start of large message. ")
		buf.WriteString(strings.Repeat("abcdefghij", 50000))
		buf.WriteString(" card=4323124312341234 END_OF_LOG")
		return buf.Bytes()
	}()

	b.Run("large_500KB/exclude_complex/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeCCExcludeLogLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("large_500KB/include_complex/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.IncludeAtMatch, "", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeCCExcludeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/include_complex/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.IncludeAtMatch, "", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
		}
	})

	// Also add complex-pattern exclude/include on short content for comparison
	b.Run("exclude_complex/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(logLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("exclude_complex/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(ccLogLine), source, "")
			p.processMessage(msg)
		}
	})

	b.Run("large_500KB/mask/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED]", "(?:password|secret)=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/mask/hit", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/combined_rules", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.ExcludeAtMatch, "", "DEBUG"),
			newProcessingRule(config.IncludeAtMatch, "", "INFO|WARN|ERROR"),
			newProcessingRule(config.MaskSequences, "[REDACTED]", "api_key=[a-f0-9]{28}"),
			newProcessingRule(config.MaskSequences, "[MASKED_USER]", `(?:user_id|account_id)=\w+`),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/mask_complex/miss", func(b *testing.B) {
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED_CC]", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})

	b.Run("large_500KB/mask_complex/hit", func(b *testing.B) {
		var buf bytes.Buffer
		buf.WriteString("2025-03-09T12:00:00.000Z INFO  [main] com.example.app - Start of large message. ")
		buf.WriteString(strings.Repeat("abcdefghij", 50000))
		buf.WriteString(" card=4323124312341234 END_OF_LOG")
		largeCCLogLine := buf.Bytes()
		rules := []*config.ProcessingRule{
			newProcessingRule(config.MaskSequences, "[REDACTED_CC]", creditCardPattern),
		}
		p := makeProcessor(rules, noopEnc)
		b.ResetTimer()
		for range b.N {
			msg := newMessage(slices.Clone(largeCCLogLine), source, "")
			p.processMessage(msg)
			<-p.outputChan
		}
	})
}

type noopEncoder struct{}

func (n *noopEncoder) Encode(msg *message.Message, _ string) error {
	msg.State = message.StateEncoded
	return nil
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
