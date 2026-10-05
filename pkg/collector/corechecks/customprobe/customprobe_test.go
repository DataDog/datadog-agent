// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package customprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func configureCheck(t *testing.T, raw string) (*Check, *mocksender.MockSender) {
	t.Helper()
	return configureCheckWithInit(t, raw, "")
}

func configureCheckWithInit(t *testing.T, raw, init string) (*Check, *mocksender.MockSender) {
	t.Helper()
	configmock.New(t).SetInTest("hostname", "custom-probe-test-host")
	c := newCheck().(*Check)
	id := checkid.BuildID(CheckName, integration.FakeConfigHash, integration.Data(raw), integration.Data(init))
	s := mocksender.NewMockSender(t, id)
	s.On("FinalizeCheckServiceTag").Return().Once()
	s.On("SetCheckCustomTags", []string{"env:test"}).Return().Maybe()
	require.NoError(t, c.Configure(s.GetSenderManager(), integration.FakeConfigHash, integration.Data(raw), integration.Data(init), "test", "file"))
	require.Equal(t, id, c.ID())
	mocksender.SetSender(s, c.ID())
	return c, s
}

func TestRun(t *testing.T) {
	configmock.New(t).SetInTest("hostname", "custom-probe-test-host")
	host, err := hostname.Get(context.Background())
	require.NoError(t, err)

	for _, tt := range []struct {
		name          string
		body          string
		path          string
		httpStatus    int
		transportFail bool
		status        servicecheck.ServiceCheckStatus
		message       string
	}{
		{name: "ok", body: `{"replication":[{"replay_lag_bytes":9}]}`, status: servicecheck.ServiceCheckOK, message: "replication.0.replay_lag_bytes=9; warning_above=10; critical_above=20"},
		{name: "warning boundary", body: `{"replication":[{"replay_lag_bytes":10}]}`, status: servicecheck.ServiceCheckWarning, message: "replication.0.replay_lag_bytes=10; warning_above=10; critical_above=20"},
		{name: "warning", body: `{"replication":[{"replay_lag_bytes":15}]}`, status: servicecheck.ServiceCheckWarning, message: "replication.0.replay_lag_bytes=15; warning_above=10; critical_above=20"},
		{name: "critical boundary", body: `{"replication":[{"replay_lag_bytes":20}]}`, status: servicecheck.ServiceCheckCritical, message: "replication.0.replay_lag_bytes=20; warning_above=10; critical_above=20"},
		{name: "critical", body: `{"replication":[{"replay_lag_bytes":25}]}`, status: servicecheck.ServiceCheckCritical, message: "replication.0.replay_lag_bytes=25; warning_above=10; critical_above=20"},
		{name: "transport error", transportFail: true, status: servicecheck.ServiceCheckCritical, message: "HTTP probe failed"},
		{name: "HTTP error", httpStatus: http.StatusServiceUnavailable, body: `{}`, status: servicecheck.ServiceCheckCritical, message: "503 Service Unavailable"},
		{name: "HTTP redirect", httpStatus: http.StatusFound, body: `{}`, status: servicecheck.ServiceCheckCritical, message: "302 Found"},
		{name: "invalid JSON", body: `{`, status: servicecheck.ServiceCheckUnknown, message: "probe response was not valid JSON"},
		{name: "trailing garbage", body: `{} invalid`, status: servicecheck.ServiceCheckUnknown, message: "probe response was not valid JSON"},
		{name: "multiple documents", body: `{} {}`, status: servicecheck.ServiceCheckUnknown, message: "probe response was not valid JSON"},
		{name: "body at limit", body: "42" + strings.Repeat(" ", maxProbeBodyBytes-2), path: ".", status: servicecheck.ServiceCheckCritical, message: ".=42; warning_above=10; critical_above=20"},
		{name: "body over limit", body: "42" + strings.Repeat(" ", maxProbeBodyBytes-1), path: ".", status: servicecheck.ServiceCheckUnknown, message: "probe response exceeded 5MiB"},
		{name: "root dot OK", body: `9`, path: ".", status: servicecheck.ServiceCheckOK, message: ".=9; warning_above=10; critical_above=20"},
		{name: "root dot warning", body: `10`, path: ".", status: servicecheck.ServiceCheckWarning, message: ".=10; warning_above=10; critical_above=20"},
		{name: "root dot critical", body: `42`, path: ".", status: servicecheck.ServiceCheckCritical, message: ".=42; warning_above=10; critical_above=20"},
		{name: "root dollar OK", body: `9`, path: "$", status: servicecheck.ServiceCheckOK, message: "$=9; warning_above=10; critical_above=20"},
		{name: "root dollar warning", body: `10`, path: "$", status: servicecheck.ServiceCheckWarning, message: "$=10; warning_above=10; critical_above=20"},
		{name: "root dollar critical", body: `42`, path: "$", status: servicecheck.ServiceCheckCritical, message: "$=42; warning_above=10; critical_above=20"},
		{name: "missing field", body: `{"replication":[{}]}`, status: servicecheck.ServiceCheckUnknown, message: `missing field "replay_lag_bytes"`},
		{name: "non numeric", body: `{"replication":[{"replay_lag_bytes":"9"}]}`, status: servicecheck.ServiceCheckUnknown, message: "value is not numeric"},
		{name: "null", body: `{"replication":[{"replay_lag_bytes":null}]}`, status: servicecheck.ServiceCheckUnknown, message: "value is not numeric"},
		{name: "boolean", body: `{"replication":[{"replay_lag_bytes":true}]}`, status: servicecheck.ServiceCheckUnknown, message: "value is not numeric"},
		{name: "array out of bounds", body: `{"replication":[]}`, status: servicecheck.ServiceCheckUnknown, message: "invalid array index"},
		{name: "negative index", body: `{"replication":[{}]}`, path: "replication.-1.replay_lag_bytes", status: servicecheck.ServiceCheckUnknown, message: "invalid array index"},
		{name: "invalid index", body: `{"replication":[{}]}`, path: "replication.first.replay_lag_bytes", status: servicecheck.ServiceCheckUnknown, message: "invalid array index"},
		{name: "scalar traversal", body: `{"replication":1}`, status: servicecheck.ServiceCheckUnknown, message: "cannot traverse"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				if tt.httpStatus == http.StatusFound {
					w.Header().Set("Location", "/redirected")
				}
				if tt.httpStatus != 0 {
					w.WriteHeader(tt.httpStatus)
				}
				_, _ = fmt.Fprint(w, tt.body)
			}))
			t.Cleanup(server.Close)
			if tt.transportFail {
				server.Close()
			}
			path := tt.path
			if path == "" {
				path = "replication.0.replay_lag_bytes"
			}
			raw := fmt.Sprintf(`
name: replica_lag
type: http
url: %s
json_path: %s
warning_above: 10
critical_above: 20
tags: [env:test]
`, server.URL, path)
			c, s := configureCheck(t, raw)
			tags := []string{
				"env:test", "probe_type:http", "custom_probe:replica_lag",
				"correlation_key:custom_probe:replica_lag:" + host,
			}
			s.On("ServiceCheck", "replica_lag", tt.status, "", tags,
				mock.MatchedBy(func(message string) bool {
					if tt.message == "probe response was not valid JSON" {
						return message == tt.message
					}
					return strings.Contains(message, tt.message)
				})).Return().Once()
			s.On("Event", mock.MatchedBy(func(e event.Event) bool { return strings.HasPrefix(e.Title, detectionOutcome(tt.status)+":") })).Return().Once()
			s.On("Commit").Return().Once()
			if tt.status == servicecheck.ServiceCheckUnknown {
				require.Error(t, c.Run())
			} else {
				require.NoError(t, c.Run())
			}
			s.AssertExpectations(t)
		})
	}
}

func TestOptionalThresholdsAndServiceCheckName(t *testing.T) {
	configmock.New(t).SetInTest("hostname", "custom-probe-test-host")
	host, err := hostname.Get(context.Background())
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"nested":{"value":0}}`)
	}))
	defer server.Close()
	for _, tt := range []struct {
		name       string
		thresholds string
		status     servicecheck.ServiceCheckStatus
		message    string
	}{
		{"no thresholds", "", servicecheck.ServiceCheckOK, "nested.value=0; warning_above=unset; critical_above=unset"},
		{"warning only", "warning_above: 0", servicecheck.ServiceCheckWarning, "nested.value=0; warning_above=0; critical_above=unset"},
		{"critical only", "critical_above: 0", servicecheck.ServiceCheckCritical, "nested.value=0; warning_above=unset; critical_above=0"},
		{"critical precedence", "warning_above: 0\ncritical_above: 0", servicecheck.ServiceCheckCritical, "nested.value=0; warning_above=0; critical_above=0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, s := configureCheck(t, fmt.Sprintf("name: custom\ntype: http\nurl: %s\njson_path: nested.value\nservice_check_name: replica.health\n%s\n", server.URL, tt.thresholds))
			s.On("ServiceCheck", "replica.health", tt.status, "", mock.MatchedBy(func(tags []string) bool {
				return assert.ElementsMatch(t, []string{
					"probe_type:http", "custom_probe:custom",
					"correlation_key:custom_probe:custom:" + host,
				}, tags)
			}), mock.MatchedBy(func(message string) bool { return strings.Contains(message, tt.message) })).Return().Once()
			s.On("Event", mock.MatchedBy(func(e event.Event) bool { return strings.HasPrefix(e.Title, detectionOutcome(tt.status)+":") })).Return().Once()
			s.On("Commit").Return().Once()
			require.NoError(t, c.Run())
			s.AssertExpectations(t)
		})
	}
}

func TestMessageFormat(t *testing.T) {
	configmock.New(t).SetInTest("hostname", "custom-probe-test-host")
	for _, tt := range []struct {
		name       string
		value      string
		thresholds string
		message    string
	}{
		{"fractional", "1.5", "warning_above: 1.5\ncritical_above: 2.5", "value=1.5; warning_above=1.5; critical_above=2.5"},
		{"large", "16777216", "warning_above: 16777216\ncritical_above: 134217728", "value=1.6777216e+07; warning_above=1.6777216e+07; critical_above=1.34217728e+08"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, `{"value":%s}`, tt.value)
			}))
			t.Cleanup(server.Close)
			c, s := configureCheck(t, fmt.Sprintf("name: custom\ntype: http\nurl: %s\njson_path: value\n%s\n", server.URL, tt.thresholds))
			s.On("ServiceCheck", "custom", servicecheck.ServiceCheckWarning, "", mock.Anything, mock.MatchedBy(func(message string) bool { return strings.Contains(message, tt.message) })).Return().Once()
			s.On("Event", mock.MatchedBy(func(e event.Event) bool { return strings.HasPrefix(e.Title, "warning:") })).Return().Once()
			s.On("Commit").Return().Once()
			require.NoError(t, c.Run())
			s.AssertExpectations(t)
		})
	}
}

func TestAgentLocalImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		require.NoError(t, err)
		for _, imported := range source.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			require.NoError(t, err)
			assert.NotContains(t, path, "healthplatform", "%s imports %s", name, path)
			assert.NotContains(t, path, "/store", "%s imports %s", name, path)
		}
	}
}

func TestConfigureDefaultsAndMultipleInstances(t *testing.T) {
	configmock.New(t).SetInTest("hostname", "custom-probe-test-host")
	raw := "name: first\ntype: http\nurl: https://example.com/health\njson_path: value\n"
	first, firstSender := configureCheck(t, raw)
	second, secondSender := configureCheck(t, strings.Replace(raw, "first", "second", 1)+"timeout: 2\n")
	assert.NotEqual(t, first.ID(), second.ID())
	assert.Equal(t, "first", first.cfg.ServiceCheckName)
	assert.Equal(t, 5*time.Second, first.client.Timeout)
	assert.Equal(t, 2*time.Second, second.client.Timeout)
	assert.Nil(t, first.cfg.WarningAbove)
	assert.Nil(t, first.cfg.CriticalAbove)
	firstSender.AssertExpectations(t)
	secondSender.AssertExpectations(t)
}

func TestInvalidConfig(t *testing.T) {
	configmock.New(t)
	valid := "name: replica\ntype: http\nurl: http://localhost/health\njson_path: value\n"
	for _, tt := range []struct {
		name string
		raw  string
	}{
		{"missing name", strings.Replace(valid, "name: replica\n", "", 1)},
		{"missing URL", strings.Replace(valid, "url: http://localhost/health\n", "", 1)},
		{"missing type", strings.Replace(valid, "type: http\n", "", 1)},
		{"unsupported type", "name: replica\ntype: udp\n"},
		{"missing TCP host", "name: replica\ntype: tcp\nport: 5432\n"},
		{"missing TCP port", "name: replica\ntype: tcp\nhost: localhost\n"},
		{"invalid TCP port", "name: replica\ntype: tcp\nhost: localhost\nport: 65536\n"},
		{"inverted thresholds", valid + "warning_above: 100\ncritical_above: 10\n"},
		{"empty command", "name: replica\ntype: command\nrun: []\n"},
		{"empty executable", "name: replica\ntype: command\nrun: ['']\n"},
		{"empty remediation argv", valid + "remediation:\n  type: command\n  run: []\n"},
		{"string remediation", valid + "remediation:\n  type: command\n  run: echo hello\n"},
		{"missing command", "name: replica\ntype: command\n"},
		{"string command rejected", "name: replica\ntype: command\nrun: \"echo '\"\n"},
		{"missing file path", "name: replica\ntype: file\n"},
		{"negative file age", "name: replica\ntype: file\npath: /tmp/file\nmax_age_seconds: -1\n"},
		{"negative size", "name: replica\ntype: file\npath: /tmp/file\nmin_size_bytes: -1\n"},
		{"inverted size bounds", "name: replica\ntype: file\npath: /tmp/file\nmin_size_bytes: 2\nmax_size_bytes: 1\n"},
		{"invalid content regexp", valid + "content_match: '['\n"},
		{"invalid stdout regexp", "name: replica\ntype: command\nrun: [true]\nstdout_match: '['\n"},
		{"invalid status code", valid + "expected_status: 999\n"},
		{"invalid method", valid + "method: 'bad method'\n"},
		{"threshold without path", strings.Replace(valid, "json_path: value\n", "critical_above: 10\n", 1)},
		{"empty remediation", valid + "remediation: {}\n"},
		{"unsupported remediation", valid + "remediation:\n  type: tcp\n"},
		{"missing remediation command", valid + "remediation:\n  type: command\n"},
		{"missing remediation URL", valid + "remediation:\n  type: http\n"},
		{"invalid remediation timeout", valid + "remediation:\n  type: command\n  run: [true]\n  timeout: -1\n"},
		{"invalid remediation attempts", valid + "remediation:\n  type: command\n  run: [true]\n  max_attempts: -1\n"},
		{"invalid remediation backoff", valid + "remediation:\n  type: command\n  run: [true]\n  backoff_seconds: -1\n"},
		{"invalid URL", strings.Replace(valid, "http://localhost/health", "http://[", 1)},
		{"unsupported scheme", strings.Replace(valid, "http://", "file://", 1)},
		{"missing host", strings.Replace(valid, "http://localhost/health", "http:///health", 1)},
		{"negative timeout", valid + "timeout: -1\n"},
		{"zero timeout", valid + "timeout: 0\n"},
		{"infinite threshold", valid + "critical_above: .inf\n"},
		{"NaN threshold", valid + "warning_above: .nan\n"},
		{"wrong type", valid + "warning_above: [10]\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newCheck().(*Check)
			id := checkid.BuildID(CheckName, integration.FakeConfigHash, integration.Data(tt.raw), nil)
			s := mocksender.NewMockSender(t, id)
			s.On("FinalizeCheckServiceTag").Return().Maybe()
			require.Error(t, c.Configure(s.GetSenderManager(), integration.FakeConfigHash, integration.Data(tt.raw), nil, "test", "file"))
			s.AssertExpectations(t)
		})
	}
}

func expectOutcome(t *testing.T, c *Check, s *mocksender.MockSender, status servicecheck.ServiceCheckStatus, outcome, message string) {
	t.Helper()
	s.On("ServiceCheck", c.cfg.ServiceCheckName, status, "", c.tags, mock.MatchedBy(func(value string) bool {
		return value != "" && strings.Contains(value, message)
	})).Return().Once()
	s.On("Event", mock.MatchedBy(func(e event.Event) bool {
		return strings.HasPrefix(e.Title, outcome+":") && e.Text != "" &&
			e.SourceTypeName == CheckName && assert.ElementsMatch(t, c.tags, e.Tags)
	})).Return().Once()
	s.On("Commit").Return().Once()
	if status == servicecheck.ServiceCheckUnknown {
		require.Error(t, c.Run())
	} else {
		require.NoError(t, c.Run())
	}
	s.AssertExpectations(t)
}

func TestHTTPContentAndStatus(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		body   string
		status servicecheck.ServiceCheckStatus
	}{
		{"plain text", "", "ready", servicecheck.ServiceCheckOK},
		{"content match", "content_match: '^ready$'", "ready", servicecheck.ServiceCheckOK},
		{"content mismatch", "content_match: '^ready$'", "starting", servicecheck.ServiceCheckCritical},
		{"expected status", "expected_status: 202", "", servicecheck.ServiceCheckOK},
		{"unexpected status", "expected_status: 200", "", servicecheck.ServiceCheckCritical},
		{"content and threshold", "content_match: lag\njson_path: lag\ncritical_above: 20", `{"lag":25}`, servicecheck.ServiceCheckCritical},
		{"prefixed path", "json_path: $.replication[0].lag", `{"replication":[{"lag":9}]}`, servicecheck.ServiceCheckOK},
		{"dot prefixed path", "json_path: .replication.0.lag", `{"replication":[{"lag":9}]}`, servicecheck.ServiceCheckOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				w.WriteHeader(http.StatusAccepted)
				_, _ = fmt.Fprint(w, tt.body)
			}))
			t.Cleanup(server.Close)
			c, s := configureCheck(t, fmt.Sprintf("name: orders_ready\ntype: http\nurl: %s\nmethod: POST\n%s\n", server.URL, tt.config))
			expectOutcome(t, c, s, tt.status, detectionOutcome(tt.status), "")
		})
	}
}

func TestTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	c, s := configureCheck(t, fmt.Sprintf("name: postgres_listener\ntype: tcp\nhost: %s\nport: %s\n", host, port))
	expectOutcome(t, c, s, servicecheck.ServiceCheckOK, "passing", "")
	require.NoError(t, listener.Close())
	expectOutcome(t, c, s, servicecheck.ServiceCheckCritical, "detected", "")
	s.AssertNumberOfCalls(t, "Event", 2)
	s.AssertNumberOfCalls(t, "ServiceCheck", 2)
	s.AssertNumberOfCalls(t, "Commit", 2)
}

func detectionOutcome(status servicecheck.ServiceCheckStatus) string {
	return map[servicecheck.ServiceCheckStatus]string{
		servicecheck.ServiceCheckOK:       "passing",
		servicecheck.ServiceCheckWarning:  "warning",
		servicecheck.ServiceCheckCritical: "detected",
		servicecheck.ServiceCheckUnknown:  "unknown",
	}[status]
}

func argvYAML(t *testing.T, args []string) string {
	t.Helper()
	data, err := json.Marshal(args)
	require.NoError(t, err)
	return string(data)
}

func helperCommand(t *testing.T, args ...string) []string {
	t.Helper()
	t.Setenv("CUSTOM_PROBE_TEST_HELPER", "1")
	executable, err := os.Executable()
	require.NoError(t, err)
	return append([]string{executable, "-test.run=^TestProbeCommandHelper$", "--"}, args...)
}

func TestProbeCommandHelper(_ *testing.T) {
	if os.Getenv("CUSTOM_PROBE_TEST_HELPER") != "1" {
		return
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) < 1 {
		os.Exit(2)
	}
	switch args[0] {
	case "ok":
		fmt.Print("ready")
	case "echo":
		fmt.Print(strings.Join(args[1:], "\n"))
	case "exit":
		os.Exit(7)
	case "overflow":
		fmt.Print(strings.Repeat("a", maxProbeBodyBytes) + "bad")
	case "child":
		conn, err := net.Dial("tcp", args[1])
		if err != nil {
			os.Exit(5)
		}
		_, _ = io.Copy(io.Discard, conn)
		_ = conn.Close()
	case "parent":
		executable, err := os.Executable()
		if err != nil {
			os.Exit(5)
		}
		cmd := exec.Command(executable, "-test.run=^TestProbeCommandHelper$", "--", "child", args[1])
		if err := cmd.Run(); err != nil {
			os.Exit(6)
		}
	case "stderr":
		fmt.Fprint(os.Stderr, "ready")
	case "touch":
		if len(args) != 2 || os.WriteFile(args[1], []byte("ready"), 0600) != nil {
			os.Exit(3)
		}
	default:
		os.Exit(4)
	}
	os.Exit(0)
}

func TestCommand(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mode   string
		config string
		status servicecheck.ServiceCheckStatus
	}{
		{"exit zero", "ok", "", servicecheck.ServiceCheckOK},
		{"exit mismatch", "exit", "", servicecheck.ServiceCheckCritical},
		{"expected nonzero", "exit", "expected_exit: 7", servicecheck.ServiceCheckOK},
		{"stdout matches", "ok", "stdout_match: '^ready$'", servicecheck.ServiceCheckOK},
		{"stdout mismatch", "ok", "stdout_match: '^healthy$'", servicecheck.ServiceCheckCritical},
		{"stderr is not stdout", "stderr", "stdout_match: '^ready$'", servicecheck.ServiceCheckCritical},
		{"missing binary", "missing", "", servicecheck.ServiceCheckUnknown},
		{"truncated stdout", "overflow", "stdout_match: '^a+$'", servicecheck.ServiceCheckUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			run := helperCommand(t, tt.mode)
			if tt.mode == "missing" {
				run = []string{filepath.Join(t.TempDir(), "no-such-executable")}
			}
			c, s := configureCheck(t, fmt.Sprintf("name: worker_ready\ntype: command\nrun: %s\n%s\n", argvYAML(t, run), tt.config))
			expectOutcome(t, c, s, tt.status, detectionOutcome(tt.status), "")
		})
	}
}

func TestFile(t *testing.T) {
	for _, tt := range []struct {
		name    string
		config  string
		missing bool
		old     bool
		status  servicecheck.ServiceCheckStatus
	}{
		{"exists", "", false, false, servicecheck.ServiceCheckOK},
		{"missing required", "must_exist: true", true, false, servicecheck.ServiceCheckCritical},
		{"missing forbidden", "must_exist: false", true, false, servicecheck.ServiceCheckOK},
		{"exists forbidden", "must_exist: false", false, false, servicecheck.ServiceCheckCritical},
		{"matching content", "content_match: '^ready$'", false, false, servicecheck.ServiceCheckOK},
		{"content mismatch", "content_match: '^healthy$'", false, false, servicecheck.ServiceCheckCritical},
		{"fresh", "max_age_seconds: 60", false, false, servicecheck.ServiceCheckOK},
		{"stale", "max_age_seconds: 60", false, true, servicecheck.ServiceCheckCritical},
		{"size boundaries", "min_size_bytes: 5\nmax_size_bytes: 5", false, false, servicecheck.ServiceCheckOK},
		{"too small", "min_size_bytes: 6", false, false, servicecheck.ServiceCheckCritical},
		{"too large", "max_size_bytes: 4", false, false, servicecheck.ServiceCheckCritical},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "heartbeat")
			if !tt.missing {
				require.NoError(t, os.WriteFile(path, []byte("ready"), 0600))
			}
			if tt.old {
				old := time.Now().Add(-time.Hour)
				require.NoError(t, os.Chtimes(path, old, old))
			}
			c, s := configureCheck(t, fmt.Sprintf("name: heartbeat\ntype: file\npath: %q\n%s\n", path, tt.config))
			expectOutcome(t, c, s, tt.status, detectionOutcome(tt.status), "")
		})
	}
	t.Run("stat error", func(t *testing.T) {
		c, s := configureCheck(t, "name: heartbeat\ntype: file\npath: \"invalid\\0path\"\n")
		expectOutcome(t, c, s, servicecheck.ServiceCheckUnknown, "unknown", "")
	})
}

func TestRemediationCommandGate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		init    string
		extra   string
		outcome string
		status  servicecheck.ServiceCheckStatus
	}{
		{"disabled by default", "", "", "dry-run", servicecheck.ServiceCheckCritical},
		{"explicitly disabled", "enable_remediation: false", "", "dry-run", servicecheck.ServiceCheckCritical},
		{"enabled", "enable_remediation: true", "", "remediated", servicecheck.ServiceCheckOK},
		{"explicit dry run", "enable_remediation: true", "  dry_run: true\n", "dry-run", servicecheck.ServiceCheckCritical},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "worker heartbeat")
			run := helperCommand(t, "touch", path)
			raw := fmt.Sprintf("name: heartbeat\ntype: file\npath: %q\nenable_remediation: true\nremediation:\n  type: command\n  run: %s\n%s", path, argvYAML(t, run), tt.extra)
			c, s := configureCheckWithInit(t, raw, tt.init)
			assert.Contains(t, c.tags, "remediation_type:command")
			expectOutcome(t, c, s, tt.status, tt.outcome, "")
			s.AssertNumberOfCalls(t, "Event", 1)
			if tt.status == servicecheck.ServiceCheckOK {
				body, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "ready", string(body))
				assert.Equal(t, 1, c.attempts)
			} else {
				_, err := os.Stat(path)
				require.ErrorIs(t, err, os.ErrNotExist)
				assert.Zero(t, c.attempts)
			}
		})
	}
}

func TestRemediationHTTPAndBackoff(t *testing.T) {
	var attempts atomic.Int32
	var recovered atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recover" {
			assert.Equal(t, http.MethodPost, r.Method)
			attempts.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if recovered.Load() {
			_, _ = fmt.Fprint(w, `{"lag":10}`)
		} else {
			_, _ = fmt.Fprint(w, `{"lag":25}`)
		}
	}))
	t.Cleanup(server.Close)
	c, s := configureCheckWithInit(t, fmt.Sprintf("name: lag\ntype: http\nurl: %s/probe\njson_path: lag\nwarning_above: 10\ncritical_above: 20\nremediation:\n  type: http\n  url: %s/recover\n", server.URL, server.URL), "enable_remediation: true")
	assert.Contains(t, c.tags, "remediation_type:http")
	expectOutcome(t, c, s, servicecheck.ServiceCheckCritical, "escalate", "")
	assert.EqualValues(t, 1, attempts.Load())
	expectOutcome(t, c, s, servicecheck.ServiceCheckCritical, "detected", "remediation attempt budget exhausted within backoff window")
	assert.EqualValues(t, 1, attempts.Load())
	c.lastAttempt = time.Now().Add(-time.Hour)
	expectOutcome(t, c, s, servicecheck.ServiceCheckCritical, "escalate", "")
	assert.EqualValues(t, 2, attempts.Load())
	recovered.Store(true)
	expectOutcome(t, c, s, servicecheck.ServiceCheckWarning, "warning", "")
	assert.EqualValues(t, 2, attempts.Load())
	s.AssertNumberOfCalls(t, "Event", 4)
	s.AssertNumberOfCalls(t, "ServiceCheck", 4)
	s.AssertNumberOfCalls(t, "Commit", 4)
}

func TestRemediationVerify(t *testing.T) {
	for _, tt := range []struct {
		name          string
		body          string
		remediateCode int
		status        servicecheck.ServiceCheckStatus
		outcome       string
	}{
		{"recovered", `{"lag":0}`, 204, servicecheck.ServiceCheckOK, "remediated"},
		{"warning", `{"lag":10}`, 204, servicecheck.ServiceCheckWarning, "remediated"},
		{"still critical", `{"lag":25}`, 204, servicecheck.ServiceCheckCritical, "escalate"},
		{"unknown verification", `invalid`, 204, servicecheck.ServiceCheckUnknown, "escalate"},
		{"failed action", `{"lag":25}`, 500, servicecheck.ServiceCheckCritical, "escalate"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			var probes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/recover" {
					assert.Equal(t, http.MethodPut, r.Method)
					attempts.Add(1)
					w.WriteHeader(tt.remediateCode)
					return
				}
				if probes.Add(1) == 1 {
					_, _ = fmt.Fprint(w, `{"lag":25}`)
				} else {
					_, _ = fmt.Fprint(w, tt.body)
				}
			}))
			t.Cleanup(server.Close)
			c, s := configureCheckWithInit(t, fmt.Sprintf("name: lag\ntype: http\nurl: %s/probe\njson_path: lag\nwarning_above: 10\ncritical_above: 20\nremediation:\n  type: http\n  url: %s/recover\n  method: PUT\n  max_attempts: 3\n", server.URL, server.URL), "enable_remediation: true")
			expectOutcome(t, c, s, tt.status, tt.outcome, "")
			assert.EqualValues(t, 1, attempts.Load())
			assert.EqualValues(t, 2, probes.Load())
			s.AssertNumberOfCalls(t, "Event", 1)
		})
	}
}

func TestUntrustedConfigurationCannotEnableExecution(t *testing.T) {
	configmock.New(t).SetInTest("hostname", "custom-probe-test-host")
	path := filepath.Join(t.TempDir(), "must-not-exist")
	run := helperCommand(t, "touch", path)
	for _, probeType := range []string{"file", "command", "http", "tcp"} {
		t.Run(probeType, func(t *testing.T) {
			fields := map[string]string{
				"file":    fmt.Sprintf("path: %q", path),
				"command": "run: " + argvYAML(t, run),
				"http":    "url: http://169.254.169.254/metadata",
				"tcp":     "host: 169.254.169.254\nport: 80",
			}
			raw := integration.Data(fmt.Sprintf("name: untrusted\ntype: %s\n%s\nremediation:\n  type: command\n  run: %s\n", probeType, fields[probeType], argvYAML(t, run)))
			init := integration.Data("enable_remediation: true")
			c := newCheck().(*Check)
			id := checkid.BuildID(CheckName, integration.FakeConfigHash, raw, init)
			s := mocksender.NewMockSender(t, id)
			s.On("FinalizeCheckServiceTag").Return().Maybe()
			err := c.Configure(s.GetSenderManager(), integration.FakeConfigHash, raw, init, "container:test", "container")
			if probeType == "command" || probeType == "file" {
				require.ErrorContains(t, err, "trusted local file")
			} else {
				require.NoError(t, err)
				mocksender.SetSender(s, c.ID())
				expectOutcome(t, c, s, servicecheck.ServiceCheckCritical, "dry-run", "link-local/metadata targets")
				assert.Zero(t, c.attempts)
			}
			_, err = os.Stat(path)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestDryRunDoesNotExposeActionSecrets(t *testing.T) {
	for _, remediation := range []string{
		"type: command\n  run: [recover, --password, secret-password]",
		"type: http\n  url: 'http://user:secret-password@localhost/recover?token=secret-token'",
	} {
		c, s := configureCheck(t, fmt.Sprintf("name: missing\ntype: file\npath: %q\nremediation:\n  %s\n", filepath.Join(t.TempDir(), "missing"), remediation))
		s.On("Event", mock.MatchedBy(func(e event.Event) bool {
			return strings.HasPrefix(e.Title, "dry-run: would run ") && !strings.Contains(e.Title+e.Text, "secret-")
		})).Return().Once()
		s.On("ServiceCheck", "missing", servicecheck.ServiceCheckCritical, "", mock.Anything, mock.Anything).Return().Once()
		s.On("Commit").Return().Once()
		require.NoError(t, c.Run())
		s.AssertExpectations(t)
	}
}

func TestCommandTimeoutStopsDescendants(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	require.NoError(t, listener.(*net.TCPListener).SetDeadline(time.Now().Add(15*time.Second)))
	run := helperCommand(t, "parent", listener.Addr().String())
	done := make(chan error, 1)
	go func() {
		_, _, err := runCommand(run, 3)
		done <- err
	}()
	conn, err := listener.Accept()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.ErrorIs(t, <-done, context.DeadlineExceeded)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func TestInapplicableFields(t *testing.T) {
	configmock.New(t)
	configs := map[string]string{
		"http":    "url: http://localhost/health",
		"tcp":     "host: localhost\nport: 80",
		"command": "run: [echo, ready]",
		"file":    "path: /tmp/heartbeat",
	}
	for _, field := range []struct {
		name  string
		value string
		types string
	}{
		{"url", "''", "http"}, {"method", "GET", "http"},
		{"json_path", "''", "http"}, {"warning_above", "0", "http"},
		{"critical_above", "0", "http"}, {"expected_status", "0", "http"},
		{"host", "''", "tcp"}, {"port", "0", "tcp"},
		{"run", "[]", "command"}, {"expected_exit", "0", "command"},
		{"stdout_match", "''", "command"}, {"path", "''", "file"},
		{"must_exist", "false", "file"}, {"max_age_seconds", "0", "file"},
		{"min_size_bytes", "0", "file"}, {"max_size_bytes", "0", "file"},
		{"content_match", "''", "http,file"},
	} {
		for probeType, config := range configs {
			if strings.Contains(field.types, probeType) {
				continue
			}
			t.Run(probeType+"/"+field.name, func(t *testing.T) {
				raw := integration.Data(fmt.Sprintf("name: test\ntype: %s\n%s\n%s: %s\n", probeType, config, field.name, field.value))
				c := newCheck().(*Check)
				require.ErrorContains(t, c.Configure(nil, integration.FakeConfigHash, raw, nil, "test", "file"), "field "+field.name+" does not apply")
			})
		}
	}
	for _, remediation := range []string{
		"type: command\n  run: [echo]\n  url: ''",
		"type: command\n  run: [echo]\n  method: GET",
		"type: http\n  url: http://localhost\n  run: []",
	} {
		c := newCheck().(*Check)
		raw := integration.Data("name: test\ntype: tcp\nhost: localhost\nport: 80\nremediation:\n  " + remediation)
		require.ErrorContains(t, c.Configure(nil, integration.FakeConfigHash, raw, nil, "test", "file"), "does not apply")
	}
}

func TestCommandPreservesArguments(t *testing.T) {
	args := []string{`C:\Program Files\worker\ready.exe`, "two words", "'quoted'", "$(echo secret)", ""}
	run := helperCommand(t, append([]string{"echo"}, args...)...)
	c, s := configureCheck(t, "name: argv\ntype: command\nrun: "+argvYAML(t, run))
	assert.Equal(t, run, c.cfg.Run)
	stdout, exitCode, err := runCommand(c.cfg.Run, 5)
	require.NoError(t, err)
	assert.Zero(t, exitCode)
	assert.Equal(t, strings.Join(args, "\n"), string(stdout))
	s.AssertExpectations(t)
}

func TestHTTPExpectedNonSuccessStatus(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, "ready")
			}))
			t.Cleanup(server.Close)
			c, s := configureCheck(t, fmt.Sprintf("name: expected\ntype: http\nurl: %s\nexpected_status: %d\ncontent_match: '^ready$'", server.URL, status))
			expectOutcome(t, c, s, servicecheck.ServiceCheckOK, "passing", "content_match=true")
		})
	}
}

func TestHTTPOutagesTriggerRemediation(t *testing.T) {
	for _, connectionRefused := range []bool{false, true} {
		t.Run(fmt.Sprintf("connection refused=%t", connectionRefused), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			t.Cleanup(server.Close)
			if connectionRefused {
				server.Close()
			}
			path := filepath.Join(t.TempDir(), "remediation-ran")
			raw := fmt.Sprintf("name: outage\ntype: http\nurl: %s\nremediation:\n  type: command\n  run: %s", server.URL, argvYAML(t, helperCommand(t, "touch", path)))
			c, s := configureCheckWithInit(t, raw, "enable_remediation: true")
			expectOutcome(t, c, s, servicecheck.ServiceCheckCritical, "escalate", "")
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "ready", string(body))
			assert.Equal(t, 1, c.attempts)
		})
	}
}

func TestHTTPFailuresDoNotExposeSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	secretURL := strings.Replace(server.URL, "http://", "http://secret-user:secret-password@", 1) + "/health?token=secret-token#secret-fragment"
	for _, remediationFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("remediation=%t", remediationFailure), func(t *testing.T) {
			raw := "name: secrets\ntype: http\nurl: " + secretURL
			outcome := "detected:"
			if remediationFailure {
				raw = fmt.Sprintf("name: secrets\ntype: file\npath: %q\nremediation:\n  type: http\n  url: %s", filepath.Join(t.TempDir(), "missing"), secretURL)
				outcome = "escalate:"
			}
			c, s := configureCheckWithInit(t, raw, "enable_remediation: true")
			checkMessage := func(message string) bool {
				return !strings.Contains(message, "secret-") && !strings.Contains(message, "?token=") &&
					strings.Contains(message, server.URL+"/health") && (!remediationFailure || strings.Contains(message, "remediation failed"))
			}
			s.On("ServiceCheck", "secrets", servicecheck.ServiceCheckCritical, "", c.tags, mock.MatchedBy(checkMessage)).Return().Once()
			s.On("Event", mock.MatchedBy(func(e event.Event) bool {
				return strings.HasPrefix(e.Title, outcome) && checkMessage(e.Text) && !strings.Contains(e.Title, "secret-")
			})).Return().Once()
			s.On("Commit").Return().Once()
			require.NoError(t, c.Run())
			s.AssertExpectations(t)
			if remediationFailure {
				assert.Equal(t, 1, c.attempts)
			}
		})
	}
}

func TestMetadataTargetsBlocked(t *testing.T) {
	for _, host := range []string{"169.254.169.254", "169.254.0.1", "169.254.255.254", "::ffff:169.254.169.254", "fe80::1"} {
		for _, probeType := range []string{"http", "tcp"} {
			t.Run(probeType+"/"+host, func(t *testing.T) {
				fields := fmt.Sprintf("host: %s\nport: 80", host)
				if probeType == "http" {
					fields = "url: http://" + net.JoinHostPort(host, "80") + "/metadata"
				}
				c, s := configureCheck(t, fmt.Sprintf("name: metadata\ntype: %s\n%s", probeType, fields))
				expectOutcome(t, c, s, servicecheck.ServiceCheckCritical, "detected", "link-local/metadata targets")
			})
		}
		t.Run("remediation/"+host, func(t *testing.T) {
			c := &Check{cfg: instanceConfig{Remediation: &remediationConfig{
				Type: "http", URL: "http://" + net.JoinHostPort(host, "80") + "/metadata", Method: http.MethodPost, Timeout: 5,
			}}}
			require.ErrorContains(t, c.remediate(), "link-local/metadata targets")
		})
	}
}

func TestCommandRemediationFailure(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing binary=%t", missing), func(t *testing.T) {
			run := helperCommand(t, "exit")
			if missing {
				run = []string{filepath.Join(t.TempDir(), "no-such-executable")}
			}
			raw := fmt.Sprintf("name: failed_action\ntype: file\npath: %q\nremediation:\n  type: command\n  run: %s", filepath.Join(t.TempDir(), "missing"), argvYAML(t, run))
			c, s := configureCheckWithInit(t, raw, "enable_remediation: true")
			expectOutcome(t, c, s, servicecheck.ServiceCheckCritical, "escalate", "remediation failed")
			assert.Equal(t, 1, c.attempts)
		})
	}
}

func TestNonCriticalDoesNotRemediate(t *testing.T) {
	for _, tt := range []struct {
		body   string
		status servicecheck.ServiceCheckStatus
	}{
		{`{"lag":0}`, servicecheck.ServiceCheckOK},
		{`{"lag":10}`, servicecheck.ServiceCheckWarning},
		{`invalid`, servicecheck.ServiceCheckUnknown},
	} {
		t.Run(detectionOutcome(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, tt.body)
			}))
			t.Cleanup(server.Close)
			path := filepath.Join(t.TempDir(), "must-not-exist")
			raw := fmt.Sprintf("name: gate\ntype: http\nurl: %s\njson_path: lag\nwarning_above: 10\ncritical_above: 20\nremediation:\n  type: command\n  run: %s", server.URL, argvYAML(t, helperCommand(t, "touch", path)))
			c, s := configureCheckWithInit(t, raw, "enable_remediation: true")
			expectOutcome(t, c, s, tt.status, detectionOutcome(tt.status), "")
			assert.Zero(t, c.attempts)
			assert.True(t, c.lastAttempt.IsZero())
			_, err := os.Stat(path)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
