// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package run

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestCaptureManagerDaemonLifecycle(t *testing.T) {
	var manager, sameManager *telemetrycapture.Manager
	app := fx.New(fx.NopLogger, fx.Provide(newCaptureManager), fx.Populate(&manager, &sameManager))
	require.NoError(t, app.Err())
	require.NoError(t, app.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, app.Stop(context.Background())) })
	require.Same(t, manager, sameManager)
	status := manager.Status()
	require.Equal(t, "system-probe", status.Producer.Role)
	require.Equal(t, version.AgentVersion, status.Producer.Version)
	require.Equal(t, version.FullCommit, status.Producer.Commit)
	require.NotEmpty(t, status.Producer.InstanceID)
	require.Empty(t, status.Capabilities)
	require.NoError(t, manager.Register(telemetrycapture.Capability{Stream: telemetrycapture.Connections, Cadence: time.Minute, ConnectionOwner: "system-probe"}))
	control := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "lifecycle-session-1234"}
	_, err := manager.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Connections}})
	require.NoError(t, err)
	_, err = manager.Activate(control)
	require.NoError(t, err)
	require.True(t, manager.Enabled())
	require.NoError(t, app.Stop(context.Background()))
	require.False(t, manager.Enabled())
	require.Equal(t, telemetrycapture.Failed, manager.Status().State)
	_, err = manager.Activate(control)
	require.ErrorIs(t, err, telemetrycapture.ErrClosed)
}
