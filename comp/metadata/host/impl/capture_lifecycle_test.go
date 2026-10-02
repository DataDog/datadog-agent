// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package hostimpl

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	resources "github.com/DataDog/datadog-agent/comp/metadata/resources/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type captureLifecycleHooks struct{ hooks []compdef.Hook }

func (l *captureLifecycleHooks) Append(hook compdef.Hook) { l.hooks = append(l.hooks, hook) }

type captureBlockingResources struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (r *captureBlockingResources) Get() map[string]interface{} {
	if r.calls.Add(1) == 2 {
		close(r.entered)
		<-r.release
	}
	return map[string]interface{}{"resources": "fixture-original-resources"}
}

func TestLateMetadataCollectionCannotRestoreStoppedCaptureReadiness(t *testing.T) {
	manager := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	defer manager.Close()
	sent := &cadenceSerializer{}
	blocked := &captureBlockingResources{entered: make(chan struct{}), release: make(chan struct{})}
	deps := makeRequires(fxutil.Test[testDeps](t,
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			cfg := config.NewMock(t)
			cfg.SetInTest("enable_gohai", false)
			cfg.SetInTest("cloud_provider_metadata", []string{})
			return cfg
		}),
		fx.Provide(func() resources.Component { return blocked }),
		fx.Provide(func() serializer.MetricSerializer { return sent }),
		hostnameimpl.MockModule(),
	))
	lifecycle := &captureLifecycleHooks{}
	deps.Lc, deps.CaptureManager = lifecycle, manager
	h := NewComponent(deps).Comp.(*host)
	require.Len(t, lifecycle.hooks, 1)
	h.collect(context.Background())
	require.Len(t, manager.Status().Capabilities, 1)
	control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "metadata-shutdown-session"}
	_, err := manager.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Metadata}})
	require.NoError(t, err)
	_, err = manager.Activate(control)
	require.NoError(t, err)
	done := make(chan time.Duration, 1)
	go func() { done <- h.collect(context.Background()) }()
	select {
	case <-blocked.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("metadata collection did not reach the blocking producer")
	}
	// The runner may time out while waiting for this provider during shutdown.
	require.NoError(t, lifecycle.hooks[0].OnStop(context.Background()))
	require.False(t, manager.Enabled())
	require.Empty(t, manager.Status().Capabilities)
	close(blocked.release)
	select {
	case cadence := <-done:
		require.Equal(t, 3*defaultEarlyInterval, cadence)
	case <-time.After(5 * time.Second):
		t.Fatal("capture shutdown blocked the original metadata submission")
	}
	require.Equal(t, 2, sent.calls, "late completion must still follow normal delivery")
	require.Equal(t, "fixture-original-resources", sent.payload.ResourcesPayload)
	require.Empty(t, manager.Status().Capabilities, "late completion must not re-register readiness")
	require.False(t, manager.Enabled())
	require.Equal(t, telemetrycapture.Failed, manager.Status().State)
}
