// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var containerdHeaderOut = "2018-09-20T11:54:11.753589172Z stdout F"
var containerdHeaderErr = "2018-09-20T11:54:11.753589172Z stderr F"

func TestGetContainerdSeverity(t *testing.T) {
	assert.Equal(t, StatusInfo, getContainerdSeverity([]byte("stdout")))
	assert.Equal(t, StatusError, getContainerdSeverity([]byte("stderr")))
	assert.Equal(t, "", getContainerdSeverity([]byte("")))
}

func TestContainerdParserShouldSucceedWithValidInput(t *testing.T) {
	parser := NewContainerdFileParser()
	validMessage := containerdHeaderOut + " " + "anything"
	containerdMsg, err := parser.Parse([]byte(validMessage))
	assert.Nil(t, err)
	assert.Equal(t, StatusInfo, containerdMsg.Severity)
	assert.Equal(t, []byte("anything"), containerdMsg.Content)
}

func TestContainerdParserShouldHandleEmptyMessage(t *testing.T) {
	parser := NewContainerdFileParser()
	msg, err := parser.Parse([]byte(containerdHeaderOut))
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.Content))
}

func TestContainerdParserShouldFailWithInvalidInput(t *testing.T) {
	parser := NewContainerdFileParser()
	// Missing Partial Flag
	var err error
	msg := []byte("2018-09-20T11:54:11.753589172Z stdout foo bar")
	_, err = parser.Parse(msg)
	assert.NotNil(t, err)

	// Missing stdout
	msg = []byte("2018-09-20T11:54:11.753589172Z F foo bar")
	_, err = parser.Parse(msg)
	assert.NotNil(t, err)

	// Missing timestamp
	msg = []byte("stdout F foo bar")
	_, err = parser.Parse(msg)
	assert.NotNil(t, err)
}
