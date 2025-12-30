// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Unmarshal decodes protobuf wire format data into the Log struct.
// This is only used for testing and should not be in production code.
func (l *Log) Unmarshal(data []byte) error {
	for len(data) > 0 {
		fieldNum, wireType, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("invalid protobuf tag: %d", n)
		}
		data = data[n:]

		switch fieldNum {
		case 1: // Message
			if wireType != protowire.BytesType {
				return fmt.Errorf("invalid wire type for Message field")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("invalid string for Message: %d", n)
			}
			l.Message = v
			data = data[n:]

		case 2: // Status
			if wireType != protowire.BytesType {
				return fmt.Errorf("invalid wire type for Status field")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("invalid string for Status: %d", n)
			}
			l.Status = v
			data = data[n:]

		case 3: // Timestamp
			if wireType != protowire.VarintType {
				return fmt.Errorf("invalid wire type for Timestamp field")
			}
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("invalid varint for Timestamp: %d", n)
			}
			l.Timestamp = int64(v)
			data = data[n:]

		case 4: // Hostname
			if wireType != protowire.BytesType {
				return fmt.Errorf("invalid wire type for Hostname field")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("invalid string for Hostname: %d", n)
			}
			l.Hostname = v
			data = data[n:]

		case 5: // Service
			if wireType != protowire.BytesType {
				return fmt.Errorf("invalid wire type for Service field")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("invalid string for Service: %d", n)
			}
			l.Service = v
			data = data[n:]

		case 6: // Source
			if wireType != protowire.BytesType {
				return fmt.Errorf("invalid wire type for Source field")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("invalid string for Source: %d", n)
			}
			l.Source = v
			data = data[n:]

		case 7: // Tags (repeated)
			if wireType != protowire.BytesType {
				return fmt.Errorf("invalid wire type for Tags field")
			}
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return fmt.Errorf("invalid string for Tags: %d", n)
			}
			l.Tags = append(l.Tags, v)
			data = data[n:]

		default:
			// Skip unknown fields
			n := protowire.ConsumeFieldValue(fieldNum, wireType, data)
			if n < 0 {
				return fmt.Errorf("invalid field value: %d", n)
			}
			data = data[n:]
		}
	}
	return nil
}

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
	msg.State = message.StateRendered // we can only encode rendered message
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})
	msg.SetContent([]byte("redacted"))

	err := RawEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	day := time.Now().UTC().Format("2006-01-02")

	content := string(msg.GetContent())
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
	msg.State = message.StateRendered
	err := RawEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	day := time.Now().UTC().Format("2006-01-02")

	content := string(msg.GetContent())
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
	msg.State = message.StateRendered // we can only encode rendered message
	err := RawEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)
	assert.Equal(t, rawMessage, string(msg.GetContent()))

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
	msg.State = message.StateRendered // we can only encode rendered message
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})

	err := ProtoEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	log := &Log{}
	err = log.Unmarshal(msg.GetContent())
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Equal(t, logsConfig.Service, log.Service)
	assert.Equal(t, logsConfig.Source, log.Source)
	assert.Equal(t, []string{"a", "b:c", "sourcecategory:" + logsConfig.SourceCategory, "foo:bar", "baz"}, log.Tags)
	assert.Equal(t, message.StatusError, log.Status)
	assert.NotEmpty(t, log.Timestamp)

	data, err := log.Marshal()
	assert.NoError(t, err)
	assert.Equal(t, msg.GetContent(), data)

}

func TestProtoEncoderEmpty(t *testing.T) {

	logsConfig := &config.LogsConfig{}

	source := sources.NewLogSource("", logsConfig)

	rawMessage := ""
	msg := newMessage([]byte(rawMessage), source, "")
	msg.State = message.StateRendered // we can only encode rendered message

	err := ProtoEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	log := &Log{}
	err = log.Unmarshal(msg.GetContent())
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
	msg := newMessage([]byte("a\xfez"), src, "")
	msg.State = message.StateRendered
	err := ProtoEncoder.Encode(msg, "unknown")
	assert.NotNil(t, msg.GetContent())
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

	content := []byte("message")
	msg := newMessage(content, source, message.StatusError)
	msg.State = message.StateRendered // we can only encode rendered message
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})
	assert.Equal(t, msg.GetContent(), content) // before encoding, content should be the raw message

	err := JSONEncoder.Encode(msg, "unknown")
	assert.Nil(t, err)

	log := &jsonPayload{}
	err = json.Unmarshal(msg.GetContent(), log)
	assert.Nil(t, err)

	assert.NotEmpty(t, log.Hostname)

	assert.Equal(t, logsConfig.Service, log.Service)
	assert.Equal(t, logsConfig.Source, log.Source)
	assert.Equal(t, "a,b:c,sourcecategory:"+logsConfig.SourceCategory+",foo:bar,baz", log.Tags)

	json, _ := json.Marshal(log)
	assert.Equal(t, msg.GetContent(), json)

	assert.Equal(t, message.StatusError, log.Status)
	assert.NotEmpty(t, log.Timestamp)
}

func TestEncoderToValidUTF8(t *testing.T) {
	// valid utf-8
	assert.Equal(t, "", toValidUtf8(nil))
	assert.Equal(t, "", toValidUtf8([]byte("")))
	assert.Equal(t, "a", toValidUtf8([]byte("a")))
	assert.Equal(t, "abc", toValidUtf8([]byte("abc")))
	assert.Equal(t, "Hello, 世界", toValidUtf8([]byte("Hello, 世界")))

	// invalid utf-8
	assert.Equal(t, "a�z", toValidUtf8([]byte("a\xfez")))
	assert.Equal(t, "a��z", toValidUtf8([]byte("a\xc0\xafz")))
	assert.Equal(t, "a���z", toValidUtf8([]byte("a\xed\xa0\x80z")))
	assert.Equal(t, "a����z", toValidUtf8([]byte("a\xf0\x8f\xbf\xbfz")))
	assert.Equal(t, "世界����z 世界", toValidUtf8([]byte("世界\xf0\x8f\xbf\xbfz 世界")))
}
