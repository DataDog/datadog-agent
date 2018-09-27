// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build docker

package docker

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

var dockerHeader = string([]byte{1, 0, 0, 0, 0, 0, 0, 0}) + "2018-06-14T18:27:03.246999277Z"

func TestGetDockerSeverity(t *testing.T) {
	assert.Equal(t, message.StatusInfo, getDockerSeverity([]byte{1}))
	assert.Equal(t, message.StatusError, getDockerSeverity([]byte{2}))
	assert.Equal(t, "", getDockerSeverity([]byte{3}))
}

func TestDockerStandaloneParserShouldSucceedWithValidInput(t *testing.T) {
	validMessage := dockerHeader + " " + "anything"
	parser := dockerParser
	dockerMsg, err := parser.Parse([]byte(validMessage))
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, message.StatusInfo, dockerMsg.Status)
	assert.Equal(t, []byte("anything"), dockerMsg.Content)
}

func TestDockerStandaloneParserShouldHandleEmptyMessage(t *testing.T) {
	parser := dockerParser
	msg, err := parser.Parse([]byte(dockerHeader))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
}

func TestDockerStandaloneParserShouldHandleTtyMessage(t *testing.T) {
	parser := dockerParser
	msg, err := parser.Parse([]byte("2018-06-14T18:27:03.246999277Z foo"))
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("foo"), msg.Content)
}

func TestDockerStandaloneParserShouldHandleEmptyTtyMessage(t *testing.T) {
	parser := dockerParser
	msg, err := parser.Parse([]byte("2018-06-14T18:27:03.246999277Z"))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
	msg, err = parser.Parse([]byte("2018-06-14T18:27:03.246999277Z "))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
}

func TestDockerStandaloneParserShouldFailWithInvalidInput(t *testing.T) {
	parser := dockerParser
	var msg []byte
	var err error

	// missing dockerHeader separator
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0}...)
	_, err = parser.Parse(msg)
	assert.NotNil(t, err)

}

func TestDockerStandaloneParserShouldRemovePartialHeaders(t *testing.T) {
	parser := dockerParser
	var msgToClean []byte
	var dockerMsg *message.Message
	var expectedMsg []byte
	var err error

	// 16kb log
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + dockerHeader)
	expectedMsg = []byte(buildMessage('a', dockerBufferSize))
	dockerMsg, err = parser.Parse(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, message.StatusInfo, dockerMsg.Status)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, dockerBufferSize, len(dockerMsg.Content))

	// over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', dockerBufferSize) + buildMessage('b', 50))
	dockerMsg, err = parser.Parse(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, message.StatusInfo, dockerMsg.Status)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, dockerBufferSize+50, len(dockerMsg.Content))

	// three times over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', 3*dockerBufferSize) + buildMessage('b', 50))
	dockerMsg, err = parser.Parse(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, message.StatusInfo, dockerMsg.Status)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, 3*dockerBufferSize+50, len(dockerMsg.Content))
}

func buildPartialMessage(r rune, count int) string {
	return dockerHeader + " " + strings.Repeat(string(r), count)
}

func buildMessage(r rune, count int) string {
	return strings.Repeat(string(r), count)
}
