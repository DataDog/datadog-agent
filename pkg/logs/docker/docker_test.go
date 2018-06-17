// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const header = "100000002018-06-14T18:27:03.246999277Z "

func TestParseMessageShouldRemovePartialHeaders(t *testing.T) {
	var msgToClean []byte
	var msg []byte
	var ts string
	var status string
	var expectedMsg []byte
	var err error

	// 16kb log
	msgToClean = []byte(header + strings.Repeat("a", 16*1024))
	expectedMsg = []byte(strings.Repeat("a", 16*1024))
	ts, status, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, msg)

	// over 16kb
	msgToClean = []byte(header + strings.Repeat("a", 16*1024) + header + strings.Repeat("b", 50))
	expectedMsg = []byte(strings.Repeat("a", 16*1024) + strings.Repeat("b", 50))
	ts, status, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, msg)

	// three times over 16kb
	msgToClean = []byte(header + strings.Repeat("a", 16*1024) + header + strings.Repeat("a", 16*1024) + header + strings.Repeat("a", 16*1024) + strings.Repeat("b", 50))
	expectedMsg = []byte(strings.Repeat("a", 16*1024) + strings.Repeat("a", 16*1024) + strings.Repeat("a", 16*1024) + strings.Repeat("b", 50))
	ts, status, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, msg)

	// with multibyte characters
	msgToClean = []byte(header + strings.Repeat("語", 16*1024) + header + strings.Repeat("語", 16*1024) + header + strings.Repeat("語", 16*1024) + strings.Repeat("言", 50))
	expectedMsg = []byte(strings.Repeat("語", 16*1024) + strings.Repeat("語", 16*1024) + strings.Repeat("語", 16*1024) + strings.Repeat("言", 50))
	ts, status, msg, err = ParseMessage(msgToClean)
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, expectedMsg, msg)
}

func TestParseMessageShouldSucceedWithValidInput(t *testing.T) {
	validMessage := header + "anything"
	ts, status, msg, err := ParseMessage([]byte(validMessage))
	assert.Nil(t, err)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", ts)
	assert.Equal(t, message.StatusInfo, status)
	assert.Equal(t, []byte("anything"), msg)
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
