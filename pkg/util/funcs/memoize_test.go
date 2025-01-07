// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package funcs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemoizeArgNoError(t *testing.T) {
	testFunc := MemoizeArgNoError(func(x string) *string {
		return &x
	})

	str := "asdf"
	val1 := testFunc(str)
	val2 := testFunc(str)
	assert.Same(t, val1, val2)

	val3 := testFunc("xxxx")
	assert.NotSame(t, val1, val3)
}
