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
	logMessage := message.Message{
		Content: []byte(validMessage),
	}
	msg, err := New().Parse(&logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("anything"), msg.Content)
}
func TestKubernetesParserShouldSucceedWithPartialFlag(t *testing.T) {
	validMessage := partialContainerdHeaderOut + " " + "anything"
	logMessage := message.Message{
		Content: []byte(validMessage),
	}
	msg, err := New().Parse(&logMessage)
	assert.Nil(t, err)
	assert.True(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("anything"), msg.Content)
}

func TestKubernetesParserShouldHandleEmptyMessage(t *testing.T) {
	logMessage := message.Message{
		Content: []byte(containerdHeaderOut),
	}
	msg, err := New().Parse(&logMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", msg.ParsingExtra.Timestamp)
}

func TestKubernetesParserShouldFailWithInvalidInput(t *testing.T) {
	// Only timestamp
	var err error
	logMessage := message.Message{
		Content: []byte("2018-09-20T11:54:11.753589172Z foo"),
	}
	msg, err := New().Parse(&logMessage)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.NotNil(t, err)
	assert.Equal(t, logMessage.Content, msg.Content)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "", msg.ParsingExtra.Timestamp)

	// Missing timestamp but with 3 spaces, the message is valid
	// FIXME: We might want to handle that
	logMessage.Content = []byte("stdout F foo bar")
	_, err = New().Parse(&logMessage)
	assert.Nil(t, err)
}
