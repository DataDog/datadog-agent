// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package command

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestCaptureManagerSharedAndClosedWithDaemon(t *testing.T) {
	var first, second *telemetrycapture.Manager
	app := fxtest.New(t, fx.Provide(newCaptureManager),
		fx.Populate(&first), fx.Invoke(func(manager *telemetrycapture.Manager) { second = manager }))
	app.RequireStart()
	require.Same(t, first, second)
	identity := first.Status().Producer
	require.Equal(t, "process-agent", identity.Role)
	require.NotEmpty(t, identity.InstanceID)
	require.Equal(t, version.AgentVersion, identity.Version)
	require.Equal(t, version.FullCommit, identity.Commit)
	require.Empty(t, first.Status().Capabilities, "daemon construction is not producer readiness")
	require.NoError(t, first.Register(telemetrycapture.Capability{Stream: telemetrycapture.Processes, Cadence: time.Second}))
	control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "daemon-lifecycle-session"}
	_, err := first.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Processes}})
	require.NoError(t, err)
	_, err = first.Activate(control)
	require.NoError(t, err)
	app.RequireStop()
	require.False(t, first.Enabled())
	require.Equal(t, telemetrycapture.Failed, first.Status().State)
	_, err = first.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Processes}})
	require.ErrorIs(t, err, telemetrycapture.ErrClosed)
}
