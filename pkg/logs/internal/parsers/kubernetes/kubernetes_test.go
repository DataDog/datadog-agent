// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var containerdHeaderOut = "2018-09-20T11:54:11.753589172Z stdout F"
var partialContainerdHeaderOut = "2018-09-20T11:54:11.753589172Z stdout P"

func TestKubernetesGetStatus(t *testing.T) {
	assert.Equal(t, message.StatusInfo, getStatus([]byte("stdout")))
	assert.Equal(t, message.StatusError, getStatus([]byte("stderr")))
	assert.Equal(t, message.StatusInfo, getStatus([]byte("")))
}

func TestKubernetesParserShouldSucceedWithValidInput(t *testing.T) {
	validMessage := containerdHeaderOut + " " + "anything"
	logMessage := message.NewMessage([]byte(validMessage), nil, "", 0)
	msg, err := New().Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("anything"), msg.GetContent())
}
func TestKubernetesParserShouldSucceedWithPartialFlag(t *testing.T) {
	validMessage := partialContainerdHeaderOut + " " + "anything"
	logMessage := message.NewMessage([]byte(validMessage), nil, "", 0)
	msg, err := New().Parse(logMessage)
	assert.Nil(t, err)
	assert.True(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("anything"), msg.GetContent())
}

func TestKubernetesParserShouldHandleEmptyMessage(t *testing.T) {
	logMessage := message.NewMessage([]byte(containerdHeaderOut), nil, "", 0)
	msg, err := New().Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.GetContent()))
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", msg.ParsingExtra.Timestamp)
}

func TestKubernetesParserShouldFailWithInvalidInput(t *testing.T) {
	// Only timestamp
	var err error
	logMessage := message.NewMessage([]byte("2018-09-20T11:54:11.753589172Z foo"), nil, "", 0)
	msg, err := New().Parse(logMessage)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.NotNil(t, err)
	assert.Equal(t, logMessage.GetContent(), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "", msg.ParsingExtra.Timestamp)

	// Missing timestamp - should now fail with invalid timestamp error
	logMessage.SetContent([]byte("stdout F foo bar"))
	_, err = New().Parse(logMessage)
	assert.NotNil(t, err)
	assert.Equal(t, "invalid timestamp format", err.Error())
}

func TestKubernetesParserShouldRejectInvalidTimestamp(t *testing.T) {
	// When first component is not a valid timestamp, the parser must reject it
	// because container/tailer.go uses ParsingExtra.Timestamp as an offset for
	// resuming log tailing. Invalid timestamps would cause tailer failures.
	logMessage := message.NewMessage([]byte("stdout F foo bar"), nil, "", 0)
	msg, err := New().Parse(logMessage)

	// Verify parser rejects invalid timestamp to prevent downstream failures
	assert.NotNil(t, err)
	assert.Equal(t, "invalid timestamp format", err.Error())
	assert.Equal(t, logMessage.GetContent(), msg.GetContent())

	// Test another malformed timestamp case
	logMessage.SetContent([]byte("2018-invalid-timestamp stdout F message"))
	_, err = New().Parse(logMessage)
	assert.NotNil(t, err)
	assert.Equal(t, "invalid timestamp format", err.Error())
}
