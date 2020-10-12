// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

var containerdHeaderOut = "2018-09-20T11:54:11.753589172Z stdout F"
var partialContainerdHeaderOut = "2018-09-20T11:54:11.753589172Z stdout P"

func TestGetKubernetesSeverity(t *testing.T) {
	assert.Equal(t, message.StatusInfo, getStatus([]byte("stdout")))
	assert.Equal(t, message.StatusError, getStatus([]byte("stderr")))
	assert.Equal(t, message.StatusInfo, getStatus([]byte("")))
}

func TestParserShouldSucceedWithValidInput(t *testing.T) {
	validMessage := containerdHeaderOut + " " + "anything"
	content, status, _, partial, err := Parser.Parse([]byte(validMessage))
	assert.Nil(t, err)
	assert.False(t, partial)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, []byte("anything"), content)
}
func TestParserShouldSucceedWithPartialFlag(t *testing.T) {
	validMessage := partialContainerdHeaderOut + " " + "anything"
	content, status, _, partial, err := Parser.Parse([]byte(validMessage))
	assert.Nil(t, err)
	assert.True(t, partial)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, []byte("anything"), content)
}

func TestParserShouldHandleEmptyMessage(t *testing.T) {
	msg, status, timestamp, partial, err := Parser.Parse([]byte(containerdHeaderOut))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg))
	assert.False(t, partial)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, "2018-09-20T11:54:11.753589172Z", timestamp)
}

func TestParserShouldFailWithInvalidInput(t *testing.T) {
	// Only timestamp
	var err error
	log := []byte("2018-09-20T11:54:11.753589172Z foo")
	msg, status, timestamp, partial, err := Parser.Parse(log)
	assert.False(t, partial)
	assert.NotNil(t, err)
	assert.Equal(t, log, msg)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, "", timestamp)

	// Missing timestamp but with 3 spaces, the message is valid
	// FIXME: We might want to handle that
	log = []byte("stdout F foo bar")
	_, _, _, _, err = Parser.Parse(log)
	assert.Nil(t, err)
}
