// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seclog holds seclog related files
package seclog

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	wildcard = "*"
	depth    = 4
)

// used to extract package.struct.func from the caller
var re = regexp.MustCompile(`[^\.]*\/([^\.]*)\.\(?\*?([^\.\)]*)\)?\.(.*)$`)

// TagStringer implements fmt.Stringer
type TagStringer struct {
	tag string
}

// String implements fmt.Stringer
func (t *TagStringer) String() string {
	return t.tag
}

// PatternLogger is a wrapper for the agent logger that add a level of filtering to trace log level
type PatternLogger struct {
	sync.RWMutex

	tags     []string
	patterns []string
	nodes    [][]string
}

func (l *PatternLogger) match(els []string) bool {
LOOP:
	for _, pattern := range l.nodes {
		for i, node := range pattern {
			if node == wildcard {
				continue
			}

			if i >= len(els) {
				break
			}

			if node != els[i] {
				continue LOOP
			}
		}

		return true
	}

	return false
}

func (l *PatternLogger) trace(tag fmt.Stringer, format string, params ...interface{}) {
	// check first tags
	stag := tag.String()
	if len(stag) != 0 {

		l.RLock()
		if slices.Contains(l.tags, stag) {
			l.RUnlock()
			log.TraceStackDepth(depth, fmt.Sprintf(format, params...))

			return
		}
		l.RUnlock()
	}

	pc, _, _, ok := runtime.Caller(3)
	if !ok {
		return
	}
	details := runtime.FuncForPC(pc)
	if details == nil {
		return
	}

	els := re.FindStringSubmatch(details.Name())
	if len(els) != 4 {
		return
	}

	l.RLock()
	active := l.match(els[1:])
	l.RUnlock()

	if active {
		log.TraceStackDepth(depth, fmt.Sprintf(format, params...))
	}
}

// Trace is used to print a trace level log
func (l *PatternLogger) Trace(v interface{}) {
	l.TraceTag(&TagStringer{}, v)
}

// TraceTag is used to print a trace level log for the given tag
func (l *PatternLogger) TraceTag(tag fmt.Stringer, v interface{}) {
	l.TraceTagf(tag, "%v", v)
}

// TraceTagf is used to print a trace level log
func (l *PatternLogger) TraceTagf(tag fmt.Stringer, format string, params ...interface{}) {
	if !l.IsTracing() {
		return
	}

	l.trace(tag, format, params...)
}

// Tracef is used to print a trace level log
func (l *PatternLogger) Tracef(format string, params ...interface{}) {
	if !l.IsTracing() {
		return
	}

	l.trace(&TagStringer{}, format, params...)
}

// IsTracing is used to check if TraceF would actually log
func (l *PatternLogger) IsTracing() bool {
	if logLevel, err := log.GetLogLevel(); err != nil || logLevel != log.TraceLvl {
		return false
	}
	return true
}

// IsDebugging returns true if the debug level is enabled
func (l *PatternLogger) IsDebugging() bool {
	if logLevel, err := log.GetLogLevel(); err != nil || logLevel != log.DebugLvl {
		return false
	}
	return true
}

// Debugf is used to print a trace level log
func (l *PatternLogger) Debugf(format string, params ...interface{}) {
	log.DebugStackDepth(depth-1, fmt.Sprintf(format, params...))
}

// Errorf is used to print an error
func (l *PatternLogger) Errorf(format string, params ...interface{}) {
	_ = log.ErrorStackDepth(depth-1, fmt.Sprintf(format, params...))
}

// Warnf is used to print a warn
func (l *PatternLogger) Warnf(format string, params ...interface{}) {
	log.WarnStackDepth(depth-1, fmt.Sprintf(format, params...))
}

// Infof is used to print an error
func (l *PatternLogger) Infof(format string, params ...interface{}) {
	log.InfoStackDepth(depth-1, fmt.Sprintf(format, params...))
}

// AddTags add new tags
func (l *PatternLogger) AddTags(tags ...string) []string {
	l.Lock()
	prev := l.tags
	l.tags = append(l.tags, tags...)
	l.Unlock()

	return prev
}

// AddPatterns add new patterns
func (l *PatternLogger) AddPatterns(patterns ...string) []string {
	l.Lock()
	prev := l.patterns

	for _, pattern := range patterns {
		l.patterns = append(l.patterns, pattern)
		l.nodes = append(l.nodes, strings.Split(pattern, "."))
	}
	l.Unlock()

	return prev
}

// SetPatterns set patterns
func (l *PatternLogger) SetPatterns(patterns ...string) []string {
	l.Lock()
	prev := l.patterns

	l.nodes = [][]string{}
	for _, pattern := range patterns {
		l.nodes = append(l.nodes, strings.Split(pattern, "."))
	}
	l.Unlock()

	return prev
}

// SetTags set tags
func (l *PatternLogger) SetTags(tags ...string) []string {
	l.Lock()
	prev := l.tags
	l.tags = tags
	l.Unlock()

	return prev
}

// DefaultLogger default logger of this package
var DefaultLogger *PatternLogger

// Debugf is used to print a trace level log
func Debugf(format string, params ...interface{}) {
	DefaultLogger.Debugf(format, params...)
}

// Errorf is used to print an error
func Errorf(format string, params ...interface{}) {
	DefaultLogger.Errorf(format, params...)
}

// Warnf is used to print a warn
func Warnf(format string, params ...interface{}) {
	DefaultLogger.Warnf(format, params...)
}

// Infof is used to print an error
func Infof(format string, params ...interface{}) {
	DefaultLogger.Infof(format, params...)
}

// Tracef is used to print an trace
func Tracef(format string, params ...interface{}) {
	DefaultLogger.Tracef(format, params...)
}

// TraceTagf is used to print an trace
func TraceTagf(tag fmt.Stringer, format string, params ...interface{}) {
	DefaultLogger.TraceTagf(tag, format, params...)
}

// Trace is used to print an trace
func Trace(v interface{}) {
	DefaultLogger.Trace(v)
}

// TraceTag is used to print an trace
func TraceTag(tag fmt.Stringer, v interface{}) {
	DefaultLogger.TraceTag(tag, v)
}

// AddTags add tags
func AddTags(tags ...string) []string {
	return DefaultLogger.AddTags(tags...)
}

// AddPatterns add patterns
func AddPatterns(patterns ...string) []string {
	return DefaultLogger.AddPatterns(patterns...)
}

// SetTags set tags
func SetTags(tags ...string) []string {
	return DefaultLogger.SetTags(tags...)
}

// SetPatterns set patterns
func SetPatterns(patterns ...string) []string {
	return DefaultLogger.SetPatterns(patterns...)
}

func init() {
	DefaultLogger = &PatternLogger{}
}

// phaseProfilingEnabled reports whether shutdown/startup phase profiling is enabled. It is
// driven by DD_CWS_PHASE_PROFILING so that it stays a no-op unless explicitly asked for.
var phaseProfilingEnabled = os.Getenv("DD_CWS_PHASE_PROFILING") != ""

// PhaseProfilingEnabled reports whether phase profiling is enabled
func PhaseProfilingEnabled() bool {
	return phaseProfilingEnabled
}

// StartPhase starts a named timing phase and returns the function that ends it and reports the
// elapsed time. Reporting happens at warn level so that it shows up with the default log level
// of the functional test suite. When phase profiling is disabled both calls are no-ops.
//
// Usage:
//
//	defer seclog.StartPhase("EBPFProbe.Close")()
func StartPhase(name string) func() {
	if !phaseProfilingEnabled {
		return func() {}
	}

	start := time.Now()
	return func() {
		Warnf("[phase] %s took %s", name, time.Since(start))
	}
}

// userHZ is the fixed unit of the utime/stime fields of /proc/<pid>/stat. Unlike CONFIG_HZ it is
// part of the kernel ABI and is always 100, so there is no need to call sysconf(_SC_CLK_TCK).
const userHZ = 100

// phaseSampleInterval is how often StartPhaseProfile looks at the thread states of the process.
const phaseSampleInterval = 200 * time.Millisecond

// StartPhaseProfile times a named phase like StartPhase, and additionally answers the question
// "was this phase burning CPU or blocked in the kernel?" — which is what distinguishes a cost paid
// scanning kernel structures under a mutex from one paid waiting on RCU grace periods.
//
// It reports a single line of flat key=value pairs so it can be grepped locally, parsed by a
// Datadog log pipeline with the built-in key/value parser, or promoted to junit properties:
//
//		[phase-profile] name=... wall_ms=... cpu_ms=... cpu_ratio=... threads_max=... state_D_max=... wchan=sym:count,...
//
//	  - cpu_ratio near 1 means a thread saturated a core for the whole phase: the cost is CPU-bound.
//	  - cpu_ratio near 0 means every thread was parked: the cost is a wait, and wchan names it.
//
// The first call also reports the kernel facts needed to compare one platform against another, see
// logPhaseEnvironment. Everything is a no-op unless DD_CWS_PHASE_PROFILING is set.
func StartPhaseProfile(name string) func() {
	if !phaseProfilingEnabled {
		return func() {}
	}

	phaseEnvironmentOnce.Do(logPhaseEnvironment)

	start := time.Now()
	startCPU, cpuOK := processCPUTime()

	var (
		threadsMax int
		stateMax   = map[byte]int{}
		wchans     = map[string]int{}
		done       = make(chan struct{})
		stopped    = make(chan struct{})
	)

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(phaseSampleInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				threads, states, blocked := sampleThreads()
				if threads > threadsMax {
					threadsMax = threads
				}
				for state, n := range states {
					if n > stateMax[state] {
						stateMax[state] = n
					}
				}
				for _, symbol := range blocked {
					wchans[symbol]++
				}
			}
		}
	}()

	return func() {
		close(done)
		<-stopped

		wall := time.Since(start)
		fields := []string{
			"name=" + name,
			fmt.Sprintf("wall_ms=%d", wall.Milliseconds()),
		}

		if endCPU, ok := processCPUTime(); ok && cpuOK {
			cpu := endCPU - startCPU
			fields = append(fields, fmt.Sprintf("cpu_ms=%d", cpu.Milliseconds()))
			if wall > 0 {
				fields = append(fields, fmt.Sprintf("cpu_ratio=%.3f", cpu.Seconds()/wall.Seconds()))
			}
		}

		fields = append(fields, fmt.Sprintf("threads_max=%d", threadsMax))
		// R is runnable, D is uninterruptible sleep. A phase dominated by D with cpu_ratio near
		// zero is blocked; one with a single R and cpu_ratio near one is scanning under a lock.
		for _, state := range []byte{'R', 'D', 'S'} {
			if n := stateMax[state]; n > 0 {
				fields = append(fields, fmt.Sprintf("state_%c_max=%d", state, n))
			}
		}
		if top := topCounts(wchans, 5); top != "" {
			fields = append(fields, "wchan="+top)
		}

		Warnf("[phase-profile] %s", strings.Join(fields, " "))
	}
}

// processCPUTime returns the CPU time consumed by every thread of this process so far.
func processCPUTime() (time.Duration, bool) {
	fields, ok := readProcStat("/proc/self/stat")
	if !ok {
		return 0, false
	}

	// Fields are numbered from 3 (state) since the leading pid and comm are stripped, so utime
	// (14) and stime (15) sit at indexes 11 and 12.
	if len(fields) < 13 {
		return 0, false
	}
	utime, err1 := strconv.ParseInt(fields[11], 10, 64)
	stime, err2 := strconv.ParseInt(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}

	return time.Duration(utime+stime) * time.Second / userHZ, true
}

// sampleThreads returns the number of threads of this process, how many are in each scheduler
// state, and the kernel symbol each non-running thread is parked in.
func sampleThreads() (int, map[byte]int, []string) {
	entries, err := os.ReadDir("/proc/self/task")
	if err != nil {
		return 0, nil, nil
	}

	states := map[byte]int{}
	var blocked []string
	for _, entry := range entries {
		fields, ok := readProcStat("/proc/self/task/" + entry.Name() + "/stat")
		if !ok || len(fields) == 0 || len(fields[0]) == 0 {
			continue
		}

		state := fields[0][0]
		states[state]++
		if state == 'R' {
			continue
		}

		// wchan is a single kernel symbol, and only readable with enough privileges. Threads
		// parked in the Go runtime report an empty value or "0", which is noise here.
		if raw, err := os.ReadFile("/proc/self/task/" + entry.Name() + "/wchan"); err == nil {
			if symbol := strings.TrimSpace(string(raw)); symbol != "" && symbol != "0" {
				blocked = append(blocked, symbol)
			}
		}
	}

	return len(entries), states, blocked
}

// readProcStat reads a /proc stat file and returns its fields starting at the state field. The
// comm field can contain spaces and parentheses, so everything up to the last ')' is dropped.
func readProcStat(path string) ([]string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	end := strings.LastIndexByte(string(raw), ')')
	if end < 0 || end+2 >= len(raw) {
		return nil, false
	}

	return strings.Fields(string(raw[end+2:])), true
}

// topCounts renders the n highest counts as "key:count,key:count", highest first.
func topCounts(counts map[string]int, n int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})

	if len(keys) > n {
		keys = keys[:n]
	}

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, counts[key]))
	}
	return strings.Join(parts, ",")
}

var phaseEnvironmentOnce sync.Once

// logPhaseEnvironment reports the kernel facts that make phase timings comparable across
// platforms, as flat key=value pairs. available_filter_functions is the number of functions ftrace
// can instrument, which bounds the work each probe detach has to do, so it is the first thing to
// compare between a platform where teardown is cheap and one where it is not.
func logPhaseEnvironment() {
	fields := []string{
		fmt.Sprintf("ncpu=%d", runtime.NumCPU()),
	}

	for _, item := range []struct {
		key  string
		path string
	}{
		{"kernel", "/proc/sys/kernel/osrelease"},
		{"rcu_expedited", "/sys/kernel/rcu_expedited"},
		{"rcu_normal", "/sys/kernel/rcu_normal"},
		{"ftrace_enabled", "/proc/sys/kernel/ftrace_enabled"},
		{"kprobes_optimization", "/proc/sys/debug/kprobes-optimization"},
	} {
		if raw, err := os.ReadFile(item.path); err == nil {
			fields = append(fields, fmt.Sprintf("%s=%s", item.key, strings.TrimSpace(string(raw))))
		} else {
			fields = append(fields, item.key+"=absent")
		}
	}

	// tracefs moved from debugfs, so try both mount points.
	for _, item := range []struct {
		key  string
		name string
	}{
		{"available_filter_functions", "available_filter_functions"},
		{"enabled_functions", "enabled_functions"},
	} {
		count := -1
		for _, dir := range []string{"/sys/kernel/tracing/", "/sys/kernel/debug/tracing/"} {
			if n, ok := countLines(dir + item.name); ok {
				count = n
				break
			}
		}
		fields = append(fields, fmt.Sprintf("%s=%d", item.key, count))
	}

	if raw, err := os.ReadFile("/proc/cmdline"); err == nil {
		fields = append(fields, fmt.Sprintf("cmdline=%q", strings.TrimSpace(string(raw))))
	}

	Warnf("[phase-env] %s", strings.Join(fields, " "))
}

// countLines counts the lines of a file without holding all of it in memory, since
// available_filter_functions lists every instrumentable kernel function.
func countLines(path string) (int, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()

	var count int
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, false
	}

	return count, true
}
