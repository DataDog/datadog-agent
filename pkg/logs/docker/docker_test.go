// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const header = "100000002018-06-14T18:27:03.246999277Z "

func TestParseMessageShouldSucceedWithValidInput(t *testing.T) {
	validMessage := header + "anything"
	ts, status, msg, err := ParseMessage([]byte(validMessage))
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, []byte("anything"), msg)
}

func TestParseMessageShouldFailWithInvalidInput(t *testing.T) {
	var msg []byte
	var err error

	// missing header separator
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0}...)
	_, _, _, err = ParseMessage(msg)
	assert.NotNil(t, err)

	// invalid header size
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0, 62, 49, 103}...)
	msg = append(msg, []byte("INFO_10:26:31_Loading_settings_from_file:/etc/cassandra/cassandra.yaml")...)
	_, _, _, err = ParseMessage(msg)
	assert.NotNil(t, err)
}

func TestParseMessageShouldRemovePartialHeaders(t *testing.T) {
	var msgToClean []byte
	var msg []byte
	var ts string
	var status string
	var expectedMsg []byte
	var err error

	// 16kb log
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize))
	expectedMsg = []byte(buildMessage('a', dockerBufferSize))
	ts, status, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, msg)

	// over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', dockerBufferSize) + buildMessage('b', 50))
	ts, status, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, msg)

	// three times over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', 3*dockerBufferSize) + buildMessage('b', 50))
	ts, status, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, msg)
}

func buildPartialMessage(c rune, count int) string {
	return header + strings.Repeat(string(c), count)
}

func buildMessage(c rune, count int) string {
	return strings.Repeat(string(c), count)
}
