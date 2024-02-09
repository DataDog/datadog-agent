// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerstream

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var dockerHeader = string([]byte{1, 0, 0, 0, 0, 0, 0, 0}) + "2018-06-14T18:27:03.246999277Z"
var container1Parser = New("container_1")

func TestGetDockerSeverity(t *testing.T) {
	assert.Equal(t, message.StatusInfo, getDockerSeverity([]byte{1}))
	assert.Equal(t, message.StatusError, getDockerSeverity([]byte{2}))
	assert.Equal(t, "", getDockerSeverity([]byte{3}))
}

func TestDockerStandaloneParserShouldSucceedWithValidInput(t *testing.T) {
	validMessage := dockerHeader + " " + "anything"
	parser := New("container_1")
	logMessage := message.NewMessage([]byte(validMessage), nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("anything"), msg.GetContent())
}

func TestDockerStandaloneParserShouldHandleEmptyMessage(t *testing.T) {
	logMessage := message.NewMessage([]byte(dockerHeader), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.GetContent()))
}

func TestDockerStandaloneParserShouldHandleNewlineOnlyMessage(t *testing.T) {
	emptyContent := [3]string{"\\n", "\\r", "\\r\\n"}

	for _, em := range emptyContent {
		logMessage := message.NewMessage([]byte("2018-06-14T18:27:03.246999277Z "+em), nil, "", 0)
		msg, err := container1Parser.Parse(logMessage)
		assert.Nil(t, err)
		assert.Equal(t, 0, len(msg.GetContent()))
	}
}

func TestDockerStandaloneParserShouldHandleTtyMessage(t *testing.T) {
	logMessage := message.NewMessage([]byte("2018-06-14T18:27:03.246999277Z foo"), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("foo"), msg.GetContent())
}

func TestDockerStandaloneParserShouldHandleEmptyTtyMessage(t *testing.T) {
	logMessage := message.NewMessage([]byte("2018-06-14T18:27:03.246999277Z"), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.GetContent()))
	logMessage.SetContent([]byte("2018-06-14T18:27:03.246999277Z "))
	msg, err = container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.GetContent()))
}

func TestDockerStandaloneParserShouldFailWithInvalidInput(t *testing.T) {
	var msg []byte
	var err error

	// missing dockerHeader separator
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0}...)
	logMessage := message.NewMessage(msg, nil, "", 0)
	_, err = container1Parser.Parse(logMessage)
	assert.Equal(t, errors.New("cannot parse docker message for container container_1: expected a 8 bytes header"), err)

}

func TestDockerStandaloneParserShouldRemovePartialHeaders(t *testing.T) {
	var msgToClean []byte
	var expectedMsg []byte

	// 16kb log
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + dockerHeader)
	expectedMsg = []byte(buildMessage('a', dockerBufferSize))
	logMessage := message.NewMessage(msgToClean, nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, expectedMsg, msg.GetContent())
	assert.Equal(t, dockerBufferSize, len(msg.GetContent()))

	// over 16kb
	logMessage.SetContent([]byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50)))
	expectedMsg = []byte(buildMessage('a', dockerBufferSize) + buildMessage('b', 50))
	msg, err = container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, expectedMsg, msg.GetContent())
	assert.Equal(t, dockerBufferSize+50, len(msg.GetContent()))

	// three times over 16kb
	logMessage.SetContent([]byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50)))
	expectedMsg = []byte(buildMessage('a', 3*dockerBufferSize) + buildMessage('b', 50))
	msg, err = container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, expectedMsg, msg.GetContent())
	assert.Equal(t, 3*dockerBufferSize+50, len(msg.GetContent()))
}

func buildPartialMessage(r rune, count int) string {
	return dockerHeader + " " + strings.Repeat(string(r), count)
}

func buildMessage(r rune, count int) string {
	return strings.Repeat(string(r), count)
}
