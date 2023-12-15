// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encodedtext

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestUTF16LEParserHandleMessages(t *testing.T) {
	parser := New(UTF16LE)
	logMessage := message.NewMessage([]byte{'F', 0x0, 'o', 0x0, 'o', 0x0}, nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.GetContent()))

	// We should support BOM
	logMessage.SetContent([]byte{0xFF, 0xFE, 'F', 0x0, 'o', 0x0, 'o', 0x0})
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.GetContent()))

	// BOM overrides endianness
	logMessage.SetContent([]byte{0xFE, 0xFF, 0x0, 'F', 0x0, 'o', 0x0, 'o'})
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.GetContent()))
}

func TestUTF16BEParserHandleMessages(t *testing.T) {
	parser := New(UTF16BE)
	logMessage := message.NewMessage([]byte{0x0, 'F', 0x0, 'o', 0x0, 'o'}, nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.GetContent()))

	// We should support BOM
	logMessage.SetContent([]byte{0xFE, 0xFF, 0x0, 'F', 0x0, 'o', 0x0, 'o'})
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.GetContent()))

	// BOM overrides endianness
	logMessage.SetContent([]byte{0xFF, 0xFE, 'F', 0x0, 'o', 0x0, 'o', 0x0})
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.GetContent()))
}

func TestSHIFTJISParserHandleMessages(t *testing.T) {
	parser := New(SHIFTJIS)
	logMessage := message.NewMessage([]byte{0x93, 0xfa, 0x96, 0x7b}, nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, "日本", string(msg.GetContent()))
}
