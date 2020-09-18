// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build docker

package docker

import (
	"errors"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

var dockerHeader = string([]byte{1, 0, 0, 0, 0, 0, 0, 0}) + "2018-06-14T18:27:03.246999277Z"
var container1Parser = NewParser("container_1")

func TestGetDockerSeverity(t *testing.T) {
	assert.Equal(t, message.StatusInfo, getDockerSeverity([]byte{1}))
	assert.Equal(t, message.StatusError, getDockerSeverity([]byte{2}))
	assert.Equal(t, "", getDockerSeverity([]byte{3}))
}

func TestDockerStandaloneParserShouldSucceedWithValidInput(t *testing.T) {
	validMessage := dockerHeader + " " + "anything"
	parser := NewParser("container_1")
	content, status, timestamp, partial, err := parser.Parse([]byte(validMessage))
	assert.Nil(t, err)
	assert.False(t, partial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", timestamp)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, []byte("anything"), content)
}

func TestDockerStandaloneParserShouldHandleEmptyMessage(t *testing.T) {
	content, _, _, _, err := container1Parser.Parse([]byte(dockerHeader))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(content))
}

func TestDockerStandaloneParserShouldHandleNewlineOnlyMessage(t *testing.T) {
	emptyContent := [3]string{"\\n", "\\r", "\\r\\n"}

	for _, em := range emptyContent {
		msg, _, _, _, err := container1Parser.Parse([]byte("2018-06-14T18:27:03.246999277Z " + em))
		assert.Nil(t, err)
		assert.Equal(t, 0, len(msg))
	}
}

func TestDockerStandaloneParserShouldHandleTtyMessage(t *testing.T) {
	msg, status, timestamp, partial, err := container1Parser.Parse([]byte("2018-06-14T18:27:03.246999277Z foo"))
	assert.Nil(t, err)
	assert.False(t, partial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", timestamp)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, []byte("foo"), msg)
}

func TestDockerStandaloneParserShouldHandleEmptyTtyMessage(t *testing.T) {
	msg, _, _, _, err := container1Parser.Parse([]byte("2018-06-14T18:27:03.246999277Z"))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg))
	msg, _, _, _, err = container1Parser.Parse([]byte("2018-06-14T18:27:03.246999277Z "))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg))
}

func TestDockerStandaloneParserShouldFailWithInvalidInput(t *testing.T) {
	var msg []byte
	var err error

	// missing dockerHeader separator
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0}...)
	_, _, _, _, err = container1Parser.Parse(msg)
	assert.Equal(t, errors.New("cannot parse docker message for container container_1: expected a 8 bytes header"), err)

}

func TestDockerStandaloneParserShouldRemovePartialHeaders(t *testing.T) {
	var msgToClean []byte
	var expectedMsg []byte

	// 16kb log
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + dockerHeader)
	expectedMsg = []byte(buildMessage('a', dockerBufferSize))
	content, status, timestamp, partial, err := container1Parser.Parse(msgToClean)
	assert.Nil(t, err)
	assert.False(t, partial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", timestamp)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, content)
	assert.Equal(t, dockerBufferSize, len(content))

	// over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', dockerBufferSize) + buildMessage('b', 50))
	content, status, timestamp, partial, err = container1Parser.Parse(msgToClean)
	assert.Nil(t, err)
	assert.False(t, partial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", timestamp)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, content)
	assert.Equal(t, dockerBufferSize+50, len(content))

	// three times over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', 3*dockerBufferSize) + buildMessage('b', 50))
	content, status, timestamp, partial, err = container1Parser.Parse(msgToClean)
	assert.Nil(t, err)
	assert.False(t, partial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", timestamp)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, content)
	assert.Equal(t, 3*dockerBufferSize+50, len(content))
}

func buildPartialMessage(r rune, count int) string {
	return dockerHeader + " " + strings.Repeat(string(r), count)
}

func buildMessage(r rune, count int) string {
	return strings.Repeat(string(r), count)
}
