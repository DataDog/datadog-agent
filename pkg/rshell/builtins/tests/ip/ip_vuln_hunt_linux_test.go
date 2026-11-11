// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ip_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntBuiltinFileAccessBypass_PathLikeOperandsDoNotSelectRouteFile(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)

	cases := []struct {
		name       string
		script     string
		wantCode   int
		wantStderr string
	}{
		{
			name:       "route_show_path_operand",
			script:     "ip route show /tmp/attacker-route-table",
			wantCode:   1,
			wantStderr: `unsupported argument "/tmp/attacker-route-table"`,
		},
		{
			name:       "route_get_path_operand",
			script:     "ip route get ../../etc/passwd",
			wantCode:   1,
			wantStderr: `invalid address "../../etc/passwd"`,
		},
		{
			name:       "addr_dev_path_operand",
			script:     "ip addr show dev ../../etc/passwd",
			wantCode:   1,
			wantStderr: `cannot find device "../../etc/passwd"`,
		},
		{
			name:       "link_dev_proc_operand",
			script:     "ip link show dev /proc/net/route",
			wantCode:   1,
			wantStderr: `cannot find device "/proc/net/route"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := cmdRun(t, tc.script)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr=%q", code, tc.wantCode, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr, tc.wantStderr)
			}
		})
	}
}

func TestVulnHuntBuiltinFlagDrivenExploit_EndOfFlagsDoesNotBypassWriteBlocks(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)

	cases := []struct {
		name   string
		script string
	}{
		{
			name:   "global_end_of_flags_before_route_add",
			script: "ip -- route add 10.0.0.0/8 via 192.168.1.1",
		},
		{
			name:   "route_end_of_flags_before_add",
			script: "ip route -- add 10.0.0.0/8 via 192.168.1.1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := cmdRun(t, tc.script)
			if code == 0 {
				t.Fatalf("write-like command unexpectedly succeeded; stdout=%q stderr=%q", stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "write operations are not permitted") &&
				!strings.Contains(stderr, "is unknown") {
				t.Fatalf("stderr = %q, want write-block or unknown-subcommand error", stderr)
			}
		})
	}
}

func TestVulnHuntBuiltinResourceExhaustion_PreCanceledContextsDoNotHang(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)

	for _, script := range []string{"ip addr show", "ip link show", "ip route show"} {
		t.Run(script, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			start := time.Now()
			_, _, _ = runScriptCtx(ctx, t, script, "")
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Fatalf("%s took %s with an already-canceled context", script, elapsed)
			}
		})
	}
}

func TestVulnHuntBuiltinProcFormatParsing_NonContiguousMasksAreSkipped(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"evil0\t0002010A\t00000000\t0001\t0\t0\t0\tF0F0F0F0\t0\t0\t0\n"
	writeProcNetRoute(t, content)

	stdout, stderr, code := cmdRun(t, "ip route show")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if strings.Contains(stdout, "evil0") {
		t.Fatalf("non-contiguous-mask route was printed: %q", stdout)
	}
}

func TestVulnHuntBuiltinProcFormatParsing_MetricTieBreakAndPathLikeIfaceInert(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"/tmp/evil\t0002010A\t00000000\t0001\t0\t0\t200\t00FFFFFF\t0\t0\t0\n" +
		"low0\t0002010A\t00000000\t0001\t0\t0\t10\t00FFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)

	showOut, showErr, showCode := cmdRun(t, "ip route show")
	if showCode != 0 {
		t.Fatalf("route show exit code = %d, want 0; stderr=%q", showCode, showErr)
	}
	if !strings.Contains(showOut, "dev /tmp/evil metric 200") {
		t.Fatalf("path-like iface was not treated as inert route data: %q", showOut)
	}

	getOut, getErr, getCode := cmdRun(t, "ip route get 10.1.2.3")
	if getCode != 0 {
		t.Fatalf("route get exit code = %d, want 0; stderr=%q", getCode, getErr)
	}
	if !strings.Contains(getOut, "dev low0 metric 10") {
		t.Fatalf("route get did not select the lower metric same-prefix route: %q", getOut)
	}
	if strings.Contains(getOut, "/tmp/evil") {
		t.Fatalf("high-metric path-like iface shadowed lower metric route: %q", getOut)
	}
}
