// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build syscalltesters

// Package main holds the Go span-context syscall tester.
//
// It drives the agent's Go pprof-label span reader: it creates a tracer-info
// memfd, sets span labels either directly or through dd-trace-go, and then
// triggers the syscall a CWS rule watches (open, execve or fork+execve).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/sys/unix"
)

var (
	goSpanTest              bool
	goSpanExecTest          bool
	goSpanNoLabelsTest      bool
	goSpanNoLabelsExecTest  bool
	goSpanForkExecTest      bool
	goSpanSpanID            string
	goSpanLocalRootSpanID   string
	goSpanFilePath          string
	goSpanExecTarget        string
	ddtraceSpanTest         bool
	ddtraceSpanExecTest     bool
	ddtraceNoSpanTest       bool
	ddtraceNoSpanExecTest   bool
	ddtraceSpanForkExecTest bool
	ddtraceSpanFilePath     string
	ddtraceSpanExecTarget   string
)

// setupGoTracerMemfd creates and seals the tracer-info memfd that drives the
// agent's resolveGoLabels flow. Shared by all Go-span test modes (with/without
// pprof labels, open or exec).
func setupGoTracerMemfd(serviceName, memfdName string) (int, error) {
	type TracerMeta struct {
		SchemaVersion  uint8  `msgpack:"schema_version"`
		TracerLanguage string `msgpack:"tracer_language"`
		TracerVersion  string `msgpack:"tracer_version"`
		Hostname       string `msgpack:"hostname"`
		ServiceName    string `msgpack:"service_name"`
	}
	data, err := msgpack.Marshal(&TracerMeta{
		SchemaVersion:  2,
		TracerLanguage: "go",
		TracerVersion:  "0.0.1-test",
		Hostname:       "test",
		ServiceName:    serviceName,
	})
	if err != nil {
		return -1, fmt.Errorf("msgpack marshal: %w", err)
	}

	fd, err := unix.MemfdCreate(memfdName, unix.MFD_ALLOW_SEALING)
	if err != nil {
		return -1, fmt.Errorf("memfd_create: %w", err)
	}
	if _, err := unix.Write(fd, data); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("memfd write: %w", err)
	}
	const fAddSeals = 1033 // F_ADD_SEALS
	const fSealWrite = 0x0008
	const fSealShrink = 0x0002
	const fSealGrow = 0x0004
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), fAddSeals, fSealWrite|fSealShrink|fSealGrow); errno != 0 {
		unix.Close(fd)
		return -1, fmt.Errorf("memfd seal: %w", errno)
	}

	// Wait for the agent to process the memfd seal event and populate the
	// go_labels_procs BPF map.
	time.Sleep(500 * time.Millisecond)
	return fd, nil
}

// triggerOpen creates filePath, closes, and unlinks — the CWS rule fires on
// the open hook before unlink.
func triggerOpen(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	f.Close()
	os.Remove(filePath)
	return nil
}

// triggerExec execs the target binary with `--reference /etc/passwd <filePath>`
// so the existing exec rule (exec.args_flags == "reference") matches. The
// current process image is replaced; the eBPF probe captures the span context
// at prepare_binprm, before the replacement.
func triggerExec(target, filePath string) error {
	if target == "" {
		return fmt.Errorf("exec target is required")
	}
	argv := []string{target, "--reference", "/etc/passwd", filePath}
	return syscall.Exec(target, argv, os.Environ())
}

// triggerForkExec fork+execs the target binary via os/exec.Cmd.Run — i.e. the
// child runs in a brand-new tgid. This is the canonical "Go program shells out
// to a subprocess" pattern (the one os/exec exposes). The eBPF fork hook hands
// go_labels_procs down to the child's tgid, so the read in the child's exec
// hook still resolves the parent's labels out of the copy-on-write goroutine.
func triggerForkExec(target, filePath string) error {
	if target == "" {
		return fmt.Errorf("exec target is required")
	}
	cmd := exec.Command(target, "--reference", "/etc/passwd", filePath)
	return cmd.Run()
}

// RunGoSpanTest creates a tracer-info memfd (triggering Go label offset
// resolution), optionally sets pprof labels (skipped for negative-path
// scenarios), and then triggers the syscall the rule watches. The trigger is:
//   - open of filePath when execTarget == ""
//   - in-process execve of execTarget when forkExec == false
//   - fork+execve (os/exec.Cmd.Run) of execTarget when forkExec == true
//
// The fork+execve mode is used by the "fork_exec_propagates_to_child"
// regression test.
func RunGoSpanTest(spanID, localRootSpanID, filePath, execTarget string, setLabels, forkExec bool) error {
	fd, err := setupGoTracerMemfd("go-span-test", "datadog-tracer-info-gotest01")
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	if setLabels {
		// Set pprof labels exactly like dd-trace-go does.
		// Keys: "span id" and "local root span id", values: decimal strings.
		labels := pprof.Labels("span id", spanID, "local root span id", localRootSpanID)
		ctx := pprof.WithLabels(context.Background(), labels)
		pprof.SetGoroutineLabels(ctx)
		defer pprof.SetGoroutineLabels(context.Background())
	}

	if forkExec {
		return triggerForkExec(execTarget, filePath)
	}
	if execTarget != "" {
		return triggerExec(execTarget, filePath)
	}
	return triggerOpen(filePath)
}

// RunDDTraceSpanTest uses dd-trace-go to create a real span (which sets pprof
// labels via the profiler code-hotspots integration) and then triggers the
// syscall the rule watches. Trigger selection mirrors RunGoSpanTest:
//   - open of filePath when execTarget == ""
//   - in-process execve when forkExec == false
//   - fork+execve when forkExec == true
//
// If startSpan is false, the tracer is started but no active span is created —
// the eBPF reader should yield an empty span context (negative path).
func RunDDTraceSpanTest(filePath, execTarget string, startSpan, forkExec bool) error {
	fd, err := setupGoTracerMemfd("ddtrace-test", "datadog-tracer-info-ddtrace0")
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	// Start dd-trace-go with:
	// - WithTestDefaults: uses a dummy transport (no real agent needed)
	// - WithProfilerCodeHotspots: enables "span id" and "local root span id" pprof labels
	// - WithService: set a service name
	tracer.Start(
		tracer.WithTestDefaults(nil),
		tracer.WithProfilerCodeHotspots(true),
		tracer.WithService("ddtrace-test"),
		tracer.WithLogStartup(false),
	)
	defer tracer.Stop()

	var span *tracer.Span
	if startSpan {
		// dd-trace-go will automatically set pprof labels "span id" and
		// "local root span id" on the current goroutine.
		var ctx context.Context
		span, ctx = tracer.StartSpanFromContext(context.Background(), "test.operation")
		_ = ctx

		// Print the span ID and local root span ID so the test can parse
		// and verify them.
		spanID := span.Context().SpanID()
		localRootSpanID := span.Root().Context().SpanID()
		fmt.Printf("ddtrace_span_id=%d\n", spanID)
		fmt.Printf("ddtrace_local_root_span_id=%d\n", localRootSpanID)

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer span.Finish()
	}

	if forkExec {
		return triggerForkExec(execTarget, filePath)
	}
	if execTarget != "" {
		return triggerExec(execTarget, filePath)
	}
	return triggerOpen(filePath)
}

func main() {
	flag.BoolVar(&goSpanTest, "go-span-test", false, "when set, runs the Go pprof labels span test (open, labels set)")
	flag.BoolVar(&goSpanExecTest, "go-span-exec-test", false, "when set, runs the Go pprof labels span exec test (exec, labels set)")
	flag.BoolVar(&goSpanNoLabelsTest, "go-span-no-labels-test", false, "when set, runs the Go span open test WITHOUT setting pprof labels (negative path)")
	flag.BoolVar(&goSpanNoLabelsExecTest, "go-span-no-labels-exec-test", false, "when set, runs the Go span exec test WITHOUT setting pprof labels (negative path)")
	flag.BoolVar(&goSpanForkExecTest, "go-span-fork-exec-test", false, "when set, sets pprof labels then fork+execs the target via os/exec (parent's labels are not inherited by the child's new tgid — pins the current fork+exec gap)")
	flag.StringVar(&goSpanSpanID, "go-span-span-id", "", "span ID for the Go span test (decimal string)")
	flag.StringVar(&goSpanLocalRootSpanID, "go-span-local-root-span-id", "", "local root span ID for the Go span test (decimal string)")
	flag.StringVar(&goSpanFilePath, "go-span-file-path", "", "file path to open / touch for the Go span test")
	flag.StringVar(&goSpanExecTarget, "go-span-exec-target", "", "executable to exec for the Go span exec test (e.g. /usr/bin/touch)")
	flag.BoolVar(&ddtraceSpanTest, "ddtrace-span-test", false, "when set, runs the dd-trace-go span test (open, active span)")
	flag.BoolVar(&ddtraceSpanExecTest, "ddtrace-span-exec-test", false, "when set, runs the dd-trace-go span exec test (exec, active span)")
	flag.BoolVar(&ddtraceNoSpanTest, "ddtrace-no-span-test", false, "when set, runs the dd-trace-go open test WITHOUT an active span (negative path)")
	flag.BoolVar(&ddtraceNoSpanExecTest, "ddtrace-no-span-exec-test", false, "when set, runs the dd-trace-go exec test WITHOUT an active span (negative path)")
	flag.BoolVar(&ddtraceSpanForkExecTest, "ddtrace-span-fork-exec-test", false, "when set, starts an active dd-trace-go span and fork+execs the target via os/exec (pins the current fork+exec gap)")
	flag.StringVar(&ddtraceSpanFilePath, "ddtrace-span-file-path", "", "file path to open / touch for the dd-trace-go span test")
	flag.StringVar(&ddtraceSpanExecTarget, "ddtrace-span-exec-target", "", "executable to exec for the dd-trace-go span exec test (e.g. /usr/bin/touch)")

	flag.Parse()

	switch {
	case goSpanTest:
		if err := RunGoSpanTest(goSpanSpanID, goSpanLocalRootSpanID, goSpanFilePath, "", true, false); err != nil {
			panic(err)
		}
	case goSpanExecTest:
		if err := RunGoSpanTest(goSpanSpanID, goSpanLocalRootSpanID, goSpanFilePath, goSpanExecTarget, true, false); err != nil {
			panic(err)
		}
	case goSpanNoLabelsTest:
		if err := RunGoSpanTest("", "", goSpanFilePath, "", false, false); err != nil {
			panic(err)
		}
	case goSpanNoLabelsExecTest:
		if err := RunGoSpanTest("", "", goSpanFilePath, goSpanExecTarget, false, false); err != nil {
			panic(err)
		}
	case goSpanForkExecTest:
		if err := RunGoSpanTest(goSpanSpanID, goSpanLocalRootSpanID, goSpanFilePath, goSpanExecTarget, true, true); err != nil {
			panic(err)
		}
	}

	switch {
	case ddtraceSpanTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, "", true, false); err != nil {
			panic(err)
		}
	case ddtraceSpanExecTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, ddtraceSpanExecTarget, true, false); err != nil {
			panic(err)
		}
	case ddtraceNoSpanTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, "", false, false); err != nil {
			panic(err)
		}
	case ddtraceNoSpanExecTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, ddtraceSpanExecTarget, false, false); err != nil {
			panic(err)
		}
	case ddtraceSpanForkExecTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, ddtraceSpanExecTarget, true, true); err != nil {
			panic(err)
		}
	}
}
