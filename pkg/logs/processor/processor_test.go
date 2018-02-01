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
	"github.com/DataDog/datadog-agent/pkg/logs/pb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func NewTestProcessor() Processor {
	return Processor{nil, nil, "", ""}
}

func buildTestConfigLogSource(ruleType, replacePlaceholder, pattern string) config.LogSource {
	rule := config.LogsProcessingRule{
		Type:                    ruleType,
		Name:                    "test",
		ReplacePlaceholder:      replacePlaceholder,
		ReplacePlaceholderBytes: []byte(replacePlaceholder),
		Pattern:                 pattern,
		Reg:                     regexp.MustCompile(pattern),
	}
	return config.LogSource{Config: &config.LogsConfig{ProcessingRules: []config.LogsProcessingRule{rule}, TagsPayload: []byte{'-'}}}
}

func newNetworkMessage(content []byte, source *config.LogSource) message.Message {
	msg := message.NewNetworkMessage(content)
	msgOrigin := message.NewOrigin()
	msgOrigin.LogSource = source
	msg.SetOrigin(msgOrigin)
	return msg
}

func TestProcessor(t *testing.T) {
	var p *Processor
	p = New(nil, nil, "hello", "world")
	assert.Equal(t, "hello", p.apikey)
	assert.Equal(t, "world", p.logset)
}

func TestExclusion(t *testing.T) {
	p := NewTestProcessor()
	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestConfigLogSource("exclude_at_match", "", "world")
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, _ = p.applyRedactingRules(newNetworkMessage([]byte("world"), &source))
	assert.Equal(t, false, shouldProcess)

	shouldProcess, _ = p.applyRedactingRules(newNetworkMessage([]byte("a brand new world"), &source))
	assert.Equal(t, false, shouldProcess)

	source = buildTestConfigLogSource("exclude_at_match", "", "$world")
	shouldProcess, _ = p.applyRedactingRules(newNetworkMessage([]byte("a brand new world"), &source))
	assert.Equal(t, true, shouldProcess)
}

func TestInclusion(t *testing.T) {
	p := NewTestProcessor()
	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestConfigLogSource("include_at_match", "", "world")
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello"), &source))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("world"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("world"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("a brand new world"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("a brand new world"), redactedMessage)

	source = buildTestConfigLogSource("include_at_match", "", "^world")
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("a brand new world"), &source))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)
}

func TestExclusionWithInclusion(t *testing.T) {
	p := NewTestProcessor()
	var shouldProcess bool
	var redactedMessage []byte

	ePattern := "^bob"
	eRule := config.LogsProcessingRule{
		Type:    "exclude_at_match",
		Name:    "exclude_bob",
		Pattern: ePattern,
		Reg:     regexp.MustCompile(ePattern),
	}
	iPattern := ".*@datadoghq.com$"
	iRule := config.LogsProcessingRule{
		Type:    "include_at_match",
		Name:    "include_datadoghq",
		Pattern: iPattern,
		Reg:     regexp.MustCompile(iPattern),
	}
	source := config.LogSource{Config: &config.LogsConfig{ProcessingRules: []config.LogsProcessingRule{eRule, iRule}, TagsPayload: []byte{'-'}}}

	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("bob@datadoghq.com"), &source))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("bill@datadoghq.com"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("bill@datadoghq.com"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("bob@amail.com"), &source))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("bill@amail.com"), &source))
	assert.Equal(t, false, shouldProcess)
	assert.Nil(t, redactedMessage)
}

func TestMask(t *testing.T) {
	p := NewTestProcessor()
	var shouldProcess bool
	var redactedMessage []byte

	source := buildTestConfigLogSource("mask_sequences", "[masked_world]", "world")
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello"), redactedMessage)

	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello world!"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("hello [masked_world]!"), redactedMessage)

	source = buildTestConfigLogSource("mask_sequences", "[masked_user]", "User=\\w+@datadoghq.com")
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("new test launched by User=beats@datadoghq.com on localhost"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("new test launched by [masked_user] on localhost"), redactedMessage)

	source = buildTestConfigLogSource("mask_sequences", "[masked_credit_card]", "(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\\d{3})\\d{11})")
	shouldProcess, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("The credit card 4323124312341234 was used to buy some time"), &source))
	assert.Equal(t, true, shouldProcess)
	assert.Equal(t, []byte("The credit card [masked_credit_card] was used to buy some time"), redactedMessage)
}

func TestTruncate(t *testing.T) {
	p := NewTestProcessor()
	source := config.NewLogSource("", &config.LogsConfig{})
	var redactedMessage []byte

	_, redactedMessage = p.applyRedactingRules(newNetworkMessage([]byte("hello"), source))
	assert.Equal(t, []byte("hello"), redactedMessage)
}

func TestToProtoPayload(t *testing.T) {

	apikey := "foo"
	logset := "bar"

	p := NewTestProcessor()
	p.apikey = apikey
	p.logset = logset

	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
	}

	source := config.NewLogSource("", logsConfig)

	rawMessage := "message"
	message := newNetworkMessage([]byte(rawMessage), source)
	message.SetSeverity(config.SevError)
	message.GetOrigin().Timestamp = "Timestamp"
	message.SetTagsPayload([]byte("tag:a,b:c,d"))

	redactedMessage := "redacted"

	payload := p.toProtoPayload(message, []byte(redactedMessage))

	assert.Equal(t, payload.GetApiKey(), apikey)
	assert.Equal(t, payload.GetLogset(), logset)

	assert.NotEmpty(t, payload.GetLog().Hostname)

	assert.Equal(t, payload.GetLog().Service, logsConfig.Service)
	assert.Equal(t, payload.GetLog().Source, logsConfig.Source)
	assert.Equal(t, payload.GetLog().Category, logsConfig.SourceCategory)
	assert.Equal(t, payload.GetLog().Tags, []string{"tag:a", "b:c", "d"})

	assert.Equal(t, payload.GetLog().Message, redactedMessage)
	assert.Equal(t, payload.GetLog().Status, config.StatusError)
	assert.Equal(t, payload.GetLog().Timestamp, message.GetOrigin().Timestamp)

}

func TestToProtoPayloadEmpty(t *testing.T) {

	p := NewTestProcessor()

	logsConfig := &config.LogsConfig{}

	source := config.NewLogSource("", logsConfig)

	rawMessage := ""
	message := newNetworkMessage([]byte(rawMessage), source)

	redactedMessage := ""

	payload := p.toProtoPayload(message, []byte(redactedMessage))

	assert.Empty(t, payload.GetApiKey())
	assert.Empty(t, payload.GetLogset())

	assert.NotEmpty(t, payload.GetLog().Hostname)

	assert.Empty(t, payload.GetLog().Service)
	assert.Empty(t, payload.GetLog().Source)
	assert.Empty(t, payload.GetLog().Category)
	assert.Empty(t, payload.GetLog().Tags)

	assert.Empty(t, payload.GetLog().Message)
	assert.Equal(t, payload.GetLog().Status, config.StatusInfo)
	assert.Empty(t, payload.GetLog().Timestamp, message.GetOrigin().Timestamp)

}

func TestToBytePayload(t *testing.T) {

	p := NewTestProcessor()

	payload := &pb.LogPayload{}
	raw, err := p.toBytePayload(payload)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(raw))
	assert.Equal(t, []byte{0x0}, []byte{raw[0]})

	payload1 := &pb.LogPayload{Log: &pb.Log{Message: "foo"}}
	bytes, err := payload1.Marshal()
	assert.Nil(t, err)
	raw, err = p.toBytePayload(payload1)
	assert.Nil(t, err)
	assert.True(t, len(raw) > 1)
	assert.Equal(t, proto.EncodeVarint(uint64(len(bytes))), []byte{raw[0]})
	payload2 := &pb.LogPayload{}
	err = payload2.Unmarshal(raw[1:])
	assert.Nil(t, err)
	assert.Equal(t, payload1, payload2)

}
