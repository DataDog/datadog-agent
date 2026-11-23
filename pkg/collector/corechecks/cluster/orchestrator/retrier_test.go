// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package orchestrator

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrier(t *testing.T) {
	t.Run("Success - Back off capped before integer overflow", func(t *testing.T) {
		r := newRetrier(time.Nanosecond, time.Duration(math.MaxInt64))

		for range 100 {
			r.Fail(errors.New("error"))
		}

		require.Equal(t, 1<<maxExponent, r.RunsToSkip())
	})
	t.Run("Success - Back off up to the maximum delay", func(t *testing.T) {
		r := newRetrier(10*time.Second, 5*time.Minute)

		skipped := make([]int, 0, 100)
		for range 100 {
			r.Fail(errors.New("error"))
			skipped = append(skipped, r.RunsToSkip())
		}

		expected := append([]int{1, 2, 4, 8, 16}, slices.Repeat([]int{30}, 95)...)
		require.Equal(t, expected, skipped)
	})
	t.Run("Success - Can run after skipped runs", func(t *testing.T) {
		r := newRetrier(10*time.Second, 5*time.Minute)
		err := errors.New("error")

		r.Fail(err)

		assert.False(t, r.CanRun())
		assert.Equal(t, 1, r.RunsToSkip())

		r.SkipRun()

		assert.True(t, r.CanRun())
		assert.Zero(t, r.RunsToSkip())
	})
	t.Run("Success - Invalid run interval", func(t *testing.T) {
		r := newRetrier(0, 5*time.Minute)
		err := errors.New("error")

		r.Fail(err)

		assert.Equal(t, err, r.LastError())
		assert.Equal(t, 1, r.RunsToSkip())
	})
	t.Run("Success - Last error overrides previous error", func(t *testing.T) {
		r := newRetrier(10*time.Second, 5*time.Minute)
		err1 := errors.New("error 1")
		err2 := errors.New("error 2")

		r.Fail(err1)
		r.Fail(err2)

		assert.Equal(t, err2, r.LastError())
		assert.Equal(t, 2, r.RunsToSkip())
	})
	t.Run("Success - Maximum delay shorter than the run interval", func(t *testing.T) {
		r := newRetrier(10*time.Second, time.Second)
		err := errors.New("error")

		r.Fail(err)

		assert.Equal(t, err, r.LastError())
		assert.Equal(t, 1, r.RunsToSkip())
	})
	t.Run("Success - Reset restarts the backoff", func(t *testing.T) {
		r := newRetrier(10*time.Second, 5*time.Minute)
		err1 := errors.New("error 1")
		err2 := errors.New("error 2")

		r.Fail(err1)

		r.Reset()
		assert.Zero(t, r.LastError())
		assert.Zero(t, r.RunsToSkip())

		r.Fail(err2)
		assert.Equal(t, err2, r.LastError())
		assert.Equal(t, 1, r.RunsToSkip())
	})
	t.Run("Success - Skip without failure is a no-op", func(t *testing.T) {
		r := newRetrier(10*time.Second, 5*time.Minute)

		assert.True(t, r.CanRun())
		assert.Zero(t, r.RunsToSkip())

		r.SkipRun()

		assert.True(t, r.CanRun())
		assert.Zero(t, r.RunsToSkip())
	})
}
