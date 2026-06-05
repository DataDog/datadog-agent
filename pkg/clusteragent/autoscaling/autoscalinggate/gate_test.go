// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscalinggate

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const testTimeout = time.Second

func TestEnable(t *testing.T) {
	gate := New()

	gate.Enable()
	gate.Enable() // can be called multiple times safely

	assert.True(t, gate.WaitForEnable(context.TODO()))
}

func TestMarkPodCollectionSynced(t *testing.T) {
	gate := New()

	gate.MarkPodCollectionSynced()
	gate.MarkPodCollectionSynced() // can be called multiple times safely

	assert.True(t, gate.WaitForPodCollectionSynced(context.TODO()))
}

func TestWaitForEnable(t *testing.T) {
	t.Run("unblocks on Enable", func(t *testing.T) {
		gate := New()

		done := make(chan bool, 1)
		go func() {
			done <- gate.WaitForEnable(context.TODO())
		}()

		gate.Enable()

		select {
		case result := <-done:
			assert.True(t, result)
		case <-time.After(testTimeout):
			t.Fatal("WaitForEnable did not unblock after Enable")
		}
	})

	t.Run("returns false on cancelled context", func(t *testing.T) {
		gate := New()

		ctx, cancel := context.WithCancel(context.TODO())
		cancel()

		assert.False(t, gate.WaitForEnable(ctx))
	})
}

func TestWaitForPodCollectionSynced(t *testing.T) {
	t.Run("unblocks on MarkPodCollectionSynced", func(t *testing.T) {
		gate := New()

		done := make(chan bool, 1)
		go func() {
			done <- gate.WaitForPodCollectionSynced(context.TODO())
		}()

		gate.MarkPodCollectionSynced()

		select {
		case result := <-done:
			assert.True(t, result)
		case <-time.After(testTimeout):
			t.Fatal("WaitForPodCollectionSynced did not unblock after MarkPodCollectionSynced")
		}
	})

	t.Run("returns false on cancelled context", func(t *testing.T) {
		gate := New()

		ctx, cancel := context.WithCancel(context.TODO())
		cancel()

		assert.False(t, gate.WaitForPodCollectionSynced(ctx))
	})
}
