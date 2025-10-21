// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
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

func TestPIIRedactionEmail(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name   string
		input  []byte
		output []byte
	}{
		{
			name:   "single_email",
			input:  []byte("User john.doe@example.com logged in"),
			output: []byte("User [EMAIL_REDACTED] logged in"),
		},
		{
			name:   "multiple_emails",
			input:  []byte("From: alice@example.com To: bob@test.org"),
			output: []byte("From: [EMAIL_REDACTED] To: [EMAIL_REDACTED]"),
		},
		{
			name:   "no_email",
			input:  []byte("No email here, just text"),
			output: []byte("No email here, just text"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Use defaultPIIRedactionRules directly
			p := &Processor{processingRules: []*config.ProcessingRule{defaultPIIRedactionRules[0]}} // email rule
			source := sources.NewLogSource("", &config.LogsConfig{})
			msg := newMessage(tt.input, source, "")

			shouldProcess := p.applyRedactingRules(msg)
			assert.True(shouldProcess)
			assert.Equal(tt.output, msg.GetContent())
		})
	}
}

func TestPIIRedactionCreditCard(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name   string
		input  []byte
		output []byte
	}{
		{
			name:   "visa",
			input:  []byte("Payment with card 4111111111111111 approved"),
			output: []byte("Payment with card [CC_REDACTED] approved"),
		},
		{
			name:   "mastercard",
			input:  []byte("CC: 5500000000000004 was charged"),
			output: []byte("CC: [CC_REDACTED] was charged"),
		},
		{
			name:   "amex",
			input:  []byte("Amex 378282246310005 declined"),
			output: []byte("Amex [CC_REDACTED] declined"),
		},
		{
			name:   "no_cc",
			input:  []byte("No credit card numbers here"),
			output: []byte("No credit card numbers here"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Use credit card rule
			p := &Processor{processingRules: []*config.ProcessingRule{defaultPIIRedactionRules[1]}}
			source := sources.NewLogSource("", &config.LogsConfig{})
			msg := newMessage(tt.input, source, "")

			shouldProcess := p.applyRedactingRules(msg)
			assert.True(shouldProcess)
			assert.Equal(tt.output, msg.GetContent())
		})
	}
}

func TestPIIRedactionSSN(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name   string
		input  []byte
		output []byte
	}{
		{
			name:   "ssn_basic",
			input:  []byte("SSN: 123-45-6789 verified"),
			output: []byte("SSN: [SSN_REDACTED] verified"),
		},
		{
			name:   "no_ssn",
			input:  []byte("No social security numbers"),
			output: []byte("No social security numbers"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Use SSN rule
			p := &Processor{processingRules: []*config.ProcessingRule{defaultPIIRedactionRules[2]}}
			source := sources.NewLogSource("", &config.LogsConfig{})
			msg := newMessage(tt.input, source, "")

			shouldProcess := p.applyRedactingRules(msg)
			assert.True(shouldProcess)
			assert.Equal(tt.output, msg.GetContent())
		})
	}
}

func TestPIIRedactionPhone(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name   string
		input  []byte
		output []byte
	}{
		{
			name:   "phone_parentheses",
			input:  []byte("Call (555) 123-4567 for support"),
			output: []byte("Call [PHONE_REDACTED] for support"),
		},
		{
			name:   "phone_dashes",
			input:  []byte("Contact: 555-123-4567"),
			output: []byte("Contact: [PHONE_REDACTED]"),
		},
		{
			name:   "phone_dots",
			input:  []byte("Phone: 555.123.4567"),
			output: []byte("Phone: [PHONE_REDACTED]"),
		},
		{
			name:   "no_phone",
			input:  []byte("No phone numbers here"),
			output: []byte("No phone numbers here"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Use phone rule
			p := &Processor{processingRules: []*config.ProcessingRule{defaultPIIRedactionRules[3]}}
			source := sources.NewLogSource("", &config.LogsConfig{})
			msg := newMessage(tt.input, source, "")

			shouldProcess := p.applyRedactingRules(msg)
			assert.True(shouldProcess)
			assert.Equal(tt.output, msg.GetContent())
		})
	}
}

func TestPIIRedactionIPAddress(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		name   string
		input  []byte
		output []byte
	}{
		{
			name:   "ip_basic",
			input:  []byte("Request from 192.168.1.1 received"),
			output: []byte("Request from [IP_REDACTED] received"),
		},
		{
			name:   "ip_multiple",
			input:  []byte("Connection 10.0.0.1 to 172.16.0.1"),
			output: []byte("Connection [IP_REDACTED] to [IP_REDACTED]"),
		},
		{
			name:   "no_ip",
			input:  []byte("No IP addresses in this log"),
			output: []byte("No IP addresses in this log"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Use IP rule
			p := &Processor{processingRules: []*config.ProcessingRule{defaultPIIRedactionRules[4]}}
			source := sources.NewLogSource("", &config.LogsConfig{})
			msg := newMessage(tt.input, source, "")

			shouldProcess := p.applyRedactingRules(msg)
			assert.True(shouldProcess)
			assert.Equal(tt.output, msg.GetContent())
		})
	}
}

func TestPIIRedactionMultipleTypes(t *testing.T) {
	assert := assert.New(t)

	// Test all PII rules together
	p := &Processor{processingRules: defaultPIIRedactionRules}
	source := sources.NewLogSource("", &config.LogsConfig{})

	input := []byte("User alice@example.com from 10.0.0.1 used card 4111111111111111 SSN 123-45-6789 phone (555) 123-4567")
	expected := []byte("User [EMAIL_REDACTED] from [IP_REDACTED] used card [CC_REDACTED] SSN [SSN_REDACTED] phone [PHONE_REDACTED]")

	msg := newMessage(input, source, "")
	shouldProcess := p.applyRedactingRules(msg)

	assert.True(shouldProcess)
	assert.Equal(expected, msg.GetContent())
}

func TestPIIRedactionWithUserRules(t *testing.T) {
	assert := assert.New(t)

	// Combine default PII rules with a custom user rule
	userRule := newProcessingRule(config.MaskSequences, "[CUSTOM]", "secret")
	rules := append([]*config.ProcessingRule{userRule}, defaultPIIRedactionRules...)

	p := &Processor{processingRules: rules}
	source := sources.NewLogSource("", &config.LogsConfig{})

	input := []byte("User john@example.com has secret data from 10.0.0.1")
	expected := []byte("User [EMAIL_REDACTED] has [CUSTOM] data from [IP_REDACTED]")

	msg := newMessage(input, source, "")
	shouldProcess := p.applyRedactingRules(msg)

	assert.True(shouldProcess)
	assert.Equal(expected, msg.GetContent())
}

func TestIsPIIRule(t *testing.T) {
	assert := assert.New(t)

	// Test that PII rules are correctly identified
	for _, piiRule := range defaultPIIRedactionRules {
		assert.True(isPIIRule(piiRule), "Should identify %s as a PII rule", piiRule.Name)
	}

	// Test that user rules are not identified as PII rules
	userRule := newProcessingRule(config.MaskSequences, "[CUSTOM]", "test")
	assert.False(isPIIRule(userRule), "Should not identify user rule as a PII rule")
}

// Benchmark PII Redaction
func BenchmarkPIIRedaction(b *testing.B) {
	processor := &Processor{processingRules: defaultPIIRedactionRules}
	source := sources.NewLogSource("", &config.LogsConfig{})

	msg := newMessage(nil, source, "")

	b.Run("no PII match", func(b *testing.B) {
		// what we benchmark here is the best case scenario where no PII is present
		content := []byte("This is a regular log message without any PII data")
		b.ResetTimer()

		for range b.N {
			msg.SetContent(content)
			processor.applyRedactingRules(msg)
		}
	})

	b.Run("single email", func(b *testing.B) {
		// what we benchmark here is a common case with a single PII match
		content := []byte("User john.doe@example.com logged in successfully")
		b.ResetTimer()

		for range b.N {
			msg.SetContent(content)
			processor.applyRedactingRules(msg)
		}
	})

	b.Run("multiple PII types", func(b *testing.B) {
		// what we benchmark here is the worst case scenario with multiple PII types
		content := []byte("User alice@example.com from 10.0.0.1 used card 4111111111111111 phone (555) 123-4567")
		b.ResetTimer()

		for range b.N {
			msg.SetContent(content)
			processor.applyRedactingRules(msg)
		}
	})
}
