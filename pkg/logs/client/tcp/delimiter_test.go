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
	assert.Equal(t, &lengthPrefix, NewDelimiter(true))
	assert.Equal(t, &lineBreak, NewDelimiter(false))
}

func TestLengthPrefixDelimiter(t *testing.T) {

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

	bytes, err := lineBreak.delimit([]byte{})
	assert.Nil(t, err)
	assert.Equal(t, "\n", string(bytes))

	bytes, err = lineBreak.delimit([]byte("foo"))
	assert.Nil(t, err)
	assert.Equal(t, "foo\n", string(bytes))

}
