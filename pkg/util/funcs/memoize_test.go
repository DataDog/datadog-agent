// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package funcs

import (
	"errors"
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

func TestMemoizeArg(t *testing.T) {
	str := "asdf"
	t.Run("vals", func(t *testing.T) {
		testFunc := MemoizeArg(func(x string) (*string, error) {
			return &x, nil
		})

		val1, err := testFunc(str)
		assert.NoError(t, err)
		val2, err := testFunc(str)
		assert.NoError(t, err)
		assert.Same(t, val1, val2)

		val3, err := testFunc("xxxx")
		assert.NoError(t, err)
		assert.NotSame(t, val1, val3)
	})
	t.Run("errors", func(t *testing.T) {
		errFunc := MemoizeArg(func(x string) (*string, error) {
			return nil, errors.New(x)
		})

		val, err1 := errFunc(str)
		assert.Nil(t, val)
		assert.Error(t, err1)
		val, err2 := errFunc(str)
		assert.Nil(t, val)
		assert.Same(t, err1, err2)

		val, err3 := errFunc("xxxx")
		assert.Nil(t, val)
		assert.NotSame(t, err1, err3)
	})
}
