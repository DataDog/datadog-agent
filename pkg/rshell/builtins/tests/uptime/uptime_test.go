// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Integration tests for the uptime builtin. These exercise the handler's
// routing and output logic with a deterministic fake provider, so they pass
// on all platforms without OS privileges.
package uptime_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/sysinfo"
	uptimepkg "github.com/DataDog/datadog-agent/pkg/rshell/builtins/uptime"
)

// fixedNow is the reference time injected via CallContext.Now.
var fixedNow = time.Date(2026, 7, 21, 15, 36, 9, 0, time.UTC)

// fakeInfo is a deterministic sysinfo.Info for use in tests.
var fakeInfo = sysinfo.Info{
	UptimeSeconds: 93780, // 1 day, 2:03
	Load1:         1.23,
	Load5:         4.56,
	Load15:        7.89,
	LoadAvailable: true,
	BootTime:      1753052400, // fixed epoch; Local() is applied in test assertions
}

// runHandler invokes the uptime handler with the given provider and raw args.
// It returns stdout, stderr, and the result exit code.
func runHandler(t *testing.T, getInfo func() (sysinfo.Info, error), args ...string) (stdout, stderr string, code uint8) {
	t.Helper()
	fs := pflag.NewFlagSet("uptime", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	handler := uptimepkg.New(getInfo).MakeFlags((*builtins.FlagSet)(fs))
	require.NoError(t, fs.Parse(args))

	var outBuf, errBuf bytes.Buffer
	callCtx := &builtins.CallContext{
		Stdout: &outBuf,
		Stderr: &errBuf,
		Now:    fixedNow,
	}
	result := handler(context.Background(), callCtx, fs.Args())
	return outBuf.String(), errBuf.String(), result.Code
}

func give(info sysinfo.Info) func() (sysinfo.Info, error) {
	return func() (sysinfo.Info, error) { return info, nil }
}

func TestUptimeDefault(t *testing.T) {
	stdout, stderr, code := runHandler(t, give(fakeInfo))
	assert.Equal(t, " 15:36:09 up 1 day,  2:03,  load average: 1.23, 4.56, 7.89\n", stdout)
	assert.Empty(t, stderr)
	assert.Equal(t, uint8(0), code)
}

func TestUptimePretty(t *testing.T) {
	stdout, stderr, code := runHandler(t, give(fakeInfo), "-p")
	assert.Equal(t, "up 1 day, 2 hours, 3 minutes\n", stdout)
	assert.Empty(t, stderr)
	assert.Equal(t, uint8(0), code)
}

func TestUptimeSince(t *testing.T) {
	stdout, stderr, code := runHandler(t, give(fakeInfo), "-s")
	want := time.Unix(fakeInfo.BootTime, 0).Local().Format("2006-01-02 15:04:05") + "\n"
	assert.Equal(t, want, stdout)
	assert.Empty(t, stderr)
	assert.Equal(t, uint8(0), code)
}

func TestUptimeSinceBeatsP(t *testing.T) {
	// -s takes precedence over -p when both are set (matches reference behaviour).
	stdout, stderr, code := runHandler(t, give(fakeInfo), "-s", "-p")
	want := time.Unix(fakeInfo.BootTime, 0).Local().Format("2006-01-02 15:04:05") + "\n"
	assert.Equal(t, want, stdout)
	assert.Empty(t, stderr)
	assert.Equal(t, uint8(0), code)
}

func TestUptimeProviderError(t *testing.T) {
	fail := func() (sysinfo.Info, error) { return sysinfo.Info{}, fmt.Errorf("kernel read failed") }
	stdout, stderr, code := runHandler(t, fail)
	assert.Empty(t, stdout)
	assert.Equal(t, "uptime: kernel read failed\n", stderr)
	assert.Equal(t, uint8(1), code)
}

func TestUptimeNotSupported(t *testing.T) {
	fail := func() (sysinfo.Info, error) { return sysinfo.Info{}, sysinfo.ErrNotSupported }
	stdout, stderr, code := runHandler(t, fail)
	assert.Empty(t, stdout)
	// Message mentions the current OS name, so just check the prefix.
	assert.Contains(t, stderr, "uptime: not supported on ")
	assert.Equal(t, uint8(1), code)
}
