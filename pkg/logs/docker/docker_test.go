// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMessageShouldRemovePartialHeaders(t *testing.T) {
	var msgToClean []byte
	var msg []byte
	var expectedMsg []byte
	var err error

	header := "100000002018-06-14T18:27:03.246999277Z "

	// 16kb log
	msgToClean = []byte(header + strings.Repeat("a", 16*1024))
	expectedMsg = []byte(strings.Repeat("a", 16*1024))
	_, _, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, expectedMsg, msg)

	// over 16kb
	msgToClean = []byte(header + strings.Repeat("a", 16*1024) + header + strings.Repeat("b", 50))
	expectedMsg = []byte(strings.Repeat("a", 16*1024) + strings.Repeat("b", 50))
	_, _, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, expectedMsg, msg)

	// three times over 16kb
	msgToClean = []byte(header + strings.Repeat("a", 16*1024) + header + strings.Repeat("a", 16*1024) + header + strings.Repeat("a", 16*1024) + strings.Repeat("b", 50))
	expectedMsg = []byte(strings.Repeat("a", 16*1024) + strings.Repeat("a", 16*1024) + strings.Repeat("a", 16*1024) + strings.Repeat("b", 50))
	_, _, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, expectedMsg, msg)
}

func TestParseMessageShouldSucceedWithValidInput(t *testing.T) {
	validMessage := "100000002018-06-14T18:27:03.246999277Z anything"
	_, _, msg, err := ParseMessage([]byte(validMessage))
	assert.Nil(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, msg, []byte("anything"))
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
