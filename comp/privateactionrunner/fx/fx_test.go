// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fx

import (
	"context"
	"testing"
	"time"

	privateactionrunnerimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/impl"
	"github.com/stretchr/testify/require"
	uberfx "go.uber.org/fx"
)

func TestShutdownerAdapterStopsApplication(t *testing.T) {
	var shutdowner privateactionrunnerimpl.Shutdowner
	app := uberfx.New(
		uberfx.Provide(newShutdownerAdapter),
		uberfx.Populate(&shutdowner),
		uberfx.NopLogger,
	)
	require.NoError(t, app.Err())

	startCtx, cancelStart := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStart()
	require.NoError(t, app.Start(startCtx))
	t.Cleanup(func() {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelStop()
		require.NoError(t, app.Stop(stopCtx))
	})

	require.NoError(t, shutdowner.Shutdown())
	select {
	case <-app.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown adapter did not stop the enclosing Fx application")
	}
}
