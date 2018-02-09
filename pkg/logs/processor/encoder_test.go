// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package processor

import (
	"testing"

	"strings"

	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pb"
	"github.com/stretchr/testify/assert"
)

func TestNewEncoder(t *testing.T) {
	assert.Equal(t, &protoEncoder, NewEncoder(true))
	assert.Equal(t, &rawEncoder, NewEncoder(false))
}

func TestRawEncoder(t *testing.T) {

	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           "foo:bar,baz",
	}

	source := config.NewLogSource("", logsConfig)

	rawMessage := "message"
	message := newNetworkMessage([]byte(rawMessage), source)
	message.SetSeverity(config.SevError)
	message.GetOrigin().LogSource = source
	message.GetOrigin().SetTags([]string{"a", "b:c"})

	redactedMessage := "redacted"

	raw, err := rawEncoder.encode(message, []byte(redactedMessage))
	assert.Nil(t, err)

	day := time.Now().UTC().Format("2006-01-02")

	msg := string(raw)
	parts := strings.Fields(msg)
	assert.Equal(t, string(config.SevError)+"0", parts[0])
	assert.Equal(t, day, parts[1][:len(day)])
	assert.NotEmpty(t, parts[2])
	assert.Equal(t, "Service", parts[3])
	assert.Equal(t, "-", parts[4])
	assert.Equal(t, "-", parts[5])
	extra := msg[strings.Index(msg, "[") : strings.LastIndex(msg, "]")+1]
	assert.Equal(t, "[dd ddsource=\"Source\"][dd ddsourcecategory=\"SourceCategory\"][dd ddtags=\"foo:bar,baz,a,b:c\"]", extra)
	assert.Equal(t, "redacted", msg[strings.LastIndex(msg, " ")+1:])

}

func TestRawEncoderDefaults(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := config.NewLogSource("", logsConfig)

	rawMessage := "a"
	message := newNetworkMessage([]byte(rawMessage), source)

	redactedMessage := "a"

	raw, err := rawEncoder.encode(message, []byte(redactedMessage))
	assert.Nil(t, err)

	day := time.Now().UTC().Format("2006-01-02")

	msg := string(raw)
	parts := strings.Fields(msg)
	assert.Equal(t, 7, len(parts))
	assert.Equal(t, string(config.SevInfo)+"0", parts[0])
	assert.Equal(t, day, parts[1][:len(day)])
	assert.NotEmpty(t, parts[2])
	assert.Equal(t, "-", parts[3])
	assert.Equal(t, "-", parts[4])
	assert.Equal(t, "-", parts[5])
	assert.Equal(t, "a", parts[6])

}

func TestRawEncoderEmpty(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := config.NewLogSource("", logsConfig)

	rawMessage := ""
	message := newNetworkMessage([]byte(rawMessage), source)

	redactedMessage := "foo"

	raw, err := rawEncoder.encode(message, []byte(redactedMessage))
	assert.Nil(t, err)
	assert.Equal(t, redactedMessage, string(raw))

}

func TestIsRFC5424Formatted(t *testing.T) {
	assert.False(t, rawEncoder.isRFC5424Formatted([]byte("<- test message ->")))
	assert.False(t, rawEncoder.isRFC5424Formatted([]byte("- test message ->")))
	assert.False(t, rawEncoder.isRFC5424Formatted([]byte("<46> the rest of the message")))
	assert.True(t, rawEncoder.isRFC5424Formatted([]byte("<46>0 the rest of the message")))
}

func TestProtoEncoder(t *testing.T) {

	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           "foo:bar,baz",
	}

	source := config.NewLogSource("", logsConfig)

	rawMessage := "message"
	message := newNetworkMessage([]byte(rawMessage), source)
	message.SetSeverity(config.SevError)
	message.GetOrigin().LogSource = source
	message.GetOrigin().SetTags([]string{"a", "b:c"})

	redactedMessage := "redacted"

	proto, err := protoEncoder.encode(message, []byte(redactedMessage))
	assert.Nil(t, err)

	log := &pb.Log{}
	err = log.Unmarshal(proto)
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Equal(t, logsConfig.Service, log.Service)
	assert.Equal(t, logsConfig.Source, log.Source)
	assert.Equal(t, []string{"a", "b:c", "source:" + logsConfig.Source, "sourcecategory:" + logsConfig.SourceCategory, "foo:bar", "baz"}, log.Tags)

	assert.Equal(t, redactedMessage, log.Message)
	assert.Equal(t, config.StatusError, log.Status)
	assert.NotEmpty(t, log.Timestamp)

}

func TestProtoEncoderEmpty(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := config.NewLogSource("", logsConfig)

	rawMessage := ""
	message := newNetworkMessage([]byte(rawMessage), source)

	redactedMessage := ""

	raw, err := protoEncoder.encode(message, []byte(redactedMessage))
	assert.Nil(t, err)

	log := &pb.Log{}
	err = log.Unmarshal(raw)
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Empty(t, log.Service)
	assert.Empty(t, log.Source)
	assert.Empty(t, log.Tags)

	assert.Empty(t, log.Message)
	assert.Equal(t, log.Status, config.StatusInfo)
	assert.NotEmpty(t, log.Timestamp)

}
