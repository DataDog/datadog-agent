// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encodedtext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUTF16LEParserHandleMessages(t *testing.T) {
	parser := New(UTF16LE)
	testMsg := []byte{'F', 0x0, 'o', 0x0, 'o', 0x0}
	msg, err := parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.Content))

	// We should support BOM
	testMsg = []byte{0xFF, 0xFE, 'F', 0x0, 'o', 0x0, 'o', 0x0}
	msg, err = parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.Content))

	// BOM overrides endianness
	testMsg = []byte{0xFE, 0xFF, 0x0, 'F', 0x0, 'o', 0x0, 'o'}
	msg, err = parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.Content))
}

func TestUTF16BEParserHandleMessages(t *testing.T) {
	parser := New(UTF16BE)
	testMsg := []byte{0x0, 'F', 0x0, 'o', 0x0, 'o'}
	msg, err := parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.Content))

	// We should support BOM
	testMsg = []byte{0xFE, 0xFF, 0x0, 'F', 0x0, 'o', 0x0, 'o'}
	msg, err = parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.Content))

	// BOM overrides endianness
	testMsg = []byte{0xFF, 0xFE, 'F', 0x0, 'o', 0x0, 'o', 0x0}
	msg, err = parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.Equal(t, "Foo", string(msg.Content))
}

func TestSHIFTJISParserHandleMessages(t *testing.T) {
	parser := New(SHIFTJIS)
	testMsg := []byte{0x93, 0xfa, 0x96, 0x7b}
	msg, err := parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.Equal(t, "日本", string(msg.Content))
}
