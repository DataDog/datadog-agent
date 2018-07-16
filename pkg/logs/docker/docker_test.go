// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	// "bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const header = "100000002018-06-14T18:27:03.246999277Z"

func TestParseMessageShouldSucceedWithValidInput(t *testing.T) {
	validMessage := header + " " + "anything"
	dockerMsg, err := ParseMessage([]byte(validMessage))
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, message.StatusInfo, dockerMsg.Status)
	assert.Equal(t, []byte("anything"), dockerMsg.Content)
}

func TestParseMessageShouldFailWithInvalidInput(t *testing.T) {
	var msg []byte
	var err error

	// missing header separator
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0}...)
	_, err = ParseMessage(msg)
	assert.NotNil(t, err)

	// invalid header size
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0, 62, 49, 103}...)
	msg = append(msg, []byte("INFO_10:26:31_Loading_settings_from_file:/etc/cassandra/cassandra.yaml")...)
	_, err = ParseMessage(msg)
	assert.NotNil(t, err)
}

func TestParseMessageShouldRemovePartialHeaders(t *testing.T) {
	var msgToClean []byte
	var dockerMsg Message
	var expectedMsg []byte
	var err error

	// 16kb log
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + header)
	expectedMsg = []byte(buildMessage('a', dockerBufferSize))
	dockerMsg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, message.StatusInfo, dockerMsg.Status)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, dockerBufferSize, len(dockerMsg.Content))

	// over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', dockerBufferSize) + buildMessage('b', 50))
	dockerMsg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, message.StatusInfo, dockerMsg.Status)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, dockerBufferSize+50, len(dockerMsg.Content))

	// three times over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', 3*dockerBufferSize) + buildMessage('b', 50))
	dockerMsg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, message.StatusInfo, dockerMsg.Status)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, 3*dockerBufferSize+50, len(dockerMsg.Content))
}

func buildPartialMessage(r rune, count int) string {
	return header + " " + strings.Repeat(string(r), count)
}

func buildMessage(r rune, count int) string {
	return strings.Repeat(string(r), count)
}
