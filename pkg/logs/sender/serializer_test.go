// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestArraySerializer(t *testing.T) {
	var messages []*message.Message
	var payload []byte

	serializer := NewArraySerializer()

	messages = []*message.Message{message.NewMessage([]byte("a"), nil, "", 0)}
	payload = serializeToBytes(t, serializer, messages)
	serializer.Reset()
	assert.Equal(t, []byte("[a]"), payload)

	messages = []*message.Message{message.NewMessage([]byte("a"), nil, "", 0), message.NewMessage([]byte("b"), nil, "", 0)}
	payload = serializeToBytes(t, serializer, messages)
	serializer.Reset()
	assert.Equal(t, []byte("[a,b]"), payload)
}

func serializeToBytes(t *testing.T, s Serializer, messages []*message.Message) []byte {
	t.Helper()

	var payload bytes.Buffer
	for _, m := range messages {
		err := s.Serialize(m, &payload)
		assert.NoError(t, err)
	}
	err := s.Finish(&payload)
	assert.NoError(t, err)
	return payload.Bytes()
}
