// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build quality && test && linux

package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type qualityCheck struct {
	stub.StubCheck
	id       int
	pages    *atomic.Int64
	started  *atomic.Int64
	checksum *atomic.Uint64
}

func (*qualityCheck) String() string                 { return "postgres" }
func (c *qualityCheck) ID() checkid.ID               { return checkid.ID(fmt.Sprintf("postgres:%d", c.id)) }
func (*qualityCheck) SupportsRemoteQueries() bool    { return true }
func (*qualityCheck) CancelRemoteQuery(string) error { return nil }
func (c *qualityCheck) RemoteQuery(_ context.Context, input string) (string, error) {
	var request struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal([]byte(input), &request); err != nil {
		return "", err
	}
	if request.Operation == "resolve" {
		if c.id == 0 {
			return `{"status":"MATCHED"}`, nil
		}
		return `{"status":"FAILED","error":{"code":"target_not_found","message":"not found"}}`, nil
	}
	c.started.Store(time.Now().UnixNano())
	// Simulate an integration streaming result pages without returning them to
	// the Agent. One fixed buffer is used for both result sizes. Process RSS
	// includes this constant stub cost; rows/pages never appear in the outcome.
	var page [32 * 1024]byte
	var sum uint64
	for p := int64(0); p < c.pages.Load(); p++ {
		for i := range page {
			page[i] = byte(i + int(p))
			sum += uint64(page[i])
		}
	}
	c.checksum.Add(sum)
	return `{"status":"SUCCEEDED"}`, nil
}

type qualityRC struct{ rcclient.Client }

func (qualityRC) Subscribe(string, func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

type qualityFixture struct {
	executor       *runners.WorkflowTaskExecutor
	task           *types.Task
	pages, started atomic.Int64
	checksum       atomic.Uint64
}

func qualitySetup(t *testing.T) *qualityFixture {
	t.Helper()
	f := &qualityFixture{}
	f.pages.Store(1)
	conf := config.NewMock(t)
	conf.SetInTest("remote_queries.enabled", true)
	conf.SetInTest("private_action_runner.actions_allowlist", []string{"com.datadoghq.remoteaction.queries.execute"})
	checks := make([]check.Check, 50)
	for i := range checks {
		checks[i] = &qualityCheck{id: i, pages: &f.pages, started: &f.started, checksum: &f.checksum}
	}
	svc := &remotequeries.Service{Config: conf, Checks: func() []check.Check { return checks }}
	server := httptest.NewTLSServer(ipchttp.NewHTTPMiddleware(func(string, ...interface{}) {}, "session-token")(svc))
	t.Cleanup(server.Close)
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(address.Host)
	require.NoError(t, err)
	conf.SetInTest("cmd_host", host)
	conf.SetInTest("cmd_port", port)
	client := ipchttp.NewClient("session-token", server.Client().Transport.(*http.Transport).TLSClientConfig, conf)
	defaultTimeout := int32(60)
	f.executor, err = runners.NewWorkflowTaskExecutor(&parconfig.Config{
		ActionsAllowlist: map[string]sets.Set[string]{"com.datadoghq.remoteaction.queries": sets.New("execute")},
		MetricsClient:    &statsd.NoOpClient{}, TaskTimeoutSeconds: &defaultTimeout,
	}, conf, qualityRC{}, nil, nil, nil, nil, nil, client, nil, nil, nil)
	require.NoError(t, err)
	f.task = &types.Task{}
	f.task.Data.Attributes = &types.Attributes{Inputs: map[string]interface{}{
		"integration": "postgres", "target": map[string]interface{}{"database_instance": "orders"}, "resolveOnly": false, "query": "select 1", "includeSchema": false,
		"resultDelivery": &remotequeries.Delivery{RunID: "7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c", TaskID: "d25e7f9f-6883-4462-b31e-b449f70c0894", UploadID: "quality", BaseURL: "https://datadoghq.com/intake", Limits: remotequeries.Limits{MaxFileBytes: "32768", MaxResultBytes: "999999999999", MaxRowBytes: "32768", MaxColumns: "1", MaxSchemaBytes: "1", MaxPages: "100000", TimeoutMs: "60000"}},
	}}
	f.task.Data.ID = "quality"
	f.task.Data.Attributes.BundleID = "com.datadoghq.remoteaction.queries"
	f.task.Data.Attributes.Name = "execute"
	return f
}
func (f *qualityFixture) run(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := f.executor.RunPrepared(ctx, &runners.PreparedWorkflowTask{Task: f.task})
	require.NoError(t, err)
}
func median(values []int64) int64 { slices.Sort(values); return values[len(values)/2] }

// PERF-01: opt-in, 50 loaded checks, from runner action receipt over real IPC.
func TestPERF01(t *testing.T) {
	f := qualitySetup(t)
	f.task.Data.Attributes.Inputs = map[string]interface{}{"integration": "postgres", "target": map[string]interface{}{"database_instance": "orders"}, "resolveOnly": true}
	values := make([]int64, 5)
	for i := range values {
		start := time.Now()
		f.run(t)
		values[i] = int64(time.Since(start))
	}
	result := time.Duration(median(values))
	t.Logf("resolve median of five: %s", result)
	require.Less(t, result, time.Second, "threshold is meaningful only on an idle m6gd.4xlarge")
}

// PERF-02: measure execute admission, not the stub's result generation time.
func TestPERF02(t *testing.T) {
	f := qualitySetup(t)
	values := make([]int64, 5)
	for i := range values {
		start := time.Now().UnixNano()
		f.run(t)
		values[i] = f.started.Load() - start
	}
	result := time.Duration(median(values))
	t.Logf("execute admission median of five: %s", result)
	require.Less(t, result, time.Second, "threshold is meaningful only on an idle m6gd.4xlarge")
}

func anonymousBytes(t *testing.T) int64 {
	t.Helper()
	data, err := os.ReadFile("/proc/self/status")
	require.NoError(t, err)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "RssAnon:") {
			fields := strings.Fields(line)
			n, err := strconv.ParseInt(fields[1], 10, 64)
			require.NoError(t, err)
			return n * 1024
		}
	}
	t.Fatal("RssAnon unavailable")
	return 0
}

// RES-01: compare process anonymous memory for 32 KiB and 128 MiB streamed
// outcomes. The 8 MiB tolerance absorbs allocator/HTTP/TLS bookkeeping noise;
// the required idle-host and deployed-Agent measurements remain separate.
func TestRES01(t *testing.T) {
	f := qualitySetup(t)
	f.run(t)
	measure := func(pages int64) int64 {
		f.pages.Store(pages)
		values := make([]int64, 5)
		for i := range values {
			runtime.GC()
			before := anonymousBytes(t)
			f.run(t)
			runtime.GC()
			values[i] = anonymousBytes(t) - before
		}
		return median(values)
	}
	small, large := measure(1), measure(4096)
	t.Logf("anonymous-memory delta median of five: 32 KiB=%d, 128 MiB=%d, difference=%d", small, large, large-small)
	require.LessOrEqual(t, large-small, int64(8<<20), "memory must not grow with result bytes")
}
