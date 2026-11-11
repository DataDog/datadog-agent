// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ps_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script string) (stdout, stderr string, code int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption))
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	runErr := runner.Run(context.Background(), prog)
	exitCode := 0
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected runner error: %v", runErr)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// TestPSDefaultRuns ensures ps runs without error and produces a PID header.
func TestPSDefaultRuns(t *testing.T) {
	stdout, stderr, code := runScript(t, "ps")
	if code != 0 {
		t.Fatalf("ps exited %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "PID") {
		t.Errorf("expected PID column header, got:\n%s", stdout)
	}
}

// TestPSAllFlag ensures -e produces output and includes a PID header.
func TestPSAllFlag(t *testing.T) {
	stdout, stderr, code := runScript(t, "ps -e")
	if code != 0 {
		t.Fatalf("ps -e exited %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "PID") {
		t.Errorf("expected PID column header, got:\n%s", stdout)
	}
}

// TestPSCapitalAFlag ensures -A is equivalent to -e.
func TestPSCapitalAFlag(t *testing.T) {
	stdout, stderr, code := runScript(t, "ps -A")
	if code != 0 {
		t.Fatalf("ps -A exited %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "PID") {
		t.Errorf("expected PID column header, got:\n%s", stdout)
	}
}

// TestPSFullFormat ensures -f adds extra columns including PPID.
func TestPSFullFormat(t *testing.T) {
	stdout, stderr, code := runScript(t, "ps -ef")
	if code != 0 {
		t.Fatalf("ps -ef exited %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "PPID") {
		t.Errorf("expected PPID column header, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "UID") {
		t.Errorf("expected UID column header, got:\n%s", stdout)
	}
}

// TestPSByPID ensures -p shows the specified PID.
func TestPSByPID(t *testing.T) {
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "ps -p "+strconv.Itoa(selfPID))
	if code != 0 {
		t.Fatalf("ps -p %d exited %d; stderr: %s", selfPID, code, stderr)
	}
	if !strings.Contains(stdout, strconv.Itoa(selfPID)) {
		t.Errorf("expected PID %d in output, got:\n%s", selfPID, stdout)
	}
}

// TestPSByPIDCommaList ensures -p handles comma-separated PIDs.
func TestPSByPIDCommaList(t *testing.T) {
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "ps -p 1,"+strconv.Itoa(selfPID))
	if code != 0 {
		t.Fatalf("ps -p comma list exited %d; stderr: %s", code, stderr)
	}
	// At least the header should be there.
	if !strings.Contains(stdout, "PID") {
		t.Errorf("expected PID header, got:\n%s", stdout)
	}
}

// TestPSInvalidPIDExits1 ensures an invalid PID exits with code 1.
func TestPSInvalidPIDExits1(t *testing.T) {
	_, _, code := runScript(t, "ps -p notapid")
	if code != 1 {
		t.Errorf("expected exit code 1 for invalid PID, got %d", code)
	}
}

// TestPSNegativePIDExits1 ensures a negative PID exits with code 1.
func TestPSNegativePIDExits1(t *testing.T) {
	_, _, code := runScript(t, "ps -p -999")
	if code != 1 {
		t.Errorf("expected exit code 1 for negative PID, got %d", code)
	}
}

// TestPSZeroPIDExits1 ensures PID 0 is rejected (not a valid process PID).
func TestPSZeroPIDExits1(t *testing.T) {
	_, stderr, code := runScript(t, "ps -p 0")
	if code != 1 {
		t.Errorf("expected exit code 1 for PID 0, got %d", code)
	}
	if !strings.Contains(stderr, "invalid PID") {
		t.Errorf("expected 'invalid PID' in stderr, got: %s", stderr)
	}
}

// TestPSEmptyPIDListExits1 ensures an all-comma PID list is rejected.
func TestPSEmptyPIDListExits1(t *testing.T) {
	_, _, code := runScript(t, "ps -p ','")
	if code != 1 {
		t.Errorf("expected exit code 1 for empty PID list, got %d", code)
	}
}

// TestPSDoubleCommaExits1 ensures consecutive commas in PID list are rejected.
func TestPSDoubleCommaExits1(t *testing.T) {
	_, stderr, code := runScript(t, "ps -p '1,,2'")
	if code != 1 {
		t.Errorf("expected exit code 1 for '1,,2', got %d", code)
	}
	if !strings.Contains(stderr, "invalid PID") {
		t.Errorf("expected 'invalid PID' in stderr, got: %s", stderr)
	}
}

// TestPSEmptyStringPIDExits1 ensures an explicit empty -p value is rejected
// rather than falling through to the default session view.
func TestPSEmptyStringPIDExits1(t *testing.T) {
	_, _, code := runScript(t, "ps -p ''")
	if code != 1 {
		t.Errorf("expected exit code 1 for empty -p value, got %d", code)
	}
}

// TestPSAllWithInvalidPIDExits1 ensures -e -p with a malformed PID is rejected.
// GNU ps validates the -p argument even when -e is also set.
func TestPSAllWithInvalidPIDExits1(t *testing.T) {
	_, _, code := runScript(t, "ps -e -p notapid")
	if code != 1 {
		t.Errorf("expected exit code 1 for ps -e -p notapid, got %d", code)
	}
}

// TestPSAllWithMissingPIDExitsZero ensures -e takes priority over -p so that
// ps -e -p <missing> still shows all processes and exits 0 (additive selection).
func TestPSAllWithMissingPIDExitsZero(t *testing.T) {
	stdout, stderr, code := runScript(t, "ps -e -p 2147483647")
	if code != 0 {
		t.Fatalf("ps -e -p <missing> exited %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "PID") {
		t.Errorf("expected PID column header, got:\n%s", stdout)
	}
}

// TestPSDuplicatePIDsDeduped ensures duplicate PIDs in -p list appear once.
func TestPSDuplicatePIDsDeduped(t *testing.T) {
	selfPID := strconv.Itoa(os.Getpid())
	stdout, stderr, code := runScript(t, "ps -p "+selfPID+","+selfPID)
	if code != 0 {
		t.Fatalf("ps -p <dup> exited %d; stderr: %s", code, stderr)
	}
	count := strings.Count(stdout, selfPID)
	if count != 1 {
		t.Errorf("expected PID %s to appear once, got %d occurrences:\n%s", selfPID, count, stdout)
	}
}

// TestPSMissingPIDExits1 ensures ps -p with a non-existent PID exits with code 1.
func TestPSMissingPIDExits1(t *testing.T) {
	// PID 2147483647 (max int32) is extremely unlikely to exist.
	_, _, code := runScript(t, "ps -p 2147483647")
	if code != 1 {
		t.Errorf("expected exit code 1 for non-existent PID, got %d", code)
	}
}

// TestPSNonNumericPositionalArgExits1 ensures non-numeric positional args are rejected.
func TestPSNonNumericPositionalArgExits1(t *testing.T) {
	_, stderr, code := runScript(t, "ps foo")
	if code != 1 {
		t.Errorf("expected exit code 1 for non-numeric positional arg, got %d", code)
	}
	if !strings.Contains(stderr, "ps:") {
		t.Errorf("expected error in stderr, got: %s", stderr)
	}
}

// TestPSBlankSeparatedPIDs ensures ps -p 123 456 works (blank-separated list).
func TestPSBlankSeparatedPIDs(t *testing.T) {
	selfPID := os.Getpid()
	stdout, stderr, code := runScript(t, "ps -p 1 "+strconv.Itoa(selfPID))
	if code != 0 {
		t.Fatalf("ps -p 1 %d exited %d; stderr: %s", selfPID, code, stderr)
	}
	if !strings.Contains(stdout, "PID") {
		t.Errorf("expected PID column header, got:\n%s", stdout)
	}
}

// TestPSHelp ensures --help prints usage and exits 0.
func TestPSHelp(t *testing.T) {
	stdout, _, code := runScript(t, "ps --help")
	if code != 0 {
		t.Errorf("ps --help exited %d", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("expected Usage: in output, got:\n%s", stdout)
	}
}

func TestPSCustomFormat(t *testing.T) {
	selfPID := strconv.Itoa(os.Getpid())
	stdout, stderr, code := runScript(t, "ps -p "+selfPID+" -o pid,ppid,uid,state,tty,comm")
	if code != 0 {
		t.Fatalf("custom ps format exited %d; stderr: %s", code, stderr)
	}
	header := strings.SplitN(stdout, "\n", 2)[0]
	for _, expected := range []string{"PID", "PPID", "UID", "S", "TTY", "COMMAND"} {
		if !strings.Contains(header, expected) {
			t.Errorf("expected %q in custom header, got %q", expected, header)
		}
	}
	if !strings.Contains(stdout, selfPID) {
		t.Errorf("expected PID %s in output, got:\n%s", selfPID, stdout)
	}
}

func TestPSUnsafeFormatFieldsRejected(t *testing.T) {
	for _, field := range []string{"args", "cmd", "command", "environ", "exe"} {
		t.Run(field, func(t *testing.T) {
			_, stderr, code := runScript(t, "ps -o "+field)
			if code != 1 {
				t.Fatalf("expected unsafe field %q to exit 1, got %d", field, code)
			}
			if !strings.Contains(stderr, "unknown format specifier") {
				t.Fatalf("unexpected stderr for %q: %q", field, stderr)
			}
		})
	}
}

// TestPSUnknownFlag ensures an unknown flag exits with code 1.
func TestPSUnknownFlag(t *testing.T) {
	_, _, code := runScript(t, "ps --unknownflag")
	if code != 1 {
		t.Errorf("expected exit code 1 for unknown flag, got %d", code)
	}
}

func TestPSPentestUnsupportedDisclosureFlagsRejected(t *testing.T) {
	tests := []string{
		"ps --forest",
		"ps --cols 200",
		"ps aux",
	}
	for _, script := range tests {
		t.Run(script, func(t *testing.T) {
			_, stderr, code := runScript(t, script)
			if code != 1 {
				t.Fatalf("expected %q to exit 1, got %d", script, code)
			}
			if !strings.Contains(stderr, "ps:") {
				t.Fatalf("expected ps error for %q, got stderr=%q", script, stderr)
			}
		})
	}
}

func TestPSPentestLargePIDListDoesNotHang(t *testing.T) {
	var b strings.Builder
	b.WriteString("ps -p ")
	for pid := 1_000_000; pid < 1_005_000; pid++ {
		if pid > 1_000_000 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(pid))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(b.String()), "")
	if err != nil {
		t.Fatal(err)
	}
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption))
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	runErr := runner.Run(ctx, prog)
	if ctx.Err() != nil {
		t.Fatalf("large PID list did not finish before context deadline: %v", ctx.Err())
	}
	var es interp.ExitStatus
	if !errors.As(runErr, &es) || int(es) != 1 {
		t.Fatalf("expected missing PID list to exit 1, got err=%v stdout=%q stderr=%q", runErr, outBuf.String(), errBuf.String())
	}
}

// TestPSNeverReadsStdin ensures ps does not block waiting for stdin.
// We provide no stdin and verify ps completes immediately.
func TestPSNeverReadsStdin(t *testing.T) {
	// ps should complete without reading stdin at all.
	_, _, code := runScript(t, "ps --help")
	if code != 0 {
		t.Errorf("ps --help should exit 0, got %d", code)
	}
}

// TestPSOutputHasPIDColumn verifies the PID column contains numeric values.
func TestPSOutputHasPIDColumn(t *testing.T) {
	stdout, _, code := runScript(t, "ps -e")
	if code != 0 {
		t.Fatalf("ps -e failed")
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least header + 1 process line, got %d lines", len(lines))
	}
	// Skip header line; check that at least one data line has a numeric first field.
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			t.Errorf("expected numeric PID in first column, got %q", fields[0])
		}
		break
	}
}
