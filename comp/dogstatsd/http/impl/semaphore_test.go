// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSemaphore(t *testing.T) {
	t.Run("hands out exactly its slots", func(t *testing.T) {
		sem := newSemaphore(2)

		require.True(t, sem.acquire())
		require.True(t, sem.acquire())
		require.False(t, sem.acquire())

		sem.release()
		require.True(t, sem.acquire())
	})

	t.Run("a non-positive size is unlimited", func(t *testing.T) {
		for _, size := range []int{0, -1} {
			sem := newSemaphore(size)
			require.Nil(t, sem)

			for i := 0; i < 100; i++ {
				require.True(t, sem.acquire())
			}
			sem.release() // must not block or panic
		}
	})
}
