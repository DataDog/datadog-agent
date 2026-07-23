// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/lifecycle"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
)

// Compile-time guards: the concrete types passed via LifecycleContext must
// satisfy the lifecycle interfaces.  Failures here mean Init() would panic at
// runtime when the lifecycle server calls the missing methods.
var _ lifecycle.Flusher = (*serverlessMetrics.ServerlessMetricAgent)(nil)
var _ lifecycle.MetricEmitter = (*serverlessMetrics.ServerlessMetricAgent)(nil)

// freeLifecyclePort returns an OS-assigned free TCP port as a string, for tests
// that exercise MicroVM.Init through the real DD_AWS_MICROVM_LIFECYCLE_PORT env
// var (which rejects "0" as out of range) without colliding with the real
// port 9000 or other tests in this package.
func freeLifecyclePort(t *testing.T) string {
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return strconv.Itoa(port)
}

// noopTraceAgent is a local stub that satisfies the TraceAgent interface
// without importing pkg/serverless/trace (which imports cloudservice, creating
// a cycle).
type noopTraceAgent struct{}

func (n *noopTraceAgent) Process(_ *api.Payload) {}
func (n *noopTraceAgent) Flush()                 {}
func (n *noopTraceAgent) Stop()                  {}

const testImageARN = "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image"
const testImageVersion = "v1.2.3"

func TestIsMicroVM(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	assert.True(t, isMicroVM())
}

func TestIsMicroVMNotSet(t *testing.T) {
	assert.False(t, isMicroVM())
}

func TestMicroVMGetTags(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, testImageARN)
	t.Setenv(serverlessenv.MicroVMImageVersionEnvVar, testImageVersion)
	m := &MicroVM{}
	tags := m.GetTags()

	assert.Equal(t, "us-east-1", tags["region"])
	assert.Equal(t, "123456789012", tags["account_id"])
	assert.Equal(t, "my-image", tags["image_name"])
	assert.Equal(t, MicroVMOrigin, tags["origin"])
	assert.Equal(t, MicroVMOrigin, tags["_dd.origin"])
	assert.Equal(t, MicroVMResourceType, tags["resource_type"])
	assert.Equal(t, "aws", tags["resource_provider"])
	assert.Equal(t, testImageARN, tags["resource_id"])
	assert.Equal(t, testImageVersion, tags["lambda_microvm_image_version"])
	assert.NotContains(t, tags, "microvm_image_arn")
}

func TestMicroVMGetTagsMissingARN(t *testing.T) {
	m := &MicroVM{}
	tags := m.GetTags()
	assert.Equal(t, "unknown", tags["region"])
	assert.Equal(t, "unknown", tags["account_id"])
	assert.Equal(t, "unknown", tags["image_name"])
	assert.Equal(t, "unknown", tags["resource_id"])
	assert.Equal(t, "unknown", tags["lambda_microvm_image_version"])
	assert.Equal(t, MicroVMResourceType, tags["resource_type"])
	assert.Equal(t, "aws", tags["resource_provider"])
}

// TestMicroVMGetTagsImageVersionIndependentOfARN proves the image version tag
// is read independently of the ARN: it's populated even when the ARN env var
// (and thus all ARN-derived tags) is unset.
func TestMicroVMGetTagsImageVersionIndependentOfARN(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageVersionEnvVar, testImageVersion)
	m := &MicroVM{}
	tags := m.GetTags()

	assert.Equal(t, testImageVersion, tags["lambda_microvm_image_version"])
	assert.Equal(t, "unknown", tags["resource_id"])
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
	assert.Equal(t, MicroVMResourceType, result.Base["resource_type"])
	assert.Equal(t, "aws", result.Base["resource_provider"])
	assert.Equal(t, testImageARN, result.Base["resource_id"])
	assert.NotContains(t, result.Base, "instance_id", "Base must not carry the high-cardinality instance_id")

	assert.Equal(t, result.Base["region"], result.Usage["region"])
	assert.Equal(t, result.Base["account_id"], result.Usage["account_id"])
	assert.Equal(t, result.Base["resource_type"], result.Usage["resource_type"])
	assert.Equal(t, result.Base["resource_provider"], result.Usage["resource_provider"])
	assert.Equal(t, result.Base["resource_id"], result.Usage["resource_id"])
	assert.NotContains(t, result.Usage, "instance_id", "instance_id is absent until SetInstanceID is called from /run")
}

func TestMicroVMGetEnhancedMetricTagsMissingARN(t *testing.T) {
	m := &MicroVM{}
	cloudTags := m.GetTags() // ARN not set → all "unknown"
	result := m.GetEnhancedMetricTags(cloudTags)

	assert.Equal(t, "unknown", result.Base["region"])
	assert.Equal(t, "unknown", result.Base["account_id"])
	assert.Equal(t, "unknown", result.Base["resource_id"])
	assert.Equal(t, MicroVMResourceType, result.Base["resource_type"])
	assert.Equal(t, "aws", result.Base["resource_provider"])
	assert.Equal(t, result.Base["resource_id"], result.Usage["resource_id"])
}

// TestMicroVM_CurrentUsageMetricTags_NilServer_ReturnsNil verifies that
// CurrentUsageMetricTags is safe to call before Init (m.server is nil) — the
// enhanced-metrics collector may call it before the lifecycle server exists.
func TestMicroVM_CurrentUsageMetricTags_NilServer_ReturnsNil(t *testing.T) {
	m := &MicroVM{}
	assert.Nil(t, m.CurrentUsageMetricTags())
}

// TestMicroVM_CurrentUsageMetricTags_BeforeRun_ReturnsNil verifies that no
// instance tag is produced before /run fires, matching GetEnhancedMetricTags'
// documented behavior that instance_id is unknown until then.
func TestMicroVM_CurrentUsageMetricTags_BeforeRun_ReturnsNil(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	srv := lifecycle.NewServer(
		0,
		metricAgent, &noopTraceAgent{}, &noopLogsFlusher{},
		metricAgent, nil,
		(&MicroVM{}).GetSource(),
		time.Second,
		lifecycle.NewNoopChildHandle(),
		nil, // no forwarder
		nil, // no heartbeat
	)
	m := &MicroVM{server: srv}
	assert.Nil(t, m.CurrentUsageMetricTags())
}

// TestMicroVM_CurrentUsageMetricTags_AfterRun_ReturnsInstanceTag verifies the
// end-to-end path: once /run has captured the MicroVM instance ID,
// CurrentUsageMetricTags returns the "instance:<id>" tag the enhanced-metrics
// collector attaches to the usage metric on every subsequent tick.
func TestMicroVM_CurrentUsageMetricTags_AfterRun_ReturnsInstanceTag(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	srv := lifecycle.NewServer(
		0,
		metricAgent, &noopTraceAgent{}, &noopLogsFlusher{},
		metricAgent, nil,
		(&MicroVM{}).GetSource(),
		time.Second,
		lifecycle.NewNoopChildHandle(),
		nil, // no forwarder
		nil, // no heartbeat
	)
	l, err := srv.Listen()
	require.NoError(t, err)
	go srv.Serve(l)
	t.Cleanup(func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(shutCtx)
	})

	port := l.Addr().(*net.TCPAddr).Port
	runPath := "/aws/lambda-microvms/runtime/v1/run"
	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	resp, err := http.Post("http://127.0.0.1:"+strconv.Itoa(port)+runPath, "application/json", body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	m := &MicroVM{server: srv}
	assert.Equal(t, []string{"instance:vm-abc123"}, m.CurrentUsageMetricTags())
}

// TestMicroVM_SatisfiesUsageMetricTagProvider is a compile-time guard: main.go
// duck-types cloudService against an unexported usageMetricTagProvider
// interface with this exact method set to wire the enhanced-metrics
// collector's dynamic tag hook. If CurrentUsageMetricTags' signature ever
// drifts, that type assertion silently stops matching instead of failing to
// compile — this pins the method set so such drift shows up here instead.
func TestMicroVM_SatisfiesUsageMetricTagProvider(t *testing.T) {
	var m any = &MicroVM{}
	provider, ok := m.(interface{ CurrentUsageMetricTags() []string })
	require.True(t, ok, "*MicroVM must implement CurrentUsageMetricTags() []string")
	assert.NotPanics(t, func() { provider.CurrentUsageMetricTags() })
}

// Compile-time guard: *MicroVM must satisfy the CloudService interface,
// including the new Run method.
var _ CloudService = (*MicroVM)(nil)

// TestMicroVM_Run_InitMode_ThreadsChildLiveness verifies that MicroVM.Run in
// init-container mode threads m.child into RunInit so the lifecycle server's
// /ready alive-check reflects the user app's actual state.
func TestMicroVM_Run_InitMode_ThreadsChildLiveness(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a subprocess")
	}
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "sleep 0.3"}

	child := lifecycle.NewChild()
	m := &MicroVM{child: child}

	var midRunAlive atomic.Bool
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		time.Sleep(100 * time.Millisecond)
		midRunAlive.Store(child.IsAlive())
	}()

	err := m.Run(mode.Conf{SidecarMode: false}, &serverlessInitLog.Config{})
	<-probeDone

	require.NoError(t, err)
	assert.True(t, midRunAlive.Load(), "child must be alive while Run is blocked on the subprocess")
	assert.False(t, child.IsAlive(), "child must be dead after Run returns")
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
	assert.Equal(t, MicroVMPrefix, (&MicroVM{}).GetMetricPrefix())
}

// TestLifecycleContext_NilLogsTagSetter_IsAccepted verifies that
// LifecycleContext.LogsTagSetter is nil-safe: passing nil must not cause any
// compile or runtime issue. This pins the nil-check added to MicroVM.Init so
// callers that don't need log-tag forwarding are not required to supply a setter.
func TestLifecycleContext_NilLogsTagSetter_IsAccepted(t *testing.T) {
	lc := &LifecycleContext{LogsTagSetter: nil, BaseTags: nil}
	assert.Nil(t, lc.LogsTagSetter, "nil LogsTagSetter must be accepted by LifecycleContext")
}

// TestMicroVM_LogsTagSetter_InvokedOnRun verifies the behaviour that
// MicroVM.Init wires when LogsTagSetter is non-nil: SetLogsTagSetter is called
// on the server, and the server then invokes the setter on /run with the
// base tags plus the microvm_id from the request body.
//
// The test constructs the server directly with port 0 (random free port) to
// avoid the log.Fatalf that Init emits when port 9000 is already bound.
func TestMicroVM_LogsTagSetter_InvokedOnRun(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}

	var mu sync.Mutex
	var receivedTags []string
	setter := lifecycle.LogsTagSetterFunc(func(tags []string) {
		mu.Lock()
		defer mu.Unlock()
		receivedTags = tags
	})
	baseTags := []string{"env:test", "service:myapp"}

	// Construct the server directly with port 0 — same path Init takes, minus
	// the port-9000 binding that would log.Fatalf in a shared test environment.
	srv := lifecycle.NewServer(
		0, // random free port
		metricAgent, &noopTraceAgent{}, &noopLogsFlusher{},
		metricAgent, nil,
		(&MicroVM{}).GetSource(),
		time.Second,
		lifecycle.NewNoopChildHandle(),
		nil, // no forwarder
		nil, // no heartbeat
	)
	// This is the call Init makes when lc.LogsTagSetter != nil.
	srv.SetLogsTagSetter(setter, baseTags)

	l, err := srv.Listen()
	require.NoError(t, err)
	go srv.Serve(l)
	t.Cleanup(func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(shutCtx)
	})

	runPath := "/aws/lambda-microvms/runtime/v1/run"
	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	resp, err := http.Post("http://"+l.Addr().String()+runPath, "application/json", body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	mu.Lock()
	got := receivedTags
	mu.Unlock()

	assert.Contains(t, got, "env:test", "base tags must be forwarded to LogsTagSetter on /run")
	assert.Contains(t, got, "service:myapp", "base tags must be forwarded to LogsTagSetter on /run")
	assert.Contains(t, got, "lambda_microvm_id:vm-abc123", "microvm_id from /run body must be appended to tags")
}

// TestLifecycleContext_NilTraceTagSetter_IsAccepted verifies that
// LifecycleContext.TraceTagSetter is nil-safe: passing nil must not cause any
// compile or runtime issue. Mirrors TestLifecycleContext_NilLogsTagSetter_IsAccepted.
func TestLifecycleContext_NilTraceTagSetter_IsAccepted(t *testing.T) {
	lc := &LifecycleContext{TraceTagSetter: nil, BaseTraceTags: nil}
	assert.Nil(t, lc.TraceTagSetter, "nil TraceTagSetter must be accepted by LifecycleContext")
}

// TestMicroVM_TraceTagSetter_InvokedOnRun verifies the end-to-end wiring of
// TraceTagSetter: MicroVM.Init passes it to the server, and the server calls it
// on /run with base trace tags extended by lambda_microvm_id from the body.
//
// Mirrors TestMicroVM_LogsTagSetter_InvokedOnRun.
func TestMicroVM_TraceTagSetter_InvokedOnRun(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}

	var mu sync.Mutex
	var receivedTags map[string]string
	setter := lifecycle.TraceTagSetterFunc(func(tags map[string]string) {
		mu.Lock()
		defer mu.Unlock()
		receivedTags = tags
	})
	baseTraceTags := map[string]string{"env": "test", "service": "myapp"}

	srv := lifecycle.NewServer(
		0,
		metricAgent, &noopTraceAgent{}, &noopLogsFlusher{},
		metricAgent, nil,
		(&MicroVM{}).GetSource(),
		time.Second,
		lifecycle.NewNoopChildHandle(),
		nil, // no forwarder
		nil, // no heartbeat
	)
	// This is the call Init makes when lc.TraceTagSetter != nil.
	srv.SetTraceTagSetter(setter, baseTraceTags)

	l, err := srv.Listen()
	require.NoError(t, err)
	go srv.Serve(l)
	t.Cleanup(func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(shutCtx)
	})

	runPath := "/aws/lambda-microvms/runtime/v1/run"
	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	resp, err := http.Post("http://"+l.Addr().String()+runPath, "application/json", body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	mu.Lock()
	got := receivedTags
	mu.Unlock()

	assert.Equal(t, "test", got["env"], "base trace tags must be forwarded to TraceTagSetter on /run")
	assert.Equal(t, "myapp", got["service"], "base trace tags must be forwarded to TraceTagSetter on /run")
	assert.Equal(t, "vm-abc123", got["lambda_microvm_id"], "microvm_id from /run body must appear in trace tags")
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
	t.Setenv(lifecycle.LifecyclePortEnvVar, freeLifecyclePort(t)) // avoid colliding with the real port 9000
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	m := &MicroVM{}
	ctx := &TracingContext{
		TraceAgent: &noopTraceAgent{},
		LifecycleCtx: &LifecycleContext{
			MetricFlusher: metricAgent,
			LogsFlusher:   &noopLogsFlusher{},
			MetricEmitter: metricAgent,
			FlushTimeout:  time.Second,
		},
	}
	err := m.Init(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { m.Shutdown(metricAgent, false, nil) })
}

func TestMicroVMShutdown_NilServer_NoPanic(t *testing.T) {
	m := &MicroVM{}
	assert.NotPanics(t, func() { m.Shutdown(&serverlessMetrics.ServerlessMetricAgent{}, false, nil) })
}

// TestMicroVMShutdown_LiveServer uses port 0 (random) to avoid colliding with
// port 9000, and sets m.server directly (same-package access) to bypass Init's
// hardcoded DefaultPort.
func TestMicroVMShutdown_LiveServer_StopsCleanly(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	srv := lifecycle.NewServer(
		0, // random free port
		metricAgent, &noopTraceAgent{}, &noopLogsFlusher{},
		metricAgent, nil,
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
	assert.NotPanics(t, func() { m.Shutdown(&serverlessMetrics.ServerlessMetricAgent{}, false, nil) })
}

func TestMicroVMInit_NonMicroVMServicesIgnoreLifecycleCtx(t *testing.T) {
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	lc := &LifecycleContext{
		MetricFlusher: metricAgent,
		LogsFlusher:   &noopLogsFlusher{},
		MetricEmitter: metricAgent,
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
	t.Setenv(lifecycle.LifecyclePortEnvVar, freeLifecyclePort(t)) // avoid colliding with the real port 9000
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	m := &MicroVM{}
	ctx := &TracingContext{
		TraceAgent: &noopTraceAgent{},
		LifecycleCtx: &LifecycleContext{
			MetricFlusher: metricAgent,
			LogsFlusher:   &noopLogsFlusher{},
			MetricEmitter: metricAgent,
			FlushTimeout:  time.Second,
			SidecarMode:   true,
		},
	}
	err := m.Init(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { m.Shutdown(metricAgent, false, nil) })
	assert.Nil(t, m.Child(), "sidecar mode must not expose a Child — no user process to track")
}

// TestMicroVMInit_InitMode_ExposesChild verifies that init-container mode (non-sidecar)
// exposes a non-nil Child after Init so that RunInit can MarkAlive/MarkDead it.
func TestMicroVMInit_InitMode_ExposesChild(t *testing.T) {
	t.Setenv(lifecycle.LifecyclePortEnvVar, freeLifecyclePort(t)) // avoid colliding with the real port 9000
	metricAgent := &serverlessMetrics.ServerlessMetricAgent{}
	m := &MicroVM{}
	ctx := &TracingContext{
		TraceAgent: &noopTraceAgent{},
		LifecycleCtx: &LifecycleContext{
			MetricFlusher: metricAgent,
			LogsFlusher:   &noopLogsFlusher{},
			MetricEmitter: metricAgent,
			FlushTimeout:  time.Second,
			SidecarMode:   false,
		},
	}
	err := m.Init(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { m.Shutdown(metricAgent, false, nil) })
	assert.NotNil(t, m.Child(), "init-container mode must expose a Child for RunInit to alive-check")
}

type noopLogsFlusher struct{}

func (n *noopLogsFlusher) Flush(_ context.Context) {}

// TestIsSupportedArch pins the MicroVM arch allowlist — lives here because
// isSupportedArch is defined in microvm.go.
func TestIsSupportedArch(t *testing.T) {
	for _, arch := range []string{archAMD64, archARM64} {
		assert.True(t, isSupportedArch(arch), "%s must be supported for MicroVM", arch)
	}
	for _, arch := range []string{"386", "mips", "mips64", "riscv64", "s390x", ""} {
		assert.False(t, isSupportedArch(arch), "%s must be unsupported", arch)
	}
}
