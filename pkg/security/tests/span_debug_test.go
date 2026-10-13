// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

package tests

// TEMPORARY: diagnostics for the TestOTelSpan resolution timeouts. Everything
// is printed with the span-dbg marker, next to the agent's own span-dbg
// warnings and the eBPF span-dbg trace_pipe lines.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// syncBuffer lets the timeout dump read the tester's output while exec's
// copying goroutine is still writing it.
type syncBuffer struct {
	lock sync.Mutex
	buf  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Bytes() []byte {
	b.lock.Lock()
	defer b.lock.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

// spanTesterPids finds the testers by the continue file runSpanTester put in
// their environment: in docker mode the started command is the docker client,
// not the tester.
func spanTesterPids(continueFile string) []uint32 {
	needle := []byte(spanTesterContinueFileEnv + "=" + continueFile)
	entries, err := os.ReadDir(kernel.ProcFSRoot())
	if err != nil {
		return nil
	}
	var pids []uint32
	for _, entry := range entries {
		pid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}
		environ, err := os.ReadFile(kernel.HostProc(entry.Name(), "environ"))
		if err != nil {
			continue
		}
		for _, kv := range bytes.Split(environ, []byte{0}) {
			if bytes.Equal(kv, needle) {
				pids = append(pids, uint32(pid))
				break
			}
		}
	}
	return pids
}

func readProcLines(pid string, name string, keep func(string) bool) string {
	data, err := os.ReadFile(kernel.HostProc(pid, name))
	if err != nil {
		return "<" + err.Error() + ">"
	}
	var kept []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if keep == nil || keep(line) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, " | ")
}

func describeSpanTesterProcess(pid uint32) string {
	pidStr := strconv.FormatUint(uint64(pid), 10)
	var b strings.Builder
	fmt.Fprintf(&b, "  pid %d\n", pid)
	if cmdline, err := os.ReadFile(kernel.HostProc(pidStr, "cmdline")); err == nil {
		fmt.Fprintf(&b, "    cmdline: %q\n", strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00"))
	}
	if exe, err := os.Readlink(kernel.HostProc(pidStr, "exe")); err == nil {
		fmt.Fprintf(&b, "    exe: %s\n", exe)
	}
	fmt.Fprintf(&b, "    status: %s\n", readProcLines(pidStr, "status", func(line string) bool {
		for _, prefix := range []string{"Name:", "State:", "Tgid:", "Pid:", "PPid:", "NSpid:", "Threads:"} {
			if strings.HasPrefix(line, prefix) {
				return true
			}
		}
		return false
	}))
	if tasks, err := os.ReadDir(kernel.HostProc(pidStr, "task")); err == nil {
		for _, task := range tasks {
			taskDir := filepath.Join("task", task.Name())
			fmt.Fprintf(&b, "    task %s: stat=%s wchan=%s syscall=%s\n", task.Name(),
				readProcLines(pidStr, filepath.Join(taskDir, "stat"), nil),
				readProcLines(pidStr, filepath.Join(taskDir, "wchan"), nil),
				readProcLines(pidStr, filepath.Join(taskDir, "syscall"), nil))
		}
	}
	fmt.Fprintf(&b, "    maps (OTEL/memfd/anon): %s\n", readProcLines(pidStr, "maps", func(line string) bool {
		return strings.Contains(line, "OTEL") || strings.Contains(line, "memfd") || strings.Contains(line, "anon")
	}))
	return b.String()
}

// spanCounters prints every counter that says where an OTel process context
// trigger went: kernel sent/lost, user space read, and each span context step.
func (tm *testModule) spanCounters() string {
	tm.eventMonitor.SendStats()

	var keys []string
	values := map[string]int64{}
	add := func(prefix string, filter func(string) bool) {
		for k, v := range tm.statsdClient.GetByPrefix(prefix) {
			if filter == nil || filter(k) {
				keys = append(keys, prefix+k)
				values[prefix+k] = v
			}
		}
	}
	otelEvent := func(k string) bool { return strings.Contains(k, "event_type:otel_process_ctx") }
	add("datadog.runtime_security.span_context", nil)
	add(metrics.MetricPerfBufferEventsWrite, otelEvent)
	add(metrics.MetricPerfBufferEventsRead, otelEvent)
	add(metrics.MetricPerfBufferLostWrite, otelEvent)
	add(metrics.MetricPerfBufferLostRead, nil)
	add(metrics.MetricPerfBufferSortingError, nil)
	add("datadog.runtime_security.syscalls_map.event_inflight:event_type:prctl", nil)
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "  %s = %d\n", k, values[k])
	}
	return b.String()
}

// spanTesterTimeoutDump describes the testers and the agent's view of them,
// then resolves them directly: a direct resolution that succeeds means the
// trigger was lost on its way to the resolver, not that resolving is broken.
func (tm *testModule) spanTesterTimeoutDump(continueFile string, started time.Time, out []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nspan-dbg: timeout dump, %s after the tester started\n", time.Since(started))
	fmt.Fprintf(&b, "span-dbg: tester output so far:\n%s\n", out)

	pids := spanTesterPids(continueFile)
	fmt.Fprintf(&b, "span-dbg: tester pids (by continue file env): %v\n", pids)
	for _, pid := range pids {
		b.WriteString(describeSpanTesterProcess(pid))
	}

	fmt.Fprintf(&b, "span-dbg: counters before the direct resolution:\n%s", tm.spanCounters())

	ebpfProbe, ok := tm.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return b.String()
	}
	// Before the direct resolution: it must describe the stream as the timeout found it.
	fmt.Fprintf(&b, "span-dbg: event stream: %s\n", ebpfProbe.DebugEventStreamState())
	fmt.Fprintf(&b, "span-dbg: goroutines inside handleEvent:\n%s", handleEventStacks())
	resolver := ebpfProbe.Resolvers.ProcessResolver
	for _, pid := range pids {
		fmt.Fprintf(&b, "span-dbg: resolver state of %d before: %s\n", pid, resolver.DebugOTelProcessContextState(pid))
		resolver.DebugResolveOTelProcessContextNow(pid)
		fmt.Fprintf(&b, "span-dbg: resolver state of %d after a direct resolution: %s\n", pid, resolver.DebugOTelProcessContextState(pid))
	}
	fmt.Fprintf(&b, "span-dbg: counters after the direct resolution:\n%s", tm.spanCounters())
	return b.String()
}

// handleEventStacks returns the stacks of the goroutines currently running the
// probe's event handler: where a blocked handler waits.
func handleEventStacks() string {
	buf := make([]byte, 16<<20)
	buf = buf[:runtime.Stack(buf, true)]
	var b strings.Builder
	for _, stack := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(stack, "EBPFProbe).handleEvent") {
			b.WriteString(stack)
			b.WriteString("\n\n")
		}
	}
	if b.Len() == 0 {
		return "  <none>\n"
	}
	return b.String()
}
