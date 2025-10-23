// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
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

func TestGetHostnameLambda(t *testing.T) {
	p := &Processor{}
	m := message.NewMessage([]byte("hello"), nil, "", 0)
	m.ServerlessExtra = message.ServerlessExtra{
		Lambda: &message.Lambda{
			ARN: "testHostName",
		},
	}
	assert.Equal(t, "testHostName", p.GetHostname(m))
}

func TestGetHostname(t *testing.T) {
	_, hostnameComponent := hostnameinterface.NewMock(hostnameinterface.MockHostname("testHostnameFromEnvVar"))
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

func newTokenizer() *automultilinedetection.Tokenizer {
	return automultilinedetection.NewTokenizer(10000)
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

	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := newMessage(nil, source, "")

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

// PII Auto-Redaction Tests
// -------------------------

// TestAutoPIIRedaction tests automatic PII redaction via config
func TestAutoPIIRedaction(t *testing.T) {
	mockConfig := pkgconfigmock.New(t)
	mockConfig.Set("logs_config.auto_redact_config.enabled", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.email", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.credit_card", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ssn", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.phone", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ip", true, model.SourceAgentRuntime)

	p := &Processor{
		config:       mockConfig,
		piiTokenizer: nil,
	}
	p.piiTokenizer = newTokenizer()

	source := sources.NewLogSource("", &config.LogsConfig{})

	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "ssn_redaction",
			input:    []byte("User SSN is 123-45-6789"),
			expected: []byte("User SSN is [SSN_REDACTED]"),
		},
		{
			name:     "email_redaction",
			input:    []byte("Contact user@example.com for info"),
			expected: []byte("Contact [EMAIL_REDACTED] for info"),
		},
		{
			name:     "phone_redaction",
			input:    []byte("Call (555) 123-4567 now"),
			expected: []byte("Call [PHONE_REDACTED] now"),
		},
		{
			name:     "credit_card_redaction",
			input:    []byte("Card number 4532-0151-1283-0366 charged"),
			expected: []byte("Card number [CC_REDACTED] charged"),
		},
		{
			name:     "ip_redaction",
			input:    []byte("Request from 192.168.1.100"),
			expected: []byte("Request from [IP_REDACTED]"),
		},
		{
			name:     "multiple_pii",
			input:    []byte("User john@test.com SSN 123-45-6789 phone 555-123-4567"),
			expected: []byte("User [EMAIL_REDACTED] SSN [SSN_REDACTED] phone [PHONE_REDACTED]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := newMessage(tt.input, source, "")
			shouldProcess := p.applyRedactingRules(msg)
			assert.True(t, shouldProcess, "Message should be processed")
			assert.Equal(t, tt.expected, msg.GetContent(), "Content should be redacted")
		})
	}
}

// TestAutoPIIRedactionDisabled tests that PII is not redacted when auto_redact_config.enabled is false
func TestAutoPIIRedactionDisabled(t *testing.T) {
	mockConfig := pkgconfigmock.New(t)
	mockConfig.Set("logs_config.auto_redact_config.enabled", false, model.SourceAgentRuntime)

	p := &Processor{
		config:       mockConfig,
		piiTokenizer: newTokenizer(),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "disabled_ssn_not_redacted",
			input: []byte("User SSN is 123-45-6789"),
		},
		{
			name:  "disabled_email_not_redacted",
			input: []byte("Contact user@example.com for info"),
		},
		{
			name:  "disabled_phone_not_redacted",
			input: []byte("Call (555) 123-4567 now"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := newMessage(tt.input, source, "")
			originalContent := make([]byte, len(tt.input))
			copy(originalContent, tt.input)

			shouldProcess := p.applyRedactingRules(msg)
			assert.True(t, shouldProcess, "Message should be processed")
			assert.Equal(t, originalContent, msg.GetContent(), "Content should NOT be redacted when disabled")
		})
	}
}

// TestAutoPIIRedactionDefaultMode tests that regex mode is used when pii_redaction_mode is not set
func TestAutoPIIRedactionWithUserRules(t *testing.T) {
	mockConfig := pkgconfigmock.New(t)
	mockConfig.Set("logs_config.auto_redact_config.enabled", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.email", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.credit_card", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ssn", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.phone", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ip", true, model.SourceAgentRuntime)

	// User-defined processing rule
	userRule := newProcessingRule(config.MaskSequences, "[CUSTOM]", "secret")

	p := &Processor{
		config:          mockConfig,
		processingRules: []*config.ProcessingRule{userRule},
		piiTokenizer:    newTokenizer(),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "user_rule_and_auto_pii",
			input:    []byte("secret data from user@example.com"),
			expected: []byte("[CUSTOM] data from [EMAIL_REDACTED]"),
		},
		{
			name:     "user_rule_only",
			input:    []byte("This has secret info"),
			expected: []byte("This has [CUSTOM] info"),
		},
		{
			name:     "auto_pii_only",
			input:    []byte("SSN: 123-45-6789"),
			expected: []byte("SSN: [SSN_REDACTED]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := newMessage(tt.input, source, "")
			shouldProcess := p.applyRedactingRules(msg)
			assert.True(t, shouldProcess, "Message should be processed")
			assert.Equal(t, tt.expected, msg.GetContent(), "Both user rules and auto PII should work")
		})
	}
}

// TestAutoPIIRedactionWithExclusionRules tests that exclusion rules work before PII redaction
func TestAutoPIIRedactionWithExclusionRules(t *testing.T) {
	mockConfig := pkgconfigmock.New(t)
	mockConfig.Set("logs_config.auto_redact_config.enabled", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.email", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.credit_card", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ssn", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.phone", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ip", true, model.SourceAgentRuntime)

	// Exclusion rule to exclude messages containing "DEBUG"
	exclusionRule := newProcessingRule(config.ExcludeAtMatch, "", "DEBUG")

	p := &Processor{
		config:          mockConfig,
		processingRules: []*config.ProcessingRule{exclusionRule},
		piiTokenizer:    newTokenizer(),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	// Message should be excluded before PII redaction
	msg := newMessage([]byte("DEBUG: User SSN is 123-45-6789"), source, "")
	shouldProcess := p.applyRedactingRules(msg)
	assert.False(t, shouldProcess, "Message should be excluded, not redacted")

	// Message should be redacted
	msg2 := newMessage([]byte("INFO: User SSN is 123-45-6789"), source, "")
	shouldProcess2 := p.applyRedactingRules(msg2)
	assert.True(t, shouldProcess2, "Message should be processed and redacted")
	assert.Equal(t, []byte("INFO: User SSN is [SSN_REDACTED]"), msg2.GetContent())
}

// TestAutoPIIRedactionNilConfig tests that processor handles nil config gracefully
func TestAutoPIIRedactionNilConfig(t *testing.T) {
	p := &Processor{
		config: nil, // No config
	}

	source := sources.NewLogSource("", &config.LogsConfig{})
	input := []byte("User SSN is 123-45-6789")
	originalContent := make([]byte, len(input))
	copy(originalContent, input)

	msg := newMessage(input, source, "")
	shouldProcess := p.applyRedactingRules(msg)

	assert.True(t, shouldProcess, "Message should be processed")
	assert.Equal(t, originalContent, msg.GetContent(), "No redaction should occur with nil config")
}

// TestAutoPIIRedactionStructuredMessages tests PII redaction on structured messages
func TestAutoPIIRedactionStructuredMessages(t *testing.T) {
	mockConfig := pkgconfigmock.New(t)
	mockConfig.Set("logs_config.auto_redact_config.enabled", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.email", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.credit_card", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ssn", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.phone", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ip", true, model.SourceAgentRuntime)

	p := &Processor{
		config:       mockConfig,
		piiTokenizer: newTokenizer(),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "structured_ssn",
			input:    []byte("User with SSN 123-45-6789 logged in"),
			expected: []byte("User with SSN [SSN_REDACTED] logged in"),
		},
		{
			name:     "structured_email",
			input:    []byte("Email: john@example.com"),
			expected: []byte("Email: [EMAIL_REDACTED]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := newStructuredMessage(tt.input, source, "")
			shouldProcess := p.applyRedactingRules(msg)
			assert.True(t, shouldProcess, "Structured message should be processed")
			assert.Equal(t, tt.expected, msg.GetContent(), "Structured message should be redacted")
		})
	}
}

// TestAutoPIIRedactionMetrics tests that processing rules are recorded correctly
func TestAutoPIIRedactionMetrics(t *testing.T) {
	mockConfig := pkgconfigmock.New(t)
	mockConfig.Set("logs_config.auto_redact_config.enabled", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.email", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.credit_card", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ssn", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.phone", true, model.SourceAgentRuntime)
	mockConfig.Set("logs_config.auto_redact_config.pii.ip", true, model.SourceAgentRuntime)

	p := &Processor{
		config:       mockConfig,
		piiTokenizer: newTokenizer(),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})
	input := []byte("SSN: 123-45-6789")

	msg := newMessage(input, source, "")
	shouldProcess := p.applyRedactingRules(msg)

	require.True(t, shouldProcess)
	// Verify that at least one PII rule was recorded
	totalCount := msg.Origin.LogSource.ProcessingInfo.GetCount("mask_sequences:auto_redact_ssn")
	assert.Greater(t, totalCount, int64(0), "Should record SSN redaction in processing info")
}

// TestAutoPIIRedactionPerSourceOverride tests that per-source config can override global config
func TestAutoPIIRedactionPerSourceOverride(t *testing.T) {
	mockConfig := pkgconfigmock.New(t)
	// Global: disabled
	mockConfig.Set("logs_config.auto_redact_config.enabled", false, model.SourceAgentRuntime)

	p := &Processor{
		config:       mockConfig,
		piiTokenizer: newTokenizer(),
	}

	// Per-source: enabled with only SSN
	enabled := true
	ssnEnabled := true
	emailEnabled := false
	sourceConfig := &config.LogsConfig{
		AutoRedactConfig: &types.AutoRedactConfig{
			Enabled: &enabled,
			PII: &types.PIITypeSettings{
				SSN:   &ssnEnabled,
				Email: &emailEnabled,
			},
		},
	}
	source := sources.NewLogSource("", sourceConfig)

	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "ssn_redacted_via_source_override",
			input:    []byte("SSN: 123-45-6789"),
			expected: []byte("SSN: [SSN_REDACTED]"),
		},
		{
			name:     "email_not_redacted_per_source",
			input:    []byte("Email: user@example.com"),
			expected: []byte("Email: user@example.com"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := newMessage(tt.input, source, "")
			shouldProcess := p.applyRedactingRules(msg)
			assert.True(t, shouldProcess, "Message should be processed")
			assert.Equal(t, tt.expected, msg.GetContent())
		})
	}
}

// TestAutoPIIRedactionDefaultsAllTypesToTrue tests that when enabled=true with no PII type config,
// all PII types default to true
func TestAutoPIIRedactionDefaultsAllTypesToTrue(t *testing.T) {
	mockConfig := pkgconfigmock.New(t)
	// Only set enabled=true, do NOT set any individual PII type configs
	mockConfig.Set("logs_config.auto_redact_config.enabled", true, model.SourceAgentRuntime)

	p := &Processor{
		config:       mockConfig,
		piiTokenizer: newTokenizer(),
	}

	source := sources.NewLogSource("", &config.LogsConfig{})

	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "ssn_defaults_to_enabled",
			input:    []byte("User SSN is 123-45-6789"),
			expected: []byte("User SSN is [SSN_REDACTED]"),
		},
		{
			name:     "email_defaults_to_enabled",
			input:    []byte("Contact user@example.com"),
			expected: []byte("Contact [EMAIL_REDACTED]"),
		},
		{
			name:     "credit_card_defaults_to_enabled",
			input:    []byte("Card 4532-0151-1283-0366"),
			expected: []byte("Card [CC_REDACTED]"),
		},
		{
			name:     "phone_defaults_to_enabled",
			input:    []byte("Call (555) 123-4567"),
			expected: []byte("Call [PHONE_REDACTED]"),
		},
		{
			name:     "ip_defaults_to_enabled",
			input:    []byte("From 192.168.1.100"),
			expected: []byte("From [IP_REDACTED]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := newMessage(tt.input, source, "")
			shouldProcess := p.applyRedactingRules(msg)
			assert.True(t, shouldProcess, "Message should be processed")
			assert.Equal(t, tt.expected, msg.GetContent(), "PII type should default to enabled")
		})
	}
}

// TestAutoPIIRedactionIndividualTypes tests enabling individual PII types
func TestAutoPIIRedactionIndividualTypes(t *testing.T) {
	tests := []struct {
		name         string
		enabledTypes map[string]bool // which types to enable
		input        []byte
		expected     []byte
		shouldRedact bool
		description  string
	}{
		{
			name: "only_credit_card_enabled",
			enabledTypes: map[string]bool{
				"email": false, "credit_card": true, "ssn": false, "phone": false, "ip": false,
			},
			input:        []byte("Card 4532-0151-1283-0366, SSN 123-45-6789, email user@test.com"),
			expected:     []byte("Card [CC_REDACTED], SSN 123-45-6789, email user@test.com"),
			shouldRedact: true,
			description:  "Only credit card should be redacted",
		},
		{
			name: "only_ssn_enabled",
			enabledTypes: map[string]bool{
				"email": false, "credit_card": false, "ssn": true, "phone": false, "ip": false,
			},
			input:        []byte("SSN: 123-45-6789, Phone: (555) 123-4567"),
			expected:     []byte("SSN: [SSN_REDACTED], Phone: (555) 123-4567"),
			shouldRedact: true,
			description:  "Only SSN should be redacted",
		},
		{
			name: "only_email_enabled",
			enabledTypes: map[string]bool{
				"email": true, "credit_card": false, "ssn": false, "phone": false, "ip": false,
			},
			input:        []byte("Contact user@example.com or call (555) 123-4567"),
			expected:     []byte("Contact [EMAIL_REDACTED] or call (555) 123-4567"),
			shouldRedact: true,
			description:  "Only email should be redacted",
		},
		{
			name: "only_phone_enabled",
			enabledTypes: map[string]bool{
				"email": false, "credit_card": false, "ssn": false, "phone": true, "ip": false,
			},
			input:        []byte("Call (555) 123-4567 or email user@test.com"),
			expected:     []byte("Call [PHONE_REDACTED] or email user@test.com"),
			shouldRedact: true,
			description:  "Only phone should be redacted",
		},
		{
			name: "only_ip_enabled",
			enabledTypes: map[string]bool{
				"email": false, "credit_card": false, "ssn": false, "phone": false, "ip": true,
			},
			input:        []byte("Request from 192.168.1.100, user@test.com"),
			expected:     []byte("Request from [IP_REDACTED], user@test.com"),
			shouldRedact: true,
			description:  "Only IP should be redacted",
		},
		{
			name: "ssn_and_credit_card_enabled",
			enabledTypes: map[string]bool{
				"email": false, "credit_card": true, "ssn": true, "phone": false, "ip": false,
			},
			input:        []byte("SSN 123-45-6789, Card 4532-0151-1283-0366, Phone (555) 123-4567"),
			expected:     []byte("SSN [SSN_REDACTED], Card [CC_REDACTED], Phone (555) 123-4567"),
			shouldRedact: true,
			description:  "SSN and credit card should be redacted, phone not",
		},
		{
			name: "all_disabled",
			enabledTypes: map[string]bool{
				"email": false, "credit_card": false, "ssn": false, "phone": false, "ip": false,
			},
			input:        []byte("SSN 123-45-6789, user@test.com, (555) 123-4567"),
			expected:     []byte("SSN 123-45-6789, user@test.com, (555) 123-4567"),
			shouldRedact: false,
			description:  "Nothing should be redacted when all types disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := pkgconfigmock.New(t)
			mockConfig.Set("logs_config.auto_redact_config.enabled", true, model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.email", tt.enabledTypes["email"], model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.credit_card", tt.enabledTypes["credit_card"], model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.ssn", tt.enabledTypes["ssn"], model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.phone", tt.enabledTypes["phone"], model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.ip", tt.enabledTypes["ip"], model.SourceAgentRuntime)

			p := &Processor{
				config:       mockConfig,
				piiTokenizer: newTokenizer(),
			}

			source := sources.NewLogSource("", &config.LogsConfig{})
			msg := newMessage(tt.input, source, "")

			shouldProcess := p.applyRedactingRules(msg)
			assert.True(t, shouldProcess, "Message should be processed")
			assert.Equal(t, tt.expected, msg.GetContent(), tt.description)
		})
	}
}

// TestAutoPIIRedactionPerSourceIndividualTypes tests per-source individual type overrides
func TestAutoPIIRedactionPerSourceIndividualTypes(t *testing.T) {
	tests := []struct {
		name        string
		globalTypes map[string]bool
		sourceTypes map[string]*bool // nil means use global
		input       []byte
		expected    []byte
		description string
	}{
		{
			name: "global_all_enabled_source_disables_email",
			globalTypes: map[string]bool{
				"email": true, "credit_card": true, "ssn": true, "phone": true, "ip": true,
			},
			sourceTypes: map[string]*bool{
				"email": boolPtr(false), // Disable email for this source
			},
			input:       []byte("Email user@test.com, SSN 123-45-6789"),
			expected:    []byte("Email user@test.com, SSN [SSN_REDACTED]"),
			description: "Email not redacted (source override), SSN redacted (global)",
		},
		{
			name: "global_none_enabled_source_enables_ssn",
			globalTypes: map[string]bool{
				"email": false, "credit_card": false, "ssn": false, "phone": false, "ip": false,
			},
			sourceTypes: map[string]*bool{
				"ssn": boolPtr(true), // Enable SSN for this source
			},
			input:       []byte("Email user@test.com, SSN 123-45-6789"),
			expected:    []byte("Email user@test.com, SSN [SSN_REDACTED]"),
			description: "SSN redacted (source override), email not redacted (global)",
		},
		{
			name: "source_selectively_enables_types",
			globalTypes: map[string]bool{
				"email": false, "credit_card": false, "ssn": false, "phone": false, "ip": false,
			},
			sourceTypes: map[string]*bool{
				"email":       boolPtr(true),
				"credit_card": boolPtr(true),
			},
			input:       []byte("user@test.com, Card 4532-0151-1283-0366, SSN 123-45-6789"),
			expected:    []byte("[EMAIL_REDACTED], Card [CC_REDACTED], SSN 123-45-6789"),
			description: "Email and CC redacted (source), SSN not (global)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := pkgconfigmock.New(t)
			mockConfig.Set("logs_config.auto_redact_config.enabled", true, model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.email", tt.globalTypes["email"], model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.credit_card", tt.globalTypes["credit_card"], model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.ssn", tt.globalTypes["ssn"], model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.phone", tt.globalTypes["phone"], model.SourceAgentRuntime)
			mockConfig.Set("logs_config.auto_redact_config.pii.ip", tt.globalTypes["ip"], model.SourceAgentRuntime)

			p := &Processor{
				config:       mockConfig,
				piiTokenizer: newTokenizer(),
			}

			// Build source config with overrides
			sourceConfig := &config.LogsConfig{
				AutoRedactConfig: &types.AutoRedactConfig{
					PII: &types.PIITypeSettings{
						Email:      tt.sourceTypes["email"],
						CreditCard: tt.sourceTypes["credit_card"],
						SSN:        tt.sourceTypes["ssn"],
						Phone:      tt.sourceTypes["phone"],
						IP:         tt.sourceTypes["ip"],
					},
				},
			}
			source := sources.NewLogSource("", sourceConfig)

			msg := newMessage(tt.input, source, "")
			shouldProcess := p.applyRedactingRules(msg)
			assert.True(t, shouldProcess, "Message should be processed")
			assert.Equal(t, tt.expected, msg.GetContent(), tt.description)
		})
	}
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}
