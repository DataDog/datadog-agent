// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build docker

package docker

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConvert(t *testing.T) {
	convertor := &Convertor{}
	msg := string(append(stdInfoHeaderPrefix, []byte{0, 0, 0, 3}...)) + "2018-06-14T18:27:03.246999277Z msg"
	line := convertor.Convert([]byte(msg), defaultPrefix)
	assert.Equal(t, "msg", string(line.Content))
	assert.Equal(t, 3, line.Size)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", line.Timestamp)
	assert.Equal(t, "info", line.Status)
}

func TestConvertMultipleMsg(t *testing.T) {
	convertor := &Convertor{}
	var msg bytes.Buffer
	msg.Write([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1f}) // first 8 Bytes
	msg.WriteString("2018-06-14T18:27:03.246999277Z ")                // timestamp
	msg.Write([]byte("Roses are red violets are blue "))              // first message
	msg.Write([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x2c}) // 2nd 8 Bytes
	msg.WriteString("2018-06-14T18:27:03.246999278Z ")                // timestamp
	msg.Write([]byte("I have a message that’s from stderr to you"))   // 2nd message
	line := convertor.Convert(msg.Bytes(), defaultPrefix)
	assert.Equal(t, "Roses are red violets are blue I have a message that’s from stderr to you", string(line.Content))
	assert.Equal(t, 75, line.Size)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", line.Timestamp)
	assert.Equal(t, "error", line.Status)
}

func TestConvertEmptyMsg(t *testing.T) {
	convertor := &Convertor{}
	msg := string(append(stdInfoHeaderPrefix, []byte{0, 0, 0, 0}...)) + "2018-06-14T18:27:03.246999277Z "
	line := convertor.Convert([]byte(msg), defaultPrefix)
	assert.Nil(t, line)

	msgs := [3][]byte{
		[]byte(msg + "\\n"),
		[]byte(msg + "\\r"),
		[]byte(msg + "\\r\\n")}
	for _, em := range msgs {
		line = convertor.Convert(em, defaultPrefix)
		assert.Nil(t, line)
	}
}

func TestConvertLogFromTTY(t *testing.T) {
	convertor := &Convertor{}
	msg := "2018-06-14T18:27:03.246999287Z msg" + string(append(stdErrHeaderPrefix, []byte{0, 0, 0, 3}...)) + "2018-06-14T18:27:03.246999277Z msg"
	line := convertor.Convert([]byte(msg), defaultPrefix)
	assert.Equal(t, "msg\x02\x00\x00\x00\x00\x00\x00\x032018-06-14T18:27:03.246999277Z msg", string(line.Content))
	assert.Equal(t, 45, line.Size)
	assert.Equal(t, "2018-06-14T18:27:03.246999287Z", line.Timestamp)
	assert.Equal(t, "info", line.Status)

	// tty empty msg
	msgs := [5][]byte{
		[]byte("2018-06-14T18:27:03.246999287Z"),
		[]byte("2018-06-14T18:27:03.246999287Z "),
		[]byte("2018-06-14T18:27:03.246999287Z \\n"),
		[]byte("2018-06-14T18:27:03.246999287Z \\r"),
		[]byte("2018-06-14T18:27:03.246999287Z \\r\\n")}
	for _, em := range msgs {
		line = convertor.Convert(em, defaultPrefix)
		assert.Nil(t, line)
	}
}

func TestConvertPartialMsg(t *testing.T) {
	convertor := &Convertor{}
	msg := "msg d d d d "
	line := convertor.Convert([]byte(msg), defaultPrefix)
	assert.Equal(t, msg, string(line.Content))
	assert.Equal(t, len(msg), line.Size)
	assert.Equal(t, defaultPrefix.Timestamp, line.Timestamp)
	assert.Equal(t, defaultPrefix.Status, line.Status)
}

var defaultPrefix = parser.Prefix{
	Timestamp: "2018-06-14T18:27:00.246999277Z",
	Status:    message.StatusInfo,
}
var stdInfoHeaderPrefix = []byte{1, 0, 0, 0}
var stdErrHeaderPrefix = []byte{2, 0, 0, 0}
