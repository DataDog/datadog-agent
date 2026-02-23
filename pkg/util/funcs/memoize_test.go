// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package funcs

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestMemoize(t *testing.T) {
	t.Run("caches successful result", func(t *testing.T) {
		var callCount atomic.Int32
		fn := Memoize(func() (string, error) {
			callCount.Add(1)
			return "hello", nil
		})

		val1, err := fn()
		require.NoError(t, err)
		assert.Equal(t, "hello", val1)

		val2, err := fn()
		require.NoError(t, err)
		assert.Equal(t, "hello", val2)

		assert.Equal(t, int32(1), callCount.Load(), "function should only be called once")
	})

	t.Run("caches error result", func(t *testing.T) {
		var callCount atomic.Int32
		expectedErr := errors.New("test error")
		fn := Memoize(func() (string, error) {
			callCount.Add(1)
			return "", expectedErr
		})

		_, err1 := fn()
		assert.ErrorIs(t, err1, expectedErr)

		_, err2 := fn()
		assert.ErrorIs(t, err2, expectedErr)

		assert.Equal(t, int32(1), callCount.Load(), "function should only be called once even on error")
	})
}

func TestMemoizeNoError(t *testing.T) {
	var callCount atomic.Int32
	fn := MemoizeNoError(func() string {
		callCount.Add(1)
		return "cached"
	})

	val1 := fn()
	assert.Equal(t, "cached", val1)

	val2 := fn()
	assert.Equal(t, "cached", val2)

	assert.Equal(t, int32(1), callCount.Load(), "function should only be called once")
}

func TestMemoizeNoErrorUnsafe(t *testing.T) {
	var callCount int
	fn := MemoizeNoErrorUnsafe(func() int {
		callCount++
		return 42
	})

	val1 := fn()
	assert.Equal(t, 42, val1)

	val2 := fn()
	assert.Equal(t, 42, val2)

	assert.Equal(t, 1, callCount, "function should only be called once")
}

func TestCache(t *testing.T) {
	t.Run("caches result and allows flush", func(t *testing.T) {
		var callCount int
		cf := Cache(func() (*string, error) {
			callCount++
			s := "result"
			return &s, nil
		})

		val1, err := cf.Do()
		require.NoError(t, err)
		assert.Equal(t, "result", *val1)
		assert.Equal(t, 1, callCount)

		val2, err := cf.Do()
		require.NoError(t, err)
		assert.Same(t, val1, val2, "should return cached pointer")
		assert.Equal(t, 1, callCount, "should not call function again")

		cf.Flush()

		val3, err := cf.Do()
		require.NoError(t, err)
		assert.Equal(t, "result", *val3)
		assert.NotSame(t, val1, val3, "should return new pointer after flush")
		assert.Equal(t, 2, callCount, "function should be called again after flush")
	})

	t.Run("does not cache errors", func(t *testing.T) {
		var callCount int
		expectedErr := errors.New("fail")
		cf := Cache(func() (*string, error) {
			callCount++
			return nil, expectedErr
		})

		val, err := cf.Do()
		assert.Nil(t, val)
		assert.ErrorIs(t, err, expectedErr)
		assert.Equal(t, 1, callCount)

		// On error, result is not cached, so function is called again
		val, err = cf.Do()
		assert.Nil(t, val)
		assert.ErrorIs(t, err, expectedErr)
		assert.Equal(t, 2, callCount, "function should be called again since error was not cached")
	})
}

func TestCacheWithCallback(t *testing.T) {
	var callCount int
	var flushCount int
	cf := CacheWithCallback(func() (*int, error) {
		callCount++
		v := callCount
		return &v, nil
	}, func() {
		flushCount++
	})

	val1, err := cf.Do()
	require.NoError(t, err)
	assert.Equal(t, 1, *val1)

	cf.Flush()
	assert.Equal(t, 1, flushCount, "callback should be called on flush")

	val2, err := cf.Do()
	require.NoError(t, err)
	assert.Equal(t, 2, *val2, "should recompute after flush")

	cf.Flush()
	assert.Equal(t, 2, flushCount, "callback should be called on each flush")
}
