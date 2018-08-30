// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package processor

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

func buildTestConfigLogSource(ruleType, replacePlaceholder, pattern string) config.LogSource {
	rule := config.ProcessingRule{
		Type:                    ruleType,
		Name:                    "test",
		ReplacePlaceholder:      replacePlaceholder,
		ReplacePlaceholderBytes: []byte(replacePlaceholder),
		Pattern:                 pattern,
		Reg:                     regexp.MustCompile(pattern),
	}
	return config.LogSource{Config: &config.LogsConfig{ProcessingRules: []config.ProcessingRule{rule}}}
}

func newMessage(content []byte, source *config.LogSource, status string) message.Message {
	origin := message.NewOrigin(source)
	msg := message.New(content, origin, status)
	return msg
}

func TestExclusion(t *testing.T) {

	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestConfigLogSource("exclude_at_match", "", "world")
	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("hello"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, _ = applyRedactingRules(newMessage([]byte("world"), &source, ""))
	assert.Equal(t, false, shouldProcess)

	shouldProcess, _ = applyRedactingRules(newMessage([]byte("a brand new world"), &source, ""))
	assert.Equal(t, false, shouldProcess)

	source = buildTestConfigLogSource("exclude_at_match", "", "$world")
	shouldProcess, _ = applyRedactingRules(newMessage([]byte("a brand new world"), &source, ""))
	assert.Equal(t, true, shouldProcess)
}

func TestInclusion(t *testing.T) {

	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestConfigLogSource("include_at_match", "", "world")
	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("hello"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("world"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("world"), redactedMessage)

	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("a brand new world"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("a brand new world"), redactedMessage)

	source = buildTestConfigLogSource("include_at_match", "", "^world")
	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("a brand new world"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)
}

func TestExclusionWithInclusion(t *testing.T) {

	var shouldProcess bool
	var redactedMessage []byte

	ePattern := "^bob"
	eRule := config.ProcessingRule{
		Type:    "exclude_at_match",
		Name:    "exclude_bob",
		Pattern: ePattern,
		Reg:     regexp.MustCompile(ePattern),
	}
	iPattern := ".*@datadoghq.com$"
	iRule := config.ProcessingRule{
		Type:    "include_at_match",
		Name:    "include_datadoghq",
		Pattern: iPattern,
		Reg:     regexp.MustCompile(iPattern),
	}
	source := config.LogSource{Config: &config.LogsConfig{ProcessingRules: []config.ProcessingRule{eRule, iRule}}}

	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("bob@datadoghq.com"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("bill@datadoghq.com"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("bill@datadoghq.com"), redactedMessage)

	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("bob@amail.com"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("bill@amail.com"), &source, ""))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)
}

func TestMask(t *testing.T) {

	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestConfigLogSource("mask_sequences", "[masked_world]", "world")
	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("hello"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("hello world!"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello [masked_world]!"), redactedMessage)

	source = buildTestConfigLogSource("mask_sequences", "[masked_user]", "User=\\w+@datadoghq.com")
	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("new test launched by User=beats@datadoghq.com on localhost"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("new test launched by [masked_user] on localhost"), redactedMessage)

	source = buildTestConfigLogSource("mask_sequences", "[masked_credit_card]", "(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\\d{3})\\d{11})")
	shouldProcess, redactedMessage = applyRedactingRules(newMessage([]byte("The credit card 4323124312341234 was used to buy some time"), &source, ""))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("The credit card [masked_credit_card] was used to buy some time"), redactedMessage)
}

func TestTruncate(t *testing.T) {

	source := config.NewLogSource("", &config.LogsConfig{})
	var redactedMessage []byte

	_, redactedMessage = applyRedactingRules(newMessage([]byte("hello"), source, ""))
	assert.Equal(t, []byte("hello"), redactedMessage)
}
