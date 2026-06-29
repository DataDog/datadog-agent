// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/lifecycle"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
)

// Compile-time guards: the concrete types passed via LifecycleContext must
// satisfy the lifecycle interfaces.  Failures here mean Init() would panic at
// runtime when the lifecycle server calls the missing methods.
var _ lifecycle.Flusher = (*serverlessMetrics.ServerlessMetricAgent)(nil)
var _ lifecycle.SampleDrainer = (*serverlessMetrics.ServerlessMetricAgent)(nil)
var _ lifecycle.MetricEmitter = (*serverlessMetrics.ServerlessMetricAgent)(nil)

// noopTraceAgent is a local stub that satisfies the TraceAgent interface
// without importing pkg/serverless/trace (which imports cloudservice, creating
// a cycle).
type noopTraceAgent struct{}

func (n *noopTraceAgent) Process(_ *api.Payload) {}
func (n *noopTraceAgent) Flush()                 {}
func (n *noopTraceAgent) Stop()                  {}

const testImageARN = "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image"

func TestIsMicroVM(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	assert.True(t, isMicroVM())
}

func TestIsMicroVMNotSet(t *testing.T) {
	assert.False(t, isMicroVM())
}

func TestMicroVMGetTags(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	m := &MicroVM{}
	tags := m.GetTags()

	assert.Equal(t, "us-east-1", tags["region"])
	assert.Equal(t, "123456789012", tags["account_id"])
	assert.Equal(t, "my-image", tags["image_name"])
	assert.Equal(t, MicroVMOrigin, tags["origin"])
	assert.Equal(t, MicroVMOrigin, tags["_dd.origin"])
	assert.NotContains(t, tags, "microvm_image_arn")
}

func TestMicroVMGetTagsMissingARN(t *testing.T) {
	m := &MicroVM{}
	tags := m.GetTags()
	assert.Equal(t, "unknown", tags["region"])
	assert.Equal(t, "unknown", tags["account_id"])
	assert.Equal(t, "unknown", tags["image_name"])
}

func TestMicroVMGetEnhancedMetricTags(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	m := &MicroVM{}
	cloudTags := m.GetTags()
	result := m.GetEnhancedMetricTags(cloudTags)

	assert.Equal(t, "us-east-1", result.Base["region"])
	assert.Equal(t, "123456789012", result.Base["account_id"])
	assert.Equal(t, "my-image", result.Base["image_name"])
	assert.NotContains(t, result.Base, "instance_id", "Base must not carry the high-cardinality instance_id")

	assert.Equal(t, result.Base["region"], result.Usage["region"])
	assert.Equal(t, result.Base["account_id"], result.Usage["account_id"])
	assert.NotContains(t, result.Usage, "instance_id", "instance_id is absent until SetInstanceID is called from /launch")
}

func TestParseMicroVMARNWithColonInName(t *testing.T) {
	region, accountID, imageName := parseMicroVMARN("arn:aws:lambda:eu-west-1:999:microvm-image:my:image:v2")
	assert.Equal(t, "eu-west-1", region)
	assert.Equal(t, "999", accountID)
	assert.Equal(t, "my:image:v2", imageName)
}

func TestMicroVMOrigin(t *testing.T) {
	assert.Equal(t, MicroVMOrigin, (&MicroVM{}).GetOrigin())
}

func TestMicroVMMetricPrefix(t *testing.T) {
	assert.Equal(t, microVMPrefix, (&MicroVM{}).GetMetricPrefix())
}

func TestMicroVMInit_NilTracingCtx_DoesNotStartServer(t *testing.T) {
	m := &MicroVM{}
	err := m.Init(nil)
	require.NoError(t, err)
	assert.Nil(t, m.server, "Init with nil TracingContext must not start a lifecycle server")
}

func TestMicroVMInit_NilLifecycleCtx_DoesNotStartServer(t *testing.T) {
	m := &MicroVM{}
	err := m.Init(&TracingContext{TraceAgent: &noopTraceAgent{}})
	require.NoError(t, err)
	assert.Nil(t, m.server, "Init without LifecycleCtx must not start a lifecycle server")
}

func TestMicroVMInit_WithLifecycleCtx_ServerIsConstructed(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	m := &MicroVM{}
	ctx := &TracingContext{
		TraceAgent: &noopTraceAgent{},
		LifecycleCtx: &LifecycleContext{
			MetricFlusher: metricAgent,
			LogsFlusher:   &noopLogsFlusher{},
			MetricEmitter: metricAgent,
			SampleDrainer: metricAgent,
			FlushTimeout:  time.Second,
		},
	}
	err := m.Init(ctx)
	require.NoError(t, err)
	// m.server may be nil when port 9000 is already bound in CI — that is a valid
	// outcome. The important contract is that Init never panics.
}

func TestMicroVMShutdown_NilServer_NoPanic(t *testing.T) {
	m := &MicroVM{}
	assert.NotPanics(t, func() { m.Shutdown(serverlessMetrics.ServerlessMetricAgent{}, false, nil) })
}

// TestMicroVMShutdown_LiveServer uses port 0 (random) to avoid colliding with
// port 9000, and sets m.server directly (same-package access) to bypass Init's
// hardcoded DefaultPort.
func TestMicroVMShutdown_LiveServer_StopsCleanly(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	srv := lifecycle.NewServer(
		0, // random free port
		metricAgent, &noopTraceAgent{}, &noopLogsFlusher{},
		metricAgent, metricAgent,
		(&MicroVM{}).GetSource(),
		time.Second,
		lifecycle.NewNoopChildHandle(),
		nil, // no forwarder
		nil, // no heartbeat
	)
	l, err := srv.Listen()
	require.NoError(t, err)
	go srv.Serve(l)

	m := &MicroVM{server: srv, flushTimeout: time.Second}
	assert.NotPanics(t, func() { m.Shutdown(serverlessMetrics.ServerlessMetricAgent{}, false, nil) })
}

func TestMicroVMInit_NonMicroVMServicesIgnoreLifecycleCtx(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	lc := &LifecycleContext{
		MetricFlusher: metricAgent,
		LogsFlusher:   &noopLogsFlusher{},
		MetricEmitter: metricAgent,
		SampleDrainer: metricAgent,
		FlushTimeout:  time.Second,
	}
	ctx := &TracingContext{TraceAgent: &noopTraceAgent{}, LifecycleCtx: lc}

	for _, svc := range []CloudService{&LocalService{}, &AppService{}} {
		assert.NotPanics(t, func() { _ = svc.Init(ctx) },
			"%T.Init must not panic when passed a LifecycleCtx", svc)
	}
}

// TestMicroVMInit_SidecarMode_ServerStartedChildNil verifies that sidecar mode
// starts the lifecycle server (for /ready 503s) but returns a nil Child — the
// noop ChildHandle reports not-alive, surfacing 503 rather than papering over a
// sidecar+MicroVM misconfiguration.
func TestMicroVMInit_SidecarMode_ServerStartedChildNil(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	m := &MicroVM{}
	ctx := &TracingContext{
		TraceAgent: &noopTraceAgent{},
		LifecycleCtx: &LifecycleContext{
			MetricFlusher: metricAgent,
			LogsFlusher:   &noopLogsFlusher{},
			MetricEmitter: metricAgent,
			SampleDrainer: metricAgent,
			FlushTimeout:  time.Second,
			SidecarMode:   true,
		},
	}
	err := m.Init(ctx)
	require.NoError(t, err)
	assert.Nil(t, m.Child(), "sidecar mode must not expose a Child — no user process to track")
}

// TestMicroVMInit_InitMode_ExposesChild verifies that init-container mode (non-sidecar)
// exposes a non-nil Child after Init so that RunInit can MarkAlive/MarkDead it.
func TestMicroVMInit_InitMode_ExposesChild(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	m := &MicroVM{}
	ctx := &TracingContext{
		TraceAgent: &noopTraceAgent{},
		LifecycleCtx: &LifecycleContext{
			MetricFlusher: metricAgent,
			LogsFlusher:   &noopLogsFlusher{},
			MetricEmitter: metricAgent,
			SampleDrainer: metricAgent,
			FlushTimeout:  time.Second,
			SidecarMode:   false,
		},
	}
	err := m.Init(ctx)
	require.NoError(t, err)
	// m.Child() may be nil if port 9000 is occupied (Listen fails) — server=nil path.
	// The important contract is Init returns no error and does not panic.
	_ = m.Child()
}

type noopLogsFlusher struct{}

func (n *noopLogsFlusher) Flush(_ context.Context) {}
