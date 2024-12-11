// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDelimiter(t *testing.T) {
	lengthD, err := NewDelimiter(true).delimit([]byte("abc"))
	assert.Nil(t, err)
	assert.Equal(t, []byte{0, 0, 0, 3, 'a', 'b', 'c'}, lengthD)
	lineD, err := NewDelimiter(false).delimit([]byte("abc"))
	assert.Nil(t, err)
	assert.Equal(t, []byte("abc\n"), lineD)
}

func TestLengthPrefixDelimiter(t *testing.T) {

	lengthPrefix := NewDelimiter(true)

	bytes, err := lengthPrefix.delimit([]byte{})
	assert.Nil(t, err)
	assert.Equal(t, 4, len(bytes))
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x0}, bytes[:4])

	bytes, err = lengthPrefix.delimit([]byte("foo"))
	assert.Nil(t, err)
	assert.Equal(t, 7, len(bytes))
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x3}, bytes[:4])
	assert.Equal(t, "foo", string(bytes[4:]))

}

func TestLineBreakDelimiter(t *testing.T) {

	lineBreak := NewDelimiter(false)

	bytes, err := lineBreak.delimit([]byte{})
	assert.Nil(t, err)
	assert.Equal(t, "\n", string(bytes))

	bytes, err = lineBreak.delimit([]byte("foo"))
	assert.Nil(t, err)
	assert.Equal(t, "foo\n", string(bytes))

}
