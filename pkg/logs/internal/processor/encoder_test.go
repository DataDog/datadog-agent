// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"testing"

	"strings"

	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/pb"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestRawEncoder(t *testing.T) {

	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := "message"
	msg := newMessage([]byte(rawMessage), source, message.StatusError)
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})

	redactedMessage := "redacted"

	raw, err := RawEncoder.Encode(msg, []byte(redactedMessage))
	assert.Nil(t, err)

	day := time.Now().UTC().Format("2006-01-02")

	content := string(raw)
	parts := strings.Fields(content)
	assert.Equal(t, string(message.SevError)+"0", parts[0])
	assert.Equal(t, day, parts[1][:len(day)])
	assert.NotEmpty(t, parts[2])
	assert.Equal(t, "Service", parts[3])
	assert.Equal(t, "-", parts[4])
	assert.Equal(t, "-", parts[5])
	extra := content[strings.Index(content, "[") : strings.LastIndex(content, "]")+1]
	assert.Equal(t, "[dd ddsource=\"Source\"][dd ddsourcecategory=\"SourceCategory\"][dd ddtags=\"foo:bar,baz,a,b:c\"]", extra)
	assert.Equal(t, "redacted", content[strings.LastIndex(content, " ")+1:])

}

func TestRawEncoderDefaults(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := "a"
	msg := newMessage([]byte(rawMessage), source, "")

	redactedMessage := "a"

	raw, err := RawEncoder.Encode(msg, []byte(redactedMessage))
	assert.Nil(t, err)

	day := time.Now().UTC().Format("2006-01-02")

	content := string(raw)
	parts := strings.Fields(content)
	assert.Equal(t, 8, len(parts))
	assert.Equal(t, string(message.SevInfo)+"0", parts[0])
	assert.Equal(t, day, parts[1][:len(day)])
	assert.NotEmpty(t, parts[2])
	assert.Equal(t, "-", parts[3])
	assert.Equal(t, "-", parts[4])
	assert.Equal(t, "-", parts[5])
	assert.Equal(t, "-", parts[6])
	assert.Equal(t, "a", parts[7])

}

func TestRawEncoderEmpty(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := ""
	msg := newMessage([]byte(rawMessage), source, "")

	redactedMessage := "foo"

	raw, err := RawEncoder.Encode(msg, []byte(redactedMessage))
	assert.Nil(t, err)
	assert.Equal(t, redactedMessage, string(raw))

}

func TestIsRFC5424Formatted(t *testing.T) {
	assert.False(t, isRFC5424Formatted([]byte("<- test message ->")))
	assert.False(t, isRFC5424Formatted([]byte("- test message ->")))
	assert.False(t, isRFC5424Formatted([]byte("<46> the rest of the message")))
	assert.True(t, isRFC5424Formatted([]byte("<46>0 the rest of the message")))
}

func TestProtoEncoder(t *testing.T) {

	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := "message"
	msg := newMessage([]byte(rawMessage), source, message.StatusError)
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})

	redactedMessage := "redacted"

	proto, err := ProtoEncoder.Encode(msg, []byte(redactedMessage))
	assert.Nil(t, err)

	log := &pb.Log{}
	err = log.Unmarshal(proto)
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Equal(t, logsConfig.Service, log.Service)
	assert.Equal(t, logsConfig.Source, log.Source)
	assert.Equal(t, []string{"a", "b:c", "sourcecategory:" + logsConfig.SourceCategory, "foo:bar", "baz"}, log.Tags)

	assert.Equal(t, redactedMessage, log.Message)
	assert.Equal(t, message.StatusError, log.Status)
	assert.NotEmpty(t, log.Timestamp)

}

func TestProtoEncoderEmpty(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := ""
	msg := newMessage([]byte(rawMessage), source, "")

	redactedMessage := ""

	raw, err := ProtoEncoder.Encode(msg, []byte(redactedMessage))
	assert.Nil(t, err)

	log := &pb.Log{}
	err = log.Unmarshal(raw)
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Empty(t, log.Service)
	assert.Empty(t, log.Source)
	assert.Empty(t, log.Tags)

	assert.Empty(t, log.Message)
	assert.Equal(t, log.Status, message.StatusInfo)
	assert.NotEmpty(t, log.Timestamp)

}

func TestProtoEncoderHandleInvalidUTF8(t *testing.T) {
	cfg := &config.LogsConfig{}
	src := sources.NewLogSource("", cfg)
	msg := newMessage([]byte(""), src, "")
	encoded, err := ProtoEncoder.Encode(msg, []byte("a\xfez"))
	assert.NotNil(t, encoded)
	assert.Nil(t, err)
}

func TestJsonEncoder(t *testing.T) {
	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := "message"
	msg := newMessage([]byte(rawMessage), source, message.StatusError)
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})

	redactedMessage := "redacted"

	jsonMessage, err := JSONEncoder.Encode(msg, []byte(redactedMessage))
	assert.Nil(t, err)

	log := &jsonPayload{}
	err = json.Unmarshal(jsonMessage, log)
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Equal(t, logsConfig.Service, log.Service)
	assert.Equal(t, logsConfig.Source, log.Source)
	assert.Equal(t, "a,b:c,sourcecategory:"+logsConfig.SourceCategory+",foo:bar,baz", log.Tags)

	assert.Equal(t, redactedMessage, log.Message)
	assert.Equal(t, message.StatusError, log.Status)
	assert.NotEmpty(t, log.Timestamp)
}

func TestEncoderToValidUTF8(t *testing.T) {
	assert.Equal(t, "a�z", toValidUtf8([]byte("a\xfez")))
	assert.Equal(t, "a��z", toValidUtf8([]byte("a\xc0\xafz")))
	assert.Equal(t, "a���z", toValidUtf8([]byte("a\xed\xa0\x80z")))
	assert.Equal(t, "a����z", toValidUtf8([]byte("a\xf0\x8f\xbf\xbfz")))
}
