// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taskverifier

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestWaitForReadyCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager, callback := NewKeyManagerWithCallback()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- manager.WaitForReady(ctx) }()
		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
		callback(nil, func(string, state.ApplyStatus) {})
		require.ErrorIs(t, manager.WaitForReady(ctx), context.Canceled)
		require.NoError(t, manager.WaitForReady(t.Context()))
	})
}
