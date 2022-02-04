// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringBuilder(t *testing.T) {
	var builder StringBuilder

	builder.WriteString("12345")
	assert.Equal(t, 5, builder.Len())
	assert.Equal(t, "12345", builder.String())

	builder.WriteString("123456789")
	assert.Equal(t, 9, builder.Len())
	assert.Equal(t, "123456789", builder.String())

	prevPtr := &builder.buf

	builder.WriteString("12345")
	assert.Equal(t, 5, builder.Len())
	assert.Equal(t, "12345", builder.String())

	currPtr := &builder.buf
	assert.Equal(t, prevPtr, currPtr)

	builder.Reset()
	assert.Equal(t, 0, builder.Len())
	assert.Equal(t, "", builder.String())

	currPtr = &builder.buf
	assert.Equal(t, prevPtr, currPtr)
}
