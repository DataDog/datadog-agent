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
	msg, err := New().Parse([]byte(validMessage))
	assert.Nil(t, err)
	assert.False(t, msg.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("anything"), msg.Content)
}
func TestKubernetesParserShouldSucceedWithPartialFlag(t *testing.T) {
	validMessage := partialContainerdHeaderOut + " " + "anything"
	msg, err := New().Parse([]byte(validMessage))
	assert.Nil(t, err)
	assert.True(t, msg.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("anything"), msg.Content)
}

func TestKubernetesParserShouldHandleEmptyMessage(t *testing.T) {
	msg, err := New().Parse([]byte(containerdHeaderOut))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
	assert.False(t, msg.IsPartial)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", msg.Timestamp)
}

func TestKubernetesParserShouldFailWithInvalidInput(t *testing.T) {
	// Only timestamp
	var err error
	log := []byte("2018-09-20T11:54:11.753589172Z foo")
	msg, err := New().Parse(log)
	assert.False(t, msg.IsPartial)
	assert.NotNil(t, err)
	assert.Equal(t, log, msg.Content)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "", msg.Timestamp)

	// Missing timestamp but with 3 spaces, the message is valid
	// FIXME: We might want to handle that
	log = []byte("stdout F foo bar")
	msg, err = New().Parse(log)
	assert.Nil(t, err)
}
