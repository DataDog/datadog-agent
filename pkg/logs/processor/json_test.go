// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type jsonTestPayload struct {
	Message   string `json:"message"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
	Hostname  string `json:"hostname"`
	Service   string `json:"service"`
	Source    string `json:"ddsource"`
	Tags      string `json:"ddtags"`
}

func newRenderedMessage(t *testing.T) *message.Message {
	t.Helper()
	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}
	source := sources.NewLogSource("", logsConfig)

	msg := newMessage([]byte("hello"), source, message.StatusError)
	msg.State = message.StateRendered
	msg.Origin.LogSource = source
	msg.Origin.SetTags([]string{"a", "b:c"})
	return msg
}

func TestJSONEncoderWithHostTagsNilProvider(t *testing.T) {
	msg := newRenderedMessage(t)
	enc := NewJSONEncoderWithHostTags(nil)
	assert.NoError(t, enc.Encode(msg, "unknown"))

	var payload jsonTestPayload
	assert.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
	assert.Equal(t, "a,b:c,sourcecategory:SourceCategory,foo:bar,baz", payload.Tags)
}

func TestJSONEncoderWithHostTagsNonEmpty(t *testing.T) {
	msg := newRenderedMessage(t)
	enc := NewJSONEncoderWithHostTags(func() []string {
		return []string{"host:x", "env:prod"}
	})
	assert.NoError(t, enc.Encode(msg, "unknown"))

	var payload jsonTestPayload
	assert.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
	assert.Equal(t, "a,b:c,sourcecategory:SourceCategory,foo:bar,baz,host:x,env:prod", payload.Tags)
}

func TestJSONEncoderWithHostTagsEmptySlice(t *testing.T) {
	msg := newRenderedMessage(t)
	enc := NewJSONEncoderWithHostTags(func() []string { return nil })
	assert.NoError(t, enc.Encode(msg, "unknown"))

	var payload jsonTestPayload
	assert.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
	assert.Equal(t, "a,b:c,sourcecategory:SourceCategory,foo:bar,baz", payload.Tags)
}

func TestJSONEncoderWithHostTagsAppendsToEmptyMessageTags(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := newMessage([]byte("hello"), source, "")
	msg.State = message.StateRendered

	enc := NewJSONEncoderWithHostTags(func() []string { return []string{"host:x"} })
	assert.NoError(t, enc.Encode(msg, "unknown"))

	var payload jsonTestPayload
	assert.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
	assert.Equal(t, "host:x", payload.Tags)
}
