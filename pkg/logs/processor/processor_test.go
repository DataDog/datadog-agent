// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package processor

import (
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

func NewTestProcessor() Processor {
	return Processor{nil, nil, "", "", nil}
}

func buildTestProcessingRule(ruleType, replacePlaceholder, pattern string, p *Processor) config.IntegrationConfigLogSource {
	rule := config.LogsProcessingRule{
		Type:                    ruleType,
		Name:                    "test",
		ReplacePlaceholder:      replacePlaceholder,
		ReplacePlaceholderBytes: []byte(replacePlaceholder),
		Pattern:                 pattern,
		Reg:                     regexp.MustCompile(pattern),
	}
	return config.IntegrationConfigLogSource{ProcessingRules: []config.LogsProcessingRule{rule}, TagsPayload: []byte{'-'}}
}

func newNetworkMessage(content []byte, source *config.IntegrationConfigLogSource) message.Message {
	msg := message.NewNetworkMessage(content)
	msgOrigin := message.NewOrigin()
	msgOrigin.LogSource = source
	msg.SetOrigin(msgOrigin)
	return msg
}

func TestProcessor(t *testing.T) {
	var p *Processor
	p = New(nil, nil, "hello", "world")
	assert.Equal(t, "hello/world", string(p.apikeyString))
	p = New(nil, nil, "helloworld", "")
	assert.Equal(t, "helloworld", string(p.apikeyString))
}

func TestExclusion(t *testing.T) {
	p := NewTestProcessor()
	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestProcessingRule("exclude_at_match", "", "world", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, _ = p.applyRedactingRules(newNetworkMessage([]byte("world"), &source))
	assert.Equal(t, false, shouldProcess)

	shouldProcess, _ = p.applyRedactingRules(newNetworkMessage([]byte("a brand new world"), &source))
	assert.Equal(t, false, shouldProcess)

	source = buildTestProcessingRule("exclude_at_match", "", "$world", &p)
	shouldProcess, _ = p.applyRedactingRules(newNetworkMessage([]byte("a brand new world"), &source))
	assert.Equal(t, true, shouldProcess)
}

func TestMask(t *testing.T) {
	p := NewTestProcessor()
	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestProcessingRule("mask_sequences", "[masked_world]", "world", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello world!"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello [masked_world]!"), redactedMessage)

	source = buildTestProcessingRule("mask_sequences", "[masked_user]", "User=\\w+@datadoghq.com", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("new test launched by User=beats@datadoghq.com on localhost"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("new test launched by [masked_user] on localhost"), redactedMessage)

	source = buildTestProcessingRule("mask_sequences", "[masked_credit_card]", "(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\\d{3})\\d{11})", &p)
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("The credit card 4323124312341234 was used to buy some time"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("The credit card [masked_credit_card] was used to buy some time"), redactedMessage)
}

func TestTruncate(t *testing.T) {
	p := NewTestProcessor()
	source := config.IntegrationConfigLogSource{}
	var redactedMessage []byte

	_, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello"), &source))
	assert.Equal(t, []byte("hello"), redactedMessage)
}

func TestComputeExtraContent(t *testing.T) {
	p := NewTestProcessor()
	var extraContent []byte
	var extraContentParts []string
	source := &config.IntegrationConfigLogSource{TagsPayload: []byte{'-'}}

	// message with Content only, check default values

	extraContent = p.computeExtraContent(newNetworkMessage([]byte("message"), source))
	extraContentParts = strings.Split(string(extraContent), " ")
	assert.Equal(t, 8, len(extraContentParts))

	assert.Equal(t, "<46>0", extraContentParts[0])
	format := "2006-01-02T15:04:05"
	timestamp, err := time.Parse(format, extraContentParts[1][:len(format)])
	assert.Nil(t, err)
	assert.True(t, math.Abs(time.Now().UTC().Sub(timestamp).Minutes()) < 1)

	extraContent = p.computeExtraContent(newNetworkMessage([]byte("<message"), source))
	assert.Nil(t, extraContent)

	// message with additional information
	msg := newNetworkMessage([]byte("message"), source)
	msg.GetOrigin().Timestamp = "ts"
	msg.SetSeverity([]byte("sev"))
	msg.SetTagsPayload([]byte("tags"))

	extraContent = p.computeExtraContent(msg)
	extraContentParts = strings.Split(string(extraContent), " ")
	assert.Equal(t, "sev0", extraContentParts[0])
	assert.Equal(t, "ts", extraContentParts[1])
	assert.Equal(t, "tags", extraContentParts[6])
}

func TestComputeApiKeyString(t *testing.T) {
	p := New(nil, nil, "hello", "world")

	source := &config.IntegrationConfigLogSource{}
	extraContent := p.computeAPIKeyString(newNetworkMessage(nil, source))
	assert.Equal(t, "hello/world", string(extraContent))

	source = &config.IntegrationConfigLogSource{Logset: "hi"}
	extraContent = p.computeAPIKeyString(newNetworkMessage(nil, source))
	assert.Equal(t, "hello/hi", string(extraContent))
}
