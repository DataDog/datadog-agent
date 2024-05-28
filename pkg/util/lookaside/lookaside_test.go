// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package lookaside

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testStruct struct {
	idx int
}

func TestLookaside(t *testing.T) {
	l, err := New[*testStruct](2)
	assert.NoError(t, err)

	err = l.Put(&testStruct{idx: 1})
	assert.NoError(t, err)

	err = l.Put(&testStruct{idx: 2})
	assert.NoError(t, err)

	err = l.Put(&testStruct{idx: 3})
	assert.Error(t, err)

	v, err := l.Get()
	assert.NoError(t, err)
	assert.Equal(t, 2, v.idx)

	v, err = l.Get()
	assert.NoError(t, err)
	assert.Equal(t, 1, v.idx)

	_, err = l.Get()
	assert.Error(t, err)
}