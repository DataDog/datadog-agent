// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLengthPrefixDelimiter(t *testing.T) {

	bytes, err := LengthPrefix.delimit([]byte{})
	assert.Nil(t, err)
	assert.Equal(t, 4, len(bytes))
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x0}, bytes[:4])

	bytes, err = LengthPrefix.delimit([]byte("foo"))
	assert.Nil(t, err)
	assert.Equal(t, 7, len(bytes))
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x3}, bytes[:4])
	assert.Equal(t, "foo", string(bytes[4:]))

}

func TestLineBreakDelimiter(t *testing.T) {

	bytes, err := LineBreak.delimit([]byte{})
	assert.Nil(t, err)
	assert.Equal(t, "\n", string(bytes))

	bytes, err = LineBreak.delimit([]byte("foo"))
	assert.Nil(t, err)
	assert.Equal(t, "foo\n", string(bytes))

}
