// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package parser

import (

	// "bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var dockerHeader = string([]byte{1, 0, 0, 0, 0, 0, 0, 0}) + "2018-06-14T18:27:03.246999277Z"
var containerdHeaderOut = "2018-09-20T11:54:11.753589172Z stdout F"
var containerdHeaderErr = "2018-09-20T11:54:11.753589172Z stderr F"

func TestGetDockerSeverity(t *testing.T) {
	assert.Equal(t, StatusInfo, getDockerSeverity([]byte{1}))
	assert.Equal(t, StatusError, getDockerSeverity([]byte{2}))
	assert.Equal(t, "", getDockerSeverity([]byte{3}))
}

func TestDockerStandaloneParserShouldSucceedWithValidInput(t *testing.T) {
	validMessage := dockerHeader + " " + "anything"
	dockerMsg, err := DockerStandaloneParser([]byte(validMessage))
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, StatusInfo, dockerMsg.Severity)
	assert.Equal(t, []byte("anything"), dockerMsg.Content)
}

func TestDockerStandaloneParserShouldHandleEmptyMessage(t *testing.T) {
	msg, err := DockerStandaloneParser([]byte(dockerHeader))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
}

func TestDockerStandaloneParserShouldHandleTtyMessage(t *testing.T) {
	msg, err := DockerStandaloneParser([]byte("2018-06-14T18:27:03.246999277Z foo"))
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.Timestamp)
	assert.Equal(t, StatusInfo, msg.Severity)
	assert.Equal(t, []byte("foo"), msg.Content)
}

func TestDockerStandaloneParserShouldHandleEmptyTtyMessage(t *testing.T) {
	msg, err := DockerStandaloneParser([]byte("2018-06-14T18:27:03.246999277Z"))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
	msg, err = DockerStandaloneParser([]byte("2018-06-14T18:27:03.246999277Z "))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
}

func TestDockerStandaloneParserShouldFailWithInvalidInput(t *testing.T) {
	var msg []byte
	var err error

	// missing dockerHeader separator
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0}...)
	_, err = DockerStandaloneParser(msg)
	assert.NotNil(t, err)

}

func TestDockerStandaloneParserShouldRemovePartialHeaders(t *testing.T) {
	var msgToClean []byte
	var dockerMsg ParsedLine
	var expectedMsg []byte
	var err error

	// 16kb log
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + dockerHeader)
	expectedMsg = []byte(buildMessage('a', dockerBufferSize))
	dockerMsg, err = DockerStandaloneParser(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, StatusInfo, dockerMsg.Severity)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, dockerBufferSize, len(dockerMsg.Content))

	// over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', dockerBufferSize) + buildMessage('b', 50))
	dockerMsg, err = DockerStandaloneParser(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, StatusInfo, dockerMsg.Severity)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, dockerBufferSize+50, len(dockerMsg.Content))

	// three times over 16kb
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50))
	expectedMsg = []byte(buildMessage('a', 3*dockerBufferSize) + buildMessage('b', 50))
	dockerMsg, err = DockerStandaloneParser(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", dockerMsg.Timestamp)
	assert.Equal(t, StatusInfo, dockerMsg.Severity)
	assert.Equal(t, expectedMsg, dockerMsg.Content)
	assert.Equal(t, 3*dockerBufferSize+50, len(dockerMsg.Content))
}

func buildPartialMessage(r rune, count int) string {
	return dockerHeader + " " + strings.Repeat(string(r), count)
}

func buildMessage(r rune, count int) string {
	return strings.Repeat(string(r), count)
}

func TestGetContainerdSeverity(t *testing.T) {
	assert.Equal(t, StatusInfo, getContainerdSeverity([]byte("stdout")))
	assert.Equal(t, StatusError, getContainerdSeverity([]byte("stderr")))
	assert.Equal(t, "", getContainerdSeverity([]byte("")))
}

func TestContainerdParserShouldSucceedWithValidInput(t *testing.T) {
	validMessage := containerdHeaderOut + " " + "anything"
	containerdMsg, err := ContainerdParser([]byte(validMessage))
	assert.Nil(t, err)
	assert.Equal(t, StatusInfo, containerdMsg.Severity)
	assert.Equal(t, []byte("anything"), containerdMsg.Content)
}

func TestContainerdParserShouldHandleEmptyMessage(t *testing.T) {
	msg, err := ContainerdParser([]byte(containerdHeaderOut))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
}

func TestContainerdParserShouldFailWithInvalidInput(t *testing.T) {
	// Missing Partial Flag
	var err error
	msg := []byte("2018-09-20T11:54:11.753589172Z stdout foo bar")
	_, err = ContainerdParser(msg)
	assert.NotNil(t, err)

	// Missing stdout
	msg = []byte("2018-09-20T11:54:11.753589172Z F foo bar")
	_, err = ContainerdParser(msg)
	assert.NotNil(t, err)

	// Missing timestamp
	msg = []byte("stdout F foo bar")
	_, err = ContainerdParser(msg)
	assert.NotNil(t, err)
}
