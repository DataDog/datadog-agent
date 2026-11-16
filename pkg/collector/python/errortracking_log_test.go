// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package python

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	logslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

const sitePackages = "/opt/datadog-agent/embedded/lib/python3.13/site-packages/"

// wireFrameRe is the frame shape the error tracking parser expects.
var wireFrameRe = regexp.MustCompile(`^\S+\n\t\S+:\d+ \+0x[0-9a-f]+$`)

func TestParseTraceback(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		want     []tracebackFrame
		wantType string
	}{
		{
			name: "simple traceback",
			text: "Traceback (most recent call last):\n" +
				"  File \"/etc/datadog-agent/checks.d/mine.py\", line 7, in check\n" +
				"    raise ValueError(\"secret token abc\")\n" +
				"ValueError: secret token abc\n",
			want:     []tracebackFrame{{"/etc/datadog-agent/checks.d/mine.py", 7, "check"}},
			wantType: "ValueError",
		},
		{
			name: "only the last chained traceback is used",
			text: "Traceback (most recent call last):\n" +
				"  File \"a.py\", line 1, in first\n" +
				"KeyError: 'x'\n\n" +
				"During handling of the above exception, another exception occurred:\n\n" +
				"Traceback (most recent call last):\n" +
				"  File \"b.py\", line 2, in second\n" +
				"    foo()\n" +
				"redis.exceptions.ConnectionError: Error 111 connecting to 10.0.0.5:6379\n",
			want:     []tracebackFrame{{"b.py", 2, "second"}},
			wantType: "redis.exceptions.ConnectionError",
		},
		{
			name: "exception without message and multi-line message",
			text: "Traceback (most recent call last):\n" +
				"  File \"a.py\", line 3, in f\n" +
				"    ^^^^\n" +
				"MyError\n",
			want:     []tracebackFrame{{"a.py", 3, "f"}},
			wantType: "MyError",
		},
		{
			name: "capture text without header or exception line",
			text: "  File \"a.py\", line 1, in outer\n  File \"b.py\", line 2, in inner\n",
			want: []tracebackFrame{{"a.py", 1, "outer"}, {"b.py", 2, "inner"}},
		},
		{
			name:     "garbage exception line is not trusted as a type",
			text:     "  File \"a.py\", line 1, in f\nsome free text: with colon\n",
			want:     []tracebackFrame{{"a.py", 1, "f"}},
			wantType: "",
		},
		{name: "empty", text: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, gotType := parseTraceback(tc.text)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantType, gotType)
		})
	}
}

func TestShippedIntegrationsList(t *testing.T) {
	require.NotEmpty(t, shippedIntegrations, "the embedded list must not be empty")
	assert.Greater(t, len(shippedIntegrations), 200, "the snapshot looks truncated")

	valid := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for name := range shippedIntegrations {
		assert.Regexp(t, valid, name)
		assert.NotContains(t, basePackages, name, "base packages are not listed as integrations")
	}
	// Integrations used by the end to end tests and a few well known ones.
	for _, name := range []string{"directory", "tcp_check", "redisdb", "postgres", "nginx", "process"} {
		assert.Contains(t, shippedIntegrations, name)
	}
}

func TestRedactExceptionType(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"ValueError", "ValueError"},
		{"datadog_checks.base.errors.CheckException", "datadog_checks.base.errors.CheckException"},
		{"datadog_checks.redisdb.redisdb.RedisError", "datadog_checks.redisdb.redisdb.RedisError"},
		{"redis.exceptions.ConnectionError", "<external>.ConnectionError"},
		{"socket.gaierror", "<external>.gaierror"},
		{"acme_billing.errors.InvoiceError", "<external>.InvoiceError"},
		{"datadog_checks.acme_billing.AcmeError", "<external>.AcmeError"},
		{"__main__.CustomError", "<external>.CustomError"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, redactExceptionType(tc.in))
		})
	}
}

func TestParseTracebackAdversarial(t *testing.T) {
	frame := func(path string, line int, fn string) string {
		return fmt.Sprintf("  File %q, line %d, in %s\n", path, line, fn)
	}
	tests := []struct {
		name     string
		text     string
		want     []tracebackFrame
		wantType string
	}{
		{
			name: "frames forged in a multi-line message are ignored",
			text: tracebackHeader + "\n" + frame("a.py", 1, "real") +
				"ValueError: first line\n" +
				"  File \"datadog_checks/redisdb/forged.py\", line 9, in forged\n" +
				"KeyError: forged\n",
			want:     []tracebackFrame{{"a.py", 1, "real"}},
			wantType: "ValueError",
		},
		{
			name: "a forged header without a chain marker is ignored",
			text: tracebackHeader + "\n" + frame("a.py", 1, "real") +
				"ValueError: first line\n" +
				tracebackHeader + "\n" + frame("forged.py", 2, "forged") + "KeyError: x\n",
			want:     []tracebackFrame{{"a.py", 1, "real"}},
			wantType: "ValueError",
		},
		{
			name: "a chained exception is followed",
			text: tracebackHeader + "\n" + frame("a.py", 1, "first") + "KeyError: x\n\n" +
				chainedContext + "\n\n" +
				tracebackHeader + "\n" + frame("b.py", 2, "second") + "ValueError: y\n",
			want:     []tracebackFrame{{"b.py", 2, "second"}},
			wantType: "ValueError",
		},
		{
			name:     "frame lines must be indented like Python prints them",
			text:     tracebackHeader + "\nFile \"forged.py\", line 1, in f\nValueError\n",
			want:     nil,
			wantType: "",
		},
		{
			name:     "no exception line after the frames",
			text:     tracebackHeader + "\n" + frame("a.py", 1, "f"),
			want:     []tracebackFrame{{"a.py", 1, "f"}},
			wantType: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, gotType := parseTraceback(tc.text)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantType, gotType)
		})
	}
}

// Python 3.13 prints tracebacks of exception groups with every line prefixed
// by a bar. They are not parsed, so a check raising a group is reported
// without a stack and the caller falls back to the plain error.
func TestParseTracebackExceptionGroup(t *testing.T) {
	text := "  + Exception Group Traceback (most recent call last):\n" +
		"  |   File \"" + sitePackages + "datadog_checks/redisdb/redisdb.py\", line 42, in check\n" +
		"  | ExceptionGroup: boom (2 sub-exceptions)\n" +
		"  +-+---------------- 1 ----------------\n" +
		"    | Traceback (most recent call last):\n" +
		"    |   File \"a.py\", line 1, in f\n" +
		"    | ValueError: x\n"
	frames, excType := parseTraceback(text)
	assert.Empty(t, frames)
	assert.Empty(t, excType)

	report, err := json.Marshal([]map[string]string{{"message": "boom", "traceback": text}})
	require.NoError(t, err)
	var p errortracking.StackProvider
	assert.False(t, errors.As(newCheckError(string(report)), &p))
}

func TestClassifyFramePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantRel  string
		wantKind frameKind
	}{
		{"shipped integration", sitePackages + "datadog_checks/redisdb/redisdb.py", "datadog_checks/redisdb/redisdb.py", kindShipped},
		{"shipped base", sitePackages + "datadog_checks/base/checks/base.py", "datadog_checks/base/checks/base.py", kindShipped},
		{"windows path", `C:\Program Files\Datadog\embedded3\Lib\site-packages\datadog_checks\nginx\nginx.py`, "datadog_checks/nginx/nginx.py", kindShipped},
		{"custom check in checks.d", "/etc/datadog-agent/checks.d/error_check.py", "", kindCustom},
		{"custom check windows", `C:\ProgramData\Datadog\checks.d\mine.py`, "", kindCustom},
		{"standard library", "/opt/datadog-agent/embedded/lib/python3.13/logging/__init__.py", "", kindExternal},
		{"third party", sitePackages + "redis/connection.py", "", kindExternal},
		{"datadog_checks outside site-packages", "/home/alice/work/datadog_checks/mine/mine.py", "", kindExternal},
		{"traversal is not shipped", sitePackages + "datadog_checks/../../../etc/passwd.py", "", kindExternal},
		{"odd characters are not shipped", sitePackages + "datadog_checks/my check/x.py", "", kindExternal},
		{"non python file", sitePackages + "datadog_checks/redisdb/data.so", "", kindExternal},
		{"empty", "", "", kindExternal},
		// The namespace is shared with partner and customer wheels, so a
		// package that is not on the shipped list is treated as customer code.
		{"unlisted package in the shared namespace", sitePackages + "datadog_checks/acme_billing/acme_billing.py", "", kindExternal},
		{"module directly in the namespace", sitePackages + "datadog_checks/errors.py", "", kindExternal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rel, kind := classifyFramePath(tc.path)
			assert.Equal(t, tc.wantRel, rel)
			assert.Equal(t, tc.wantKind, kind)
		})
	}
}

// logCapture mimics the frames seen at a self.log.error call in a shipped
// integration, outermost first.
func logCapture(integration string) []tracebackFrame {
	return []tracebackFrame{
		{sitePackages + "datadog_checks/base/checks/base.py", 1200, "run"},
		{sitePackages + "datadog_checks/" + integration + "/" + integration + ".py", 42, "check"},
		{sitePackages + "datadog_checks/base/log.py", 80, "error"},
		{"/opt/datadog-agent/embedded/lib/python3.13/logging/__init__.py", 1500, "error"},
		{"/opt/datadog-agent/embedded/lib/python3.13/logging/__init__.py", 1600, "_log"},
		{sitePackages + "datadog_checks/base/log.py", 60, "emit"},
	}
}

func TestBuildExternalStack(t *testing.T) {
	t.Run("log path drops logging plumbing and starts at the integration", func(t *testing.T) {
		got, ok := buildExternalStack(logCapture("redisdb"), "ConnectionError", true)
		require.True(t, ok)

		assert.Equal(t, "redisdb", got.Source)
		assert.Equal(t, "ConnectionError", got.ErrorKind)
		wantFrames := "datadog_checks.redisdb.redisdb.check\n\tdatadog_checks/redisdb/redisdb.py:42 +0x0\n" +
			"datadog_checks.base.checks.base.run\n\tdatadog_checks/base/checks/base.py:1200 +0x0"
		assert.Equal(t, wantFrames, got.Frames)
	})

	t.Run("raise path keeps the external frame as a placeholder", func(t *testing.T) {
		frames := []tracebackFrame{
			{sitePackages + "datadog_checks/base/checks/base.py", 1200, "run"},
			{sitePackages + "datadog_checks/redisdb/redisdb.py", 42, "check"},
			{sitePackages + "redis/client.py", 10, "ping"},
			{sitePackages + "redis/connection.py", 20, "connect"},
		}
		got, ok := buildExternalStack(frames, "ConnectionError", false)
		require.True(t, ok)

		assert.True(t, strings.HasPrefix(got.Frames, "<external>\n\t<redacted>:0 +0x0\n"),
			"two consecutive third party frames collapse to one placeholder: %s", got.Frames)
		assert.Equal(t, 1, strings.Count(got.Frames, "<external>"))
		assert.Equal(t, "redisdb", got.Source)
	})

	t.Run("custom check frames are redacted", func(t *testing.T) {
		frames := []tracebackFrame{
			{sitePackages + "datadog_checks/base/checks/base.py", 1200, "run"},
			{"/etc/datadog-agent/checks.d/acme_billing_check.py", 7, "check_acme_secret_path"},
		}
		got, ok := buildExternalStack(frames, "ValueError", false)
		require.True(t, ok)

		assert.True(t, strings.HasPrefix(got.Frames, "<custom_check>\n\t<redacted>:0 +0x0"))
		assert.NotContains(t, got.Frames, "acme")
		assert.NotContains(t, got.Frames, "checks.d")
		assert.Equal(t, customSource, got.Source)
	})

	t.Run("nothing to ship", func(t *testing.T) {
		_, ok := buildExternalStack([]tracebackFrame{{"/usr/lib/python3/logging/__init__.py", 1, "error"}}, "", true)
		assert.False(t, ok, "only logging plumbing: caller falls back to the plain log")
		_, ok = buildExternalStack(nil, "", true)
		assert.False(t, ok)
	})

	t.Run("base only stack is tagged base", func(t *testing.T) {
		got, ok := buildExternalStack([]tracebackFrame{{sitePackages + "datadog_checks/base/utils/http.py", 5, "get"}}, "", false)
		require.True(t, ok)
		assert.Equal(t, baseSource, got.Source)
	})

	t.Run("stack is capped at the frame limit keeping the innermost", func(t *testing.T) {
		var frames []tracebackFrame
		for i := 0; i < 40; i++ {
			frames = append(frames, tracebackFrame{sitePackages + "datadog_checks/redisdb/redisdb.py", i, fmt.Sprintf("f%d", i)})
		}
		got, ok := buildExternalStack(frames, "", false)
		require.True(t, ok)
		lines := strings.Split(got.Frames, "\n")
		assert.Len(t, lines, 2*errortracking.MaxStackFrames)
		assert.Contains(t, lines[0], "redisdb.f39")
	})

	t.Run("odd function names and negative lines cannot break the wire shape", func(t *testing.T) {
		frames := []tracebackFrame{{sitePackages + "datadog_checks/redisdb/redisdb.py", -3, "bad name\nwith newline"}}
		got, ok := buildExternalStack(frames, "", false)
		require.True(t, ok)
		assert.Equal(t, "datadog_checks.redisdb.redisdb.unknown\n\tdatadog_checks/redisdb/redisdb.py:0 +0x0", got.Frames)
	})

	t.Run("every frame matches the wire shape and leaks no install path", func(t *testing.T) {
		got, ok := buildExternalStack(logCapture("nginx"), "", true)
		require.True(t, ok)
		parts := strings.Split(got.Frames, "\n")
		for i := 0; i+1 < len(parts); i += 2 {
			assert.Regexp(t, wireFrameRe, parts[i]+"\n"+parts[i+1])
		}
		assert.NotContains(t, got.Frames, "/opt/")
		assert.NotContains(t, got.Frames, "site-packages")
	})
}

func TestBuildExternalStack_DistinctIntegrationsDiffer(t *testing.T) {
	a, okA := buildExternalStack(logCapture("redisdb"), "", true)
	b, okB := buildExternalStack(logCapture("nginx"), "", true)
	require.True(t, okA && okB)

	assert.NotEqual(t, a.Frames, b.Frames, "different integrations must produce different dedup keys")
	assert.NotEqual(t, a.Source, b.Source)
}

func TestStackFromCapture(t *testing.T) {
	text := "  File \"" + sitePackages + "datadog_checks/redisdb/redisdb.py\", line 42, in check\n" +
		"  File \"" + sitePackages + "datadog_checks/base/log.py\", line 60, in emit\n"

	got, ok := stackFromCapture(text, "redis.exceptions.ConnectionError")
	require.True(t, ok)
	assert.Equal(t, "<external>.ConnectionError", got.ErrorKind, "third party exception modules are not shipped")
	assert.Equal(t, "redisdb", got.Source)

	got, ok = stackFromCapture(text, "not a type\nname")
	require.True(t, ok)
	assert.Empty(t, got.ErrorKind, "an invalid exception type is dropped")

	_, ok = stackFromCapture("", "")
	assert.False(t, ok)
}

func TestNewCheckError(t *testing.T) {
	traceback := "Traceback (most recent call last):\n" +
		"  File \"" + sitePackages + "datadog_checks/base/checks/base.py\", line 1200, in run\n" +
		"    self.check(instance)\n" +
		"  File \"" + sitePackages + "datadog_checks/redisdb/redisdb.py\", line 42, in check\n" +
		"    conn.ping()\n" +
		"redis.exceptions.ConnectionError: Error connecting to db.internal.example.com\n"
	report, err := json.Marshal([]map[string]string{{"message": "Error connecting to db.internal.example.com", "traceback": traceback}})
	require.NoError(t, err)

	t.Run("raised exception carries a redacted stack and keeps its text", func(t *testing.T) {
		err := newCheckError(string(report))

		assert.Equal(t, string(report), err.Error(), "error text must stay byte identical")
		wrapped := fmt.Errorf("check failed: %w", err)
		var p errortracking.StackProvider
		require.ErrorAs(t, wrapped, &p)
		stack := p.ExternalStack()
		assert.Equal(t, "<external>.ConnectionError", stack.ErrorKind)
		assert.Equal(t, "redisdb", stack.Source)
		assert.True(t, strings.HasPrefix(stack.Frames, "datadog_checks.redisdb.redisdb.check\n"))
		assert.NotContains(t, stack.Frames, "example.com", "exception message must never be shipped")
	})

	for _, tc := range []struct{ name, result string }{
		{"not json", "something went wrong"},
		{"empty report", "[]"},
		{"no traceback", `[{"message":"m","traceback":""}]`},
		{"only external frames", `[{"message":"m","traceback":"  File \"/usr/lib/x.py\", line 1, in f\nError\n"}]`},
	} {
		t.Run("plain error for "+tc.name, func(t *testing.T) {
			err := newCheckError(tc.result)

			assert.EqualError(t, err, tc.result)
			var p errortracking.StackProvider
			assert.False(t, errors.As(err, &p))
		})
	}
}

func TestLogMessageWithStack(t *testing.T) {
	var mu sync.Mutex
	var logs []errortracking.ErrorLog
	submit := errortracking.Submitter(func(e errortracking.ErrorLog) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, e)
	})
	bouncer := errortracking.NewBouncer(time.Hour, 0)
	handler := errortracking.NewHandler(func() errortracking.Submitter { return submit }).
		WithBouncerLoader(func() *errortracking.Bouncer { return bouncer })
	log.SetupLogger(logslog.NewWrapper(handler), "debug")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "debug") })

	capture := "  File \"" + sitePackages + "datadog_checks/redisdb/redisdb.py\", line 42, in check\n" +
		"  File \"" + sitePackages + "datadog_checks/base/log.py\", line 60, in emit\n"

	t.Run("error and critical are logged with the python stack", func(t *testing.T) {
		for _, level := range []int{pythonLevelError, pythonLevelCritical} {
			mu.Lock()
			logs = nil
			mu.Unlock()
			bouncer = errortracking.NewBouncer(time.Hour, 0)

			require.True(t, logMessageWithStack(level, "user data in message", capture, "ConnectionError"))

			mu.Lock()
			require.Len(t, logs, 1)
			assert.Equal(t, "ConnectionError", logs[0].ErrorKind)
			assert.Equal(t, "redisdb", logs[0].Integration)
			assert.True(t, strings.HasPrefix(logs[0].Stack, "datadog_checks.redisdb.redisdb.check\n"))
			mu.Unlock()
		}
	})

	t.Run("falls back without logging", func(t *testing.T) {
		mu.Lock()
		logs = nil
		mu.Unlock()

		assert.False(t, logMessageWithStack(30, "warning", capture, ""), "only error and critical carry a stack")
		assert.False(t, logMessageWithStack(pythonLevelError, "no frames", "", ""))
		assert.False(t, logMessageWithStack(pythonLevelError, "only stdlib", "  File \"/usr/lib/python3/logging/__init__.py\", line 1, in error\n", ""))

		mu.Lock()
		assert.Empty(t, logs)
		mu.Unlock()
	})
}
