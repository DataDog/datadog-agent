// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestStartStopAction(t *testing.T) {
	startStopAction := NewStartStopAction()
	count := atomic.NewInt32(0)
	start := func(context context.Context) {
		<-context.Done()
		count.Inc()
	}

	startStopAction.Start(start)
	// Check another call does nothing
	startStopAction.Start(start)

	require.Equal(t, int32(0), count.Load())

	startStopAction.Stop()
	// Check another call does nothing
	startStopAction.Stop()

	require.Equal(t, int32(1), count.Load())
}
