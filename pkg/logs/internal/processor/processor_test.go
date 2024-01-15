// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type processorTestCase struct {
	source        sources.LogSource
	input         []byte
	output        []byte
	shouldProcess bool
}

// exclusions tests
// ----------------

var exclusionTests = []processorTestCase{
	{
		source:        newSource("exclude_at_match", "", "world"),
		input:         []byte("hello"),
		output:        []byte("hello"),
		shouldProcess: true,
	},
	{
		source:        newSource("exclude_at_match", "", "world"),
		input:         []byte("world"),
		output:        []byte{},
		shouldProcess: false,
	},
	{
		source:        newSource("exclude_at_match", "", "world"),
		input:         []byte("a brand new world"),
		output:        []byte{},
		shouldProcess: false,
	},
	{
		source:        newSource("exclude_at_match", "", "$world"),
		input:         []byte("a brand new world"),
		output:        []byte("a brand new world"),
		shouldProcess: true,
	},
}

func TestExclusion(t *testing.T) {
	p := &Processor{}
	assert := assert.New(t)

	// unstructured messages

	for _, test := range exclusionTests {
		msg := newMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
	}

	// structured messages

	for _, test := range exclusionTests {
		msg := newStructuredMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
	}
}

// inclusion tests
// ---------------

var inclusionTests = []processorTestCase{
	{
		source:        sources.LogSource{Config: &config.LogsConfig{}},
		input:         []byte("hello"),
		output:        []byte("hello"),
		shouldProcess: false,
	},
	{
		source:        sources.LogSource{Config: &config.LogsConfig{}},
		input:         []byte("world"),
		output:        []byte("world"),
		shouldProcess: true,
	},
	{
		source:        sources.LogSource{Config: &config.LogsConfig{}},
		input:         []byte("a brand new world"),
		output:        []byte("a brand new world"),
		shouldProcess: true,
	},
	{
		source:        newSource("include_at_match", "", "^world"),
		input:         []byte("a brand new world"),
		output:        []byte("a brand new world"),
		shouldProcess: false,
	},
}

func TestInclusion(t *testing.T) {
	p := &Processor{processingRules: []*config.ProcessingRule{newProcessingRule("include_at_match", "", "world")}}
	assert := assert.New(t)

	// unstructured messages

	for _, test := range inclusionTests {
		msg := newMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
	}

	// structured messages

	for _, test := range inclusionTests {
		msg := newStructuredMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
	}

}

// exclusion with inclusion tests
// ------------------------------

var exclusionRule *config.ProcessingRule = newProcessingRule("exclude_at_match", "", "^bob")
var inclusionRule *config.ProcessingRule = newProcessingRule("include_at_match", "", ".*@datadoghq.com$")

var exclusionInclusionTests = []processorTestCase{
	{
		source:        sources.LogSource{Config: &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{inclusionRule}}},
		input:         []byte("bob@datadoghq.com"),
		output:        []byte("bob@datadoghq.com"),
		shouldProcess: false,
	},
	{
		source:        sources.LogSource{Config: &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{inclusionRule}}},
		input:         []byte("bill@datadoghq.com"),
		output:        []byte("bill@datadoghq.com"),
		shouldProcess: true,
	},
	{
		source:        sources.LogSource{Config: &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{inclusionRule}}},
		input:         []byte("bob@amail.com"),
		output:        []byte("bob@amail.com"),
		shouldProcess: false,
	},
	{
		source:        sources.LogSource{Config: &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{inclusionRule}}},
		input:         []byte("bill@amail.com"),
		output:        []byte("bill@amail.com"),
		shouldProcess: false,
	},
}

func TestExclusionWithInclusion(t *testing.T) {
	p := &Processor{processingRules: []*config.ProcessingRule{exclusionRule}}
	assert := assert.New(t)

	// unstructured messages

	for _, test := range exclusionInclusionTests {
		msg := newMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
	}

	// structured messages

	for _, test := range exclusionInclusionTests {
		msg := newStructuredMessage(test.input, &test.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(test.shouldProcess, shouldProcess)
		if test.shouldProcess {
			assert.Equal(test.output, msg.GetContent())
		}
	}
}

// mask_sequences test cases
// -------------------------

var masksTests = []processorTestCase{
	{
		source:        newSource("mask_sequences", "[masked_world]", "world"),
		input:         []byte("hello"),
		output:        []byte("hello"),
		shouldProcess: true,
	},
	{
		source:        newSource("mask_sequences", "[masked_world]", "world"),
		input:         []byte("hello world!"),
		output:        []byte("hello [masked_world]!"),
		shouldProcess: true,
	},
	{
		source:        newSource("mask_sequences", "[masked_user]", "User=\\w+@datadoghq.com"),
		input:         []byte("new test launched by User=beats@datadoghq.com on localhost"),
		output:        []byte("new test launched by [masked_user] on localhost"),
		shouldProcess: true,
	},
	{
		source:        newSource("mask_sequences", "[masked_credit_card]", "(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\\d{3})\\d{11})"),
		input:         []byte("The credit card 4323124312341234 was used to buy some time"),
		output:        []byte("The credit card [masked_credit_card] was used to buy some time"),
		shouldProcess: true,
	},
	{
		source:        newSource("mask_sequences", "${1}[masked_value]", "([Dd]ata_?values=)\\S+"),
		input:         []byte("New data added to Datavalues=123456 on prod"),
		output:        []byte("New data added to Datavalues=[masked_value] on prod"),
		shouldProcess: true,
	},
	{
		source:        newSource("mask_sequences", "${1}[masked_value]", "([Dd]ata_?values=)\\S+"),
		input:         []byte("New data added to data_values=123456 on prod"),
		output:        []byte("New data added to data_values=[masked_value] on prod"),
		shouldProcess: true,
	},
	{
		source:        newSource("mask_sequences", "${1}[masked_value]", "([Dd]ata_?values=)\\S+"),
		input:         []byte("New data added to data_values= on prod"),
		output:        []byte("New data added to data_values= on prod"),
		shouldProcess: true,
	},
}

func TestMask(t *testing.T) {
	p := &Processor{}
	assert := assert.New(t)

	// unstructured messages

	for _, maskTest := range masksTests {
		msg := newMessage(maskTest.input, &maskTest.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(maskTest.shouldProcess, shouldProcess)
		if maskTest.shouldProcess {
			assert.Equal(maskTest.output, msg.GetContent())
		}
	}

	// structured messages

	for _, maskTest := range masksTests {
		msg := newStructuredMessage(maskTest.input, &maskTest.source, "")
		shouldProcess := p.applyRedactingRules(msg)
		assert.Equal(maskTest.shouldProcess, shouldProcess)
		if maskTest.shouldProcess {
			assert.Equal(maskTest.output, msg.GetContent())
		}
	}
}

func TestTruncate(t *testing.T) {
	p := &Processor{}
	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := newMessage([]byte("hello"), source, "")
	_ = p.applyRedactingRules(msg)
	assert.Equal(t, []byte("hello"), msg.GetContent())
}

// helpers
// -

func newProcessingRule(ruleType, replacePlaceholder, pattern string) *config.ProcessingRule {
	return &config.ProcessingRule{
		Type:               ruleType,
		Name:               "test",
		ReplacePlaceholder: replacePlaceholder,
		Placeholder:        []byte(replacePlaceholder),
		Pattern:            pattern,
		Regex:              regexp.MustCompile(pattern),
	}
}

func newSource(ruleType, replacePlaceholder, pattern string) sources.LogSource {
	return sources.LogSource{Config: &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{newProcessingRule(ruleType, replacePlaceholder, pattern)}}}
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
