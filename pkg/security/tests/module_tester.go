// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests
// +build functionaltests stresstests

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"text/template"
	"time"
	"unsafe"

	"runtime/pprof"

	"github.com/cihub/seelog"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/go-multierror"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	logger seelog.LoggerInterface
	//nolint:deadcode,unused
	testSuitePid uint32
)

const (
	getEventTimeout = 10 * time.Second
)

type stringSlice []string

const testConfig = `---
log_level: DEBUG
system_probe_config:
  enabled: true
  sysprobe_socket: /tmp/test-sysprobe.sock
  enable_kernel_header_download: true
  enable_runtime_compiler: true

runtime_security_config:
  enabled: true
  fim_enabled: true
  runtime_compilation:
    enabled: true
  remote_tagger: false
  custom_sensitive_words:
    - "*custom*"
  socket: /tmp/test-security-probe.sock
  flush_discarder_window: 0
  network:
    enabled: true
{{if .EnableActivityDump}}
  activity_dump:
    enabled: true
    rate_limiter: {{ .ActivityDumpRateLimiter }}
    traced_event_types:   {{range .ActivityDumpTracedEventTypes}}
    - {{.}}
    {{end}}
{{end}}
  load_controller:
    events_count_threshold: {{ .EventsCountThreshold }}
{{if .DisableFilters}}
  enable_kernel_filters: false
{{end}}
{{if .DisableApprovers}}
  enable_approvers: false
{{end}}
  erpc_dentry_resolution_enabled: {{ .ErpcDentryResolutionEnabled }}
  map_dentry_resolution_enabled: {{ .MapDentryResolutionEnabled }}
  self_test:
    enabled: false

  policies:
    dir: {{.TestPoliciesDir}}
  log_patterns:
  {{range .LogPatterns}}
    - "{{.}}"
  {{end}}
  log_tags:
  {{range .LogTags}}
    - {{.}}
  {{end}}
  envs_with_value:
  {{range .EnvsWithValue}}
    - {{.}}
  {{end}}
`

const testPolicy = `---
macros:
{{range $Macro := .Macros}}
  - id: {{$Macro.ID}}
    expression: >-
      {{$Macro.Expression}}
{{end}}

rules:
{{range $Rule := .Rules}}
  - id: {{$Rule.ID}}
    expression: >-
      {{$Rule.Expression}}
    actions:
{{- range $Action := .Actions}}
{{- if $Action.Set}}
      - set:
          name: {{$Action.Set.Name}}
		  {{- if $Action.Set.Value}}
          value: {{$Action.Set.Value}}
          {{- else if $Action.Set.Field}}
          field: {{$Action.Set.Field}}
          {{- end}}
          scope: {{$Action.Set.Scope}}
          append: {{$Action.Set.Append}}
{{- end}}
{{- end}}
{{end}}
`

var (
	testEnvironment  string
	logLevelStr      string
	logPatterns      stringSlice
	logTags          stringSlice
	logStatusMetrics bool
	withProfile      bool
)

const (
	// HostEnvironment for the Host environment
	HostEnvironment = "host"
	// DockerEnvironment for the docker container environment
	DockerEnvironment = "docker"
)

type testOpts struct {
	testDir                      string
	disableFilters               bool
	disableApprovers             bool
	enableActivityDump           bool
	activityDumpRateLimiter      int
	activityDumpTracedEventTypes []string
	disableDiscarders            bool
	eventsCountThreshold         int
	disableERPCDentryResolution  bool
	disableMapDentryResolution   bool
	envsWithValue                []string
	disableAbnormalPathCheck     bool
}

func (s *stringSlice) String() string {
	return strings.Join(*s, " ")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (to testOpts) Equal(opts testOpts) bool {
	return to.testDir == opts.testDir &&
		to.disableApprovers == opts.disableApprovers &&
		to.enableActivityDump == opts.enableActivityDump &&
		to.activityDumpRateLimiter == opts.activityDumpRateLimiter &&
		reflect.DeepEqual(to.activityDumpTracedEventTypes, opts.activityDumpTracedEventTypes) &&
		to.disableDiscarders == opts.disableDiscarders &&
		to.disableFilters == opts.disableFilters &&
		to.eventsCountThreshold == opts.eventsCountThreshold &&
		to.disableERPCDentryResolution == opts.disableERPCDentryResolution &&
		to.disableMapDentryResolution == opts.disableMapDentryResolution &&
		reflect.DeepEqual(to.envsWithValue, opts.envsWithValue)
}

type testModule struct {
	sync.RWMutex
	config        *config.Config
	opts          testOpts
	st            *simpleTest
	t             testing.TB
	module        *module.Module
	probe         *sprobe.Probe
	eventHandlers eventHandlers
	cmdWrapper    cmdWrapper
	statsdClient  *StatsdClient
	proFile       *os.File
}

var testMod *testModule

type onRuleHandler func(*sprobe.Event, *rules.Rule)
type onProbeEventHandler func(*sprobe.Event)
type onCustomSendEventHandler func(*rules.Rule, *sprobe.CustomEvent)
type onDiscarderPushedHandler func(event eval.Event, field eval.Field, eventType eval.EventType) bool

type eventHandlers struct {
	sync.RWMutex
	onRuleMatch       onRuleHandler
	onProbeEvent      onProbeEventHandler
	onCustomSendEvent onCustomSendEventHandler
	onDiscarderPushed onDiscarderPushedHandler
}

//nolint:deadcode,unused
func getInode(tb testing.TB, path string) uint64 {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		tb.Error(err)
		return 0
	}

	stats, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		tb.Error(errors.New("Not a syscall.Stat_t"))
		return 0
	}

	return stats.Ino
}

//nolint:deadcode,unused
func which(tb testing.TB, name string) string {
	executable, err := whichNonFatal(name)
	if err != nil {
		tb.Fatalf("%s", err)
	}
	return executable
}

//nolint:deadcode,unused
// whichNonFatal is "which" which returns an error instead of fatal
func whichNonFatal(name string) (string, error) {
	executable, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("could not resolve %s: %v", name, err)
	}

	if dest, err := filepath.EvalSymlinks(executable); err == nil {
		return dest, nil
	}

	return executable, nil
}

//nolint:deadcode,unused
func copyFile(src string, dst string, mode fs.FileMode) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, input, mode)
}

//nolint:deadcode,unused
func assertMode(tb testing.TB, actualMode, expectedMode uint32, msgAndArgs ...interface{}) bool {
	tb.Helper()
	if len(msgAndArgs) == 0 {
		msgAndArgs = append(msgAndArgs, "wrong mode")
	}
	return assert.Equal(tb, strconv.FormatUint(uint64(expectedMode), 8), strconv.FormatUint(uint64(actualMode), 8), msgAndArgs...)
}

//nolint:deadcode,unused
func assertRights(tb testing.TB, actualMode, expectedMode uint16, msgAndArgs ...interface{}) bool {
	tb.Helper()
	return assertMode(tb, uint32(actualMode)&01777, uint32(expectedMode), msgAndArgs...)
}

//nolint:deadcode,unused
func assertNearTime(tb testing.TB, ns uint64) bool {
	tb.Helper()
	now, event := time.Now(), time.Unix(0, int64(ns))
	if event.After(now) || event.Before(now.Add(-1*time.Hour)) {
		tb.Errorf("expected time close to %s, got %s", now, event)
		return false
	}
	return true
}

//nolint:deadcode,unused
func assertTriggeredRule(tb testing.TB, r *rules.Rule, id string) bool {
	tb.Helper()
	return assert.Equal(tb, id, r.ID, "wrong triggered rule")
}

//nolint:deadcode,unused
func assertReturnValue(tb testing.TB, retval, expected int64) bool {
	tb.Helper()
	return assert.Equal(tb, expected, retval, "wrong return value")
}

//nolint:deadcode,unused
func assertFieldEqual(tb testing.TB, e *sprobe.Event, field string, value interface{}, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	return assert.Equal(tb, value, fieldValue, msgAndArgs...)
}

//nolint:deadcode,unused
func assertFieldNotEmpty(tb testing.TB, e *sprobe.Event, field string, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	return assert.NotEmpty(tb, fieldValue, msgAndArgs...)
}

//nolint:deadcode,unused
func assertFieldContains(tb testing.TB, e *sprobe.Event, field string, value interface{}, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	return assert.Contains(tb, fieldValue, value, msgAndArgs...)
}

//nolint:deadcode,unused
func assertFieldStringArrayIndexedOneOf(tb *testing.T, e *sprobe.Event, field string, index int, values []string, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}

	if fieldValues, ok := fieldValue.([]string); ok {
		return assert.Contains(tb, values, fieldValues[index])
	}

	tb.Errorf("failed to get field '%s' as an array", field)
	return false
}

//nolint:deadcode,unused
func validateProcessContextLineage(tb testing.TB, event *sprobe.Event) bool {
	var data interface{}
	if err := json.Unmarshal([]byte(event.String()), &data); err != nil {
		tb.Error(err)
	}

	json, err := jsonpath.JsonPathLookup(data, "$.process.ancestors")
	if err != nil {
		tb.Errorf("should have a process context with ancestors, got %+v (%s)", json, spew.Sdump(data))
		return false
	}

	var prevPID, prevPPID float64

	for _, entry := range json.([]interface{}) {
		pce, ok := entry.(map[string]interface{})
		if !ok {
			tb.Errorf("invalid process cache entry, %+v", entry)
			return false
		}

		pid, ok := pce["pid"].(float64)
		if !ok || pid == 0 {
			tb.Errorf("invalid pid, %+v", pce)
			return false
		}

		// check lineage, exec should have the exact same pid, fork pid/ppid relationship
		if prevPID != 0 && pid != prevPID && pid != prevPPID {
			tb.Errorf("invalid process tree, parent/child broken (%f -> %f/%f), %+v", pid, prevPID, prevPPID, json)
			return false
		}
		prevPID = pid

		if pid != 1 {
			ppid, ok := pce["ppid"].(float64)
			if !ok {
				tb.Errorf("invalid pid, %+v", pce)
				return false
			}

			prevPPID = ppid
		}
	}

	if prevPID != 1 {
		tb.Errorf("invalid process tree, last ancestor should be pid 1, %+v", json)
	}

	return true
}

//nolint:deadcode,unused
func validateProcessContextSECL(tb testing.TB, event *sprobe.Event) bool {
	fields := []string{
		"process.file.path",
		"process.file.name",
		"process.ancestors.file.path",
		"process.ancestors.file.name",
	}

	for _, field := range fields {
		fieldValue, err := event.GetFieldValue(field)
		if err != nil {
			tb.Errorf("failed to get field '%s': %s", field, err)
			return false
		}

		switch value := fieldValue.(type) {
		case string:
			if len(value) == 0 {
				tb.Errorf("empty value for '%s'", field)
				return false
			}
		case []string:
			for _, v := range value {
				if len(v) == 0 {
					tb.Errorf("empty value for '%s'", field)
					return false
				}
			}
		default:
			tb.Errorf("unknown type value for '%s'", field)
			return false
		}
	}

	return true
}

//nolint:deadcode,unused
func validateProcessContext(tb testing.TB, event *sprobe.Event) {
	if event.ProcessContext.IsKworker {
		return
	}

	if !validateProcessContextLineage(tb, event) {
		tb.Error(event.String())
	}

	if !validateProcessContextSECL(tb, event) {
		tb.Error(event.String())
	}
}

//nolint:deadcode,unused
func validateEvent(tb testing.TB, validate func(event *sprobe.Event, rule *rules.Rule)) func(event *sprobe.Event, rule *rules.Rule) {
	return func(event *sprobe.Event, rule *rules.Rule) {
		validateProcessContext(tb, event)
		validate(event, rule)
	}
}

//nolint:deadcode,unused
func validateExecEvent(tb *testing.T, kind wrapperType, validate func(event *sprobe.Event, rule *rules.Rule)) func(event *sprobe.Event, rule *rules.Rule) {
	return func(event *sprobe.Event, rule *rules.Rule) {
		validate(event, rule)

		if kind == dockerWrapperType {
			assertFieldNotEmpty(tb, event, "exec.container.id", "exec container id not found")
			assertFieldNotEmpty(tb, event, "process.container.id", "process container id not found")
		}

		if !validateExecSchema(tb, event) {
			tb.Error(event.String())
		}
	}
}

func setTestPolicy(dir string, macros []*rules.MacroDefinition, rules []*rules.RuleDefinition) (string, error) {
	testPolicyFile, err := os.Create(path.Join(dir, "secagent-policy.policy"))
	if err != nil {
		return "", err
	}

	fail := func(err error) error {
		os.Remove(testPolicyFile.Name())
		return err
	}

	tmpl, err := template.New("test-policy").Parse(testPolicy)
	if err != nil {
		return "", fail(err)
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"Rules":  rules,
		"Macros": macros,
	}); err != nil {
		return "", fail(err)
	}

	_, err = testPolicyFile.Write(buffer.Bytes())
	if err != nil {
		return "", fail(err)
	}

	if err := testPolicyFile.Close(); err != nil {
		return "", fail(err)
	}

	return testPolicyFile.Name(), nil
}

func genTestConfig(dir string, opts testOpts) (*config.Config, error) {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		return nil, err
	}

	if opts.eventsCountThreshold == 0 {
		opts.eventsCountThreshold = 100000000
	}

	if opts.activityDumpRateLimiter == 0 {
		opts.activityDumpRateLimiter = 100
	}

	if len(opts.activityDumpTracedEventTypes) == 0 {
		opts.activityDumpTracedEventTypes = []string{"exec", "open", "bind", "dns", "syscalls"}
	}

	erpcDentryResolutionEnabled := true
	if opts.disableERPCDentryResolution {
		erpcDentryResolutionEnabled = false
	}

	mapDentryResolutionEnabled := true
	if opts.disableMapDentryResolution {
		mapDentryResolutionEnabled = false
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPoliciesDir":              dir,
		"DisableApprovers":             opts.disableApprovers,
		"EnableActivityDump":           opts.enableActivityDump,
		"ActivityDumpRateLimiter":      opts.activityDumpRateLimiter,
		"ActivityDumpTracedEventTypes": opts.activityDumpTracedEventTypes,
		"EventsCountThreshold":         opts.eventsCountThreshold,
		"ErpcDentryResolutionEnabled":  erpcDentryResolutionEnabled,
		"MapDentryResolutionEnabled":   mapDentryResolutionEnabled,
		"LogPatterns":                  logPatterns,
		"LogTags":                      logTags,
		"EnvsWithValue":                opts.envsWithValue,
	}); err != nil {
		return nil, err
	}

	sysprobeConfig, err := os.Create(path.Join(opts.testDir, "system-probe.yaml"))
	if err != nil {
		return nil, err
	}
	defer sysprobeConfig.Close()

	_, err = io.Copy(sysprobeConfig, buffer)
	if err != nil {
		return nil, err
	}

	agentConfig, err := sysconfig.New(sysprobeConfig.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	config, err := config.NewConfig(agentConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	config.ERPCDentryResolutionEnabled = !opts.disableERPCDentryResolution
	config.MapDentryResolutionEnabled = !opts.disableMapDentryResolution

	return config, nil
}

func newTestModule(t testing.TB, macroDefs []*rules.MacroDefinition, ruleDefs []*rules.RuleDefinition, opts testOpts) (*testModule, error) {
	var proFile *os.File
	if withProfile {
		var err error
		proFile, err = os.CreateTemp("/tmp", fmt.Sprintf("cpu-profile-%s", t.Name()))
		if err != nil {
			t.Fatal(err)
		}

		if err = os.Chmod(proFile.Name(), 0666); err != nil {
			t.Fatal(err)
		}

		t.Logf("Generating CPU profile in %s", proFile.Name())

		if err := pprof.StartCPUProfile(proFile); err != nil {
			t.Fatal(err)
		}
	}

	if err := initLogger(); err != nil {
		return nil, err
	}

	st, err := newSimpleTest(t, macroDefs, ruleDefs, opts.testDir)
	if err != nil {
		return nil, err
	}

	config, err := genTestConfig(st.root, opts)
	if err != nil {
		return nil, err
	}

	if _, err = setTestPolicy(st.root, macroDefs, ruleDefs); err != nil {
		return nil, err
	}

	var cmdWrapper cmdWrapper
	if testEnvironment == DockerEnvironment {
		cmdWrapper = newStdCmdWrapper()
	} else {
		wrapper, err := newDockerCmdWrapper(st.Root(), "ubuntu")
		if err == nil {
			cmdWrapper = newMultiCmdWrapper(wrapper, newStdCmdWrapper())
		} else {
			// docker not present run only on host
			cmdWrapper = newStdCmdWrapper()
		}
	}

	if testMod != nil && opts.Equal(testMod.opts) {
		testMod.st = st
		testMod.cmdWrapper = cmdWrapper
		testMod.t = t

		if err = testMod.reloadConfiguration(); err != nil {
			return testMod, err
		}

		if ruleDefs != nil && logStatusMetrics {
			t.Logf("%s entry stats: %s\n", t.Name(), GetStatusMetrics(testMod.probe))
		}
		return testMod, nil
	} else if testMod != nil {
		testMod.cleanup()
	}

	t.Log("Instantiating a new security module")

	statsdClient := NewStatsdClient()

	if opts.disableApprovers {
		config.EnableApprovers = false
	}

	testMod = &testModule{
		config:        config,
		opts:          opts,
		st:            st,
		t:             t,
		cmdWrapper:    cmdWrapper,
		statsdClient:  statsdClient,
		proFile:       proFile,
		eventHandlers: eventHandlers{},
	}

	mod, err := module.NewModule(config, module.Opts{StatsdClient: statsdClient, EventSender: testMod})
	if err != nil {
		return nil, fmt.Errorf("failed to create module: %w", err)
	}

	testMod.module = mod.(*module.Module)
	testMod.probe = testMod.module.GetProbe()

	var loadErr *multierror.Error
	testMod.module.SetRulesetLoadedCallback(func(rs *rules.RuleSet, err *multierror.Error) {
		loadErr = err
		log.Infof("Adding test module as listener")
		rs.AddListener(testMod)
	})

	testMod.probe.AddEventHandler(model.UnknownEventType, testMod)
	testMod.probe.AddNewNotifyDiscarderPushedCallback(testMod.NotifyDiscarderPushedCallback)

	if err := testMod.module.Init(); err != nil {
		return nil, fmt.Errorf("failed to init module: %w", err)
	}

	kv, _ := kernel.NewKernelVersion()

	if os.Getenv("DD_TESTS_RUNTIME_COMPILED") == "1" && !testMod.module.GetProbe().IsRuntimeCompiled() && !kv.IsSuseKernel() {
		return nil, errors.New("failed to runtime compile module")
	}

	if err := testMod.module.Start(); err != nil {
		return nil, fmt.Errorf("failed to start module: %w", err)
	}

	if loadErr.ErrorOrNil() != nil {
		defer testMod.Close()
		return nil, loadErr.ErrorOrNil()
	}

	if logStatusMetrics {
		t.Logf("%s entry stats: %s\n", t.Name(), GetStatusMetrics(testMod.probe))
	}

	return testMod, nil
}

func (tm *testModule) HandleEvent(event *sprobe.Event) {
	tm.eventHandlers.RLock()
	defer tm.eventHandlers.RUnlock()

	if tm.eventHandlers.onProbeEvent != nil {
		tm.eventHandlers.onProbeEvent(event)
	}
}

func (tm *testModule) HandleCustomEvent(rule *rules.Rule, event *sprobe.CustomEvent) {}

func (tm *testModule) SendEvent(rule *rules.Rule, event module.Event, extTagsCb func() []string, service string) {
	tm.eventHandlers.RLock()
	defer tm.eventHandlers.RUnlock()

	switch ev := event.(type) {
	case *sprobe.Event:
	case *sprobe.CustomEvent:
		if tm.eventHandlers.onCustomSendEvent != nil {
			tm.eventHandlers.onCustomSendEvent(rule, ev)
		}
	}
}

func (tm *testModule) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	tm.cmdWrapper.Run(t, name, fnc)
}

func (tm *testModule) reloadConfiguration() error {
	log.Debugf("reload configuration with testDir: %s", tm.Root())
	tm.config.PoliciesDir = tm.Root()

	provider, err := rules.NewPoliciesDirProvider(tm.config.PoliciesDir, false)
	if err != nil {
		return err
	}

	if err := tm.module.LoadPolicies([]rules.PolicyProvider{provider}, true); err != nil {
		return fmt.Errorf("failed to reload test module: %w", err)
	}

	return nil
}

func (tm *testModule) Root() string {
	return tm.st.root
}

func (tm *testModule) RuleMatch(rule *rules.Rule, event eval.Event) {
	tm.eventHandlers.RLock()
	callback := tm.eventHandlers.onRuleMatch
	tm.eventHandlers.RUnlock()

	if callback != nil {
		callback(event.(*sprobe.Event), rule)
	}
}

func (tm *testModule) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
}

func (tm *testModule) RegisterDiscarderPushedHandler(cb onDiscarderPushedHandler) {
	tm.eventHandlers.Lock()
	tm.eventHandlers.onDiscarderPushed = cb
	tm.eventHandlers.Unlock()
}

func (tm *testModule) NotifyDiscarderPushedCallback(eventType string, event *sprobe.Event, field string) {
	tm.eventHandlers.RLock()
	callback := tm.eventHandlers.onDiscarderPushed
	tm.eventHandlers.RUnlock()

	if callback != nil {
		_ = callback(event, field, eventType)
	}
}

func (tm *testModule) GetEventDiscarder(tb testing.TB, action func() error, cb onDiscarderPushedHandler) error {
	tb.Helper()

	message := make(chan ActionMessage, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tm.RegisterDiscarderPushedHandler(func(event eval.Event, field eval.Field, eventType eval.EventType) bool {
		tb.Helper()

		select {
		case <-ctx.Done():
			return true
		case msg := <-message:
			switch msg {
			case Skip:
				cancel()
			case Continue:
				if cb(event, field, eventType) {
					cancel()
				} else {
					message <- Continue
				}
			}
		}
		return true
	})

	defer func() {
		tm.RegisterDiscarderPushedHandler(nil)
	}()

	if err := action(); err != nil {
		message <- Skip
		return err
	}
	message <- Continue

	select {
	case <-time.After(getEventTimeout):
		return NewTimeoutError(tm.probe)
	case <-ctx.Done():
		return nil
	}
}

// GetStatusMetrics returns a string representation of the perf buffer monitor metrics
func GetStatusMetrics(probe *sprobe.Probe) string {
	if probe == nil {
		return ""
	}
	monitor := probe.GetMonitor()
	if monitor == nil {
		return ""
	}
	perfBufferMonitor := monitor.GetPerfBufferMonitor()
	if perfBufferMonitor == nil {
		return ""
	}

	var status strings.Builder
	status.WriteString(fmt.Sprintf("%d lost", perfBufferMonitor.GetKernelLostCount("events", -1)))

	for i := model.UnknownEventType + 1; i < model.MaxKernelEventType; i++ {
		stats, kernelStats := perfBufferMonitor.GetEventStats(i, "events", -1)
		if stats.Count.Load() == 0 && kernelStats.Count.Load() == 0 && kernelStats.Lost.Load() == 0 {
			continue
		}
		status.WriteString(fmt.Sprintf(", %s user:%d kernel:%d lost:%d", i, stats.Count.Load(), kernelStats.Count.Load(), kernelStats.Lost.Load()))
	}

	return status.String()
}

// ErrTimeout is used to indicate that a test timed out
type ErrTimeout struct {
	msg string
}

func (et ErrTimeout) Error() string {
	return et.msg
}

// NewTimeoutError returns a new timeout error with the metrics collected during the test
func NewTimeoutError(probe *sprobe.Probe) ErrTimeout {
	err := ErrTimeout{
		"timeout, ",
	}

	err.msg += GetStatusMetrics(probe)
	return err
}

// ActionMessage is used to send a message from an action function to its callback
type ActionMessage int

const (
	// Continue means that the callback should execute normally
	Continue ActionMessage = iota
	// Skip means that the callback should skip the test
	Skip
)

// ErrSkipTest is used to notify that a test should be skipped
type ErrSkipTest struct {
	msg string
}

func (err ErrSkipTest) Error() string {
	return err.msg
}

func (tm *testModule) WaitSignal(tb testing.TB, action func() error, cb onRuleHandler) {
	tb.Helper()

	if err := tm.GetSignal(tb, action, validateEvent(tb, cb)); err != nil {
		if _, ok := err.(ErrSkipTest); ok {
			tb.Skip(err)
		} else {
			tb.Fatal(err)
		}
	}
}

func (tm *testModule) GetSignal(tb testing.TB, action func() error, cb onRuleHandler) error {
	tb.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	message := make(chan ActionMessage, 1)
	failNow := make(chan bool, 1)

	tm.RegisterRuleEventHandler(func(e *sprobe.Event, r *rules.Rule) {
		tb.Helper()
		select {
		case <-ctx.Done():
			return
		case msg := <-message:
			switch msg {
			case Continue:
				cb(e, r)
				if tb.Skipped() || tb.Failed() {
					failNow <- true
				}
			case Skip:
			}
		}
		cancel()
	})

	defer func() {
		tm.RegisterRuleEventHandler(nil)
	}()

	if err := action(); err != nil {
		message <- Skip
		return err
	}
	message <- Continue

	select {
	case <-failNow:
		tb.FailNow()
		return nil
	case <-time.After(getEventTimeout):
		return NewTimeoutError(tm.probe)
	case <-ctx.Done():
		return nil
	}
}

func (tm *testModule) RegisterRuleEventHandler(cb onRuleHandler) {
	tm.eventHandlers.Lock()
	tm.eventHandlers.onRuleMatch = cb
	tm.eventHandlers.Unlock()
}

func (tm *testModule) GetCustomEventSent(tb testing.TB, action func() error, cb func(rule *rules.Rule, event *sprobe.CustomEvent) bool, eventType ...model.EventType) error {
	tb.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	message := make(chan ActionMessage, 1)

	tm.RegisterCustomSendEventHandler(func(rule *rules.Rule, event *sprobe.CustomEvent) {
		if len(eventType) > 0 {
			if event.GetEventType() != eventType[0] {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case msg := <-message:
			switch msg {
			case Continue:
				if cb(rule, event) {
					cancel()
				} else {
					message <- Continue
				}
			case Skip:
				cancel()
			}
		}
	})
	defer tm.RegisterCustomSendEventHandler(nil)

	if err := action(); err != nil {
		message <- Skip
		return err
	}
	message <- Continue

	select {
	case <-time.After(getEventTimeout):
		return NewTimeoutError(tm.probe)
	case <-ctx.Done():
		return nil
	}
}

func (tm *testModule) RegisterProbeEventHandler(cb onProbeEventHandler) {
	tm.eventHandlers.Lock()
	tm.eventHandlers.onProbeEvent = cb
	tm.eventHandlers.Unlock()
}

func (tm *testModule) RegisterCustomSendEventHandler(cb onCustomSendEventHandler) {
	tm.eventHandlers.Lock()
	tm.eventHandlers.onCustomSendEvent = cb
	tm.eventHandlers.Unlock()
}

func (tm *testModule) GetProbeEvent(action func() error, cb func(event *sprobe.Event) bool, timeout time.Duration, eventTypes ...model.EventType) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	message := make(chan ActionMessage, 1)

	tm.RegisterProbeEventHandler(func(event *sprobe.Event) {
		if len(eventTypes) > 0 {
			match := false
			for _, eventType := range eventTypes {
				if event.GetEventType() == eventType {
					match = true
					break
				}
			}
			if !match {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case msg := <-message:
			switch msg {
			case Continue:
				if cb(event) {
					cancel()
				} else {
					message <- Continue
				}
			case Skip:
				cancel()
			}
		}
	})
	defer tm.RegisterProbeEventHandler(nil)

	if action == nil {
		message <- Continue
	} else {
		if err := action(); err != nil {
			message <- Skip
			return err
		}
		message <- Continue
	}

	select {
	case <-time.After(timeout):
		return NewTimeoutError(tm.probe)
	case <-ctx.Done():
		return nil
	}
}

func (tm *testModule) Path(filename ...string) (string, unsafe.Pointer, error) {
	return tm.st.Path(filename...)
}

func (tm *testModule) CreateWithOptions(filename string, user, group, mode int) (string, unsafe.Pointer, error) {
	testFile, testFilePtr, err := tm.st.Path(filename)
	if err != nil {
		return testFile, testFilePtr, err
	}

	// Create file
	f, err := os.OpenFile(testFile, os.O_CREATE, os.FileMode(mode))
	if err != nil {
		return "", nil, err
	}
	f.Close()

	// Chown the file
	err = os.Chown(testFile, user, group)
	return testFile, testFilePtr, err
}

func (tm *testModule) Create(filename string) (string, unsafe.Pointer, error) {
	testFile, testPtr, err := tm.st.Path(filename)
	if err != nil {
		return "", nil, err
	}

	f, err := os.Create(testFile)
	if err != nil {
		return "", nil, err
	}

	if err := f.Close(); err != nil {
		return "", nil, err
	}

	return testFile, testPtr, err
}

//nolint:unused
type tracePipeLogger struct {
	*TracePipe
	stop       chan struct{}
	executable string
}

//nolint:unused
func (l *tracePipeLogger) handleEvent(event *TraceEvent) {
	// for some reason, the event task is resolved to "<...>"
	// so we check that event.PID is the ID of a task of the running process
	taskPath := filepath.Join(util.HostProc(), strconv.Itoa(int(utils.Getpid())), "task", event.PID)
	_, err := os.Stat(taskPath)

	if event.Task == l.executable || (event.Task == "<...>" && err == nil) {
		log.Debug(event.Raw)
	}
}

//nolint:unused
func (l *tracePipeLogger) Start() {
	channelEvents, channelErrors := l.Channel()

	go func() {
		for {
			select {
			case <-l.stop:
				for len(channelEvents) > 0 {
					l.handleEvent(<-channelEvents)
				}
				return
			case event := <-channelEvents:
				l.handleEvent(event)
			case err := <-channelErrors:
				log.Error(err)
			}
		}
	}()
}

//nolint:unused
func (l *tracePipeLogger) Stop() {
	time.Sleep(time.Millisecond * 200)

	l.stop <- struct{}{}
	l.Close()
}

//nolint:unused
func (tm *testModule) startTracing() (*tracePipeLogger, error) {
	tracePipe, err := NewTracePipe()
	if err != nil {
		return nil, err
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	logger := &tracePipeLogger{
		TracePipe:  tracePipe,
		stop:       make(chan struct{}),
		executable: filepath.Base(executable),
	}
	logger.Start()

	time.Sleep(time.Millisecond * 200)

	return logger, nil
}

func (tm *testModule) cleanup() {
	tm.module.Close()
}

func (tm *testModule) validateAbnormalPaths() {
	assert.Zero(tm.t, tm.statsdClient.Get("datadog.runtime_security.rules.rate_limiter.allow:rule_id:abnormal_path"))
}

func (tm *testModule) Close() {
	tm.module.SendStats()

	if !tm.opts.disableAbnormalPathCheck {
		tm.validateAbnormalPaths()
	}

	tm.statsdClient.Flush()

	if logStatusMetrics {
		tm.t.Logf("%s exit stats: %s\n", tm.t.Name(), GetStatusMetrics(tm.probe))
	}
}

var logInitilialized bool

func initLogger() error {
	logLevel, found := seelog.LogLevelFromString(logLevelStr)
	if !found {
		return fmt.Errorf("invalid log level '%s'", logLevel)
	}

	if !logInitilialized {
		if _, err := swapLogLevel(logLevel); err != nil {
			return err
		}

		logInitilialized = true
	}
	return nil
}

func swapLogLevel(logLevel seelog.LogLevel) (seelog.LogLevel, error) {
	if logger == nil {
		logFormat := "[%Date(2006-01-02 15:04:05.000)] [%LEVEL] %Func:%Line %Msg\n"

		var err error

		logger, err = seelog.LoggerFromWriterWithMinLevelAndFormat(os.Stdout, logLevel, logFormat)
		if err != nil {
			return 0, err
		}
	}
	log.SetupLogger(logger, logLevel.String())

	prevLevel, _ := seelog.LogLevelFromString(logLevelStr)
	logLevelStr = logLevel.String()
	return prevLevel, nil
}

type simpleTest struct {
	root string
}

func (t *simpleTest) Root() string {
	return t.root
}

func (t *simpleTest) ProcessName() string {
	executable, _ := os.Executable()
	return path.Base(executable)
}

func (t *simpleTest) Path(filename ...string) (string, unsafe.Pointer, error) {
	components := []string{t.root}
	components = append(components, filename...)
	path := path.Join(components...)
	filenamePtr, err := syscall.BytePtrFromString(path)
	if err != nil {
		return "", nil, err
	}
	return path, unsafe.Pointer(filenamePtr), nil
}

func (t *simpleTest) load(macros []*rules.MacroDefinition, rules []*rules.RuleDefinition) (err error) {
	executeExpressionTemplate := func(expression string) (string, error) {
		buffer := new(bytes.Buffer)
		tmpl, err := template.New("").Parse(expression)
		if err != nil {
			return "", err
		}

		if err := tmpl.Execute(buffer, t); err != nil {
			return "", err
		}

		return buffer.String(), nil
	}

	for _, rule := range rules {
		if rule.Expression, err = executeExpressionTemplate(rule.Expression); err != nil {
			return err
		}
	}

	for _, macro := range macros {
		if macro.Expression, err = executeExpressionTemplate(macro.Expression); err != nil {
			return err
		}
	}

	return nil
}

func newSimpleTest(tb testing.TB, macros []*rules.MacroDefinition, rules []*rules.RuleDefinition, testDir string) (*simpleTest, error) {
	t := &simpleTest{
		root: testDir,
	}

	if testDir == "" {
		t.root = tb.TempDir()

		targetFileMode := fs.FileMode(0o711)

		// chmod the root and its parent since TempDir returns a 2-layers directory `/tmp/TestNameXXXX/NNN/`
		if err := os.Chmod(t.root, targetFileMode); err != nil {
			return nil, err
		}
		if err := os.Chmod(filepath.Dir(t.root), targetFileMode); err != nil {
			return nil, err
		}
	}

	if err := t.load(macros, rules); err != nil {
		return nil, err
	}

	return t, nil
}

// systemUmask caches the system umask between tests
var systemUmask int //nolint:unused

//nolint:deadcode,unused
func applyUmask(fileMode int) int {
	if systemUmask == 0 {
		// Get the system umask to compute the right access mode
		systemUmask = unix.Umask(0)
		// the previous line overrides the system umask, change it back
		_ = unix.Umask(systemUmask)
	}
	return fileMode &^ systemUmask
}

//nolint:deadcode,unused
func ifSyscallSupported(syscall string, test func(t *testing.T, syscallNB uintptr)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		syscallNB, found := supportedSyscalls[syscall]
		if !found {
			t.Skipf("%s is not supported", syscall)
		}

		test(t, syscallNB)
	}
}

// waitForProbeEvent returns the first open event with the provided filename.
// WARNING: this function may yield a "fatal error: concurrent map writes" error if the ruleset of testModule does not
// contain a rule on "open.file.path"
//nolint:deadcode,unused
func waitForProbeEvent(test *testModule, action func() error, key string, value interface{}, eventType model.EventType) error {
	return test.GetProbeEvent(action, func(event *sprobe.Event) bool {
		if v, _ := event.GetFieldValue(key); v == value {
			return true
		}
		return false
	}, getEventTimeout, eventType)
}

//nolint:deadcode,unused
func waitForOpenProbeEvent(test *testModule, action func() error, filename string) error {
	return waitForProbeEvent(test, action, "open.file.path", filename, model.FileOpenEventType)
}

// TestMain is the entry points for functional tests
func TestMain(m *testing.M) {
	flag.Parse()
	retCode := m.Run()
	if testMod != nil {
		testMod.cleanup()
	}
	os.Exit(retCode)
}

func init() {
	os.Setenv("RUNTIME_SECURITY_TESTSUITE", "true")
	flag.StringVar(&testEnvironment, "env", HostEnvironment, "environment used to run the test suite: ex: host, docker")
	flag.StringVar(&logLevelStr, "loglevel", seelog.WarnStr, "log level")
	flag.Var(&logPatterns, "logpattern", "List of log pattern")
	flag.Var(&logTags, "logtag", "List of log tag")
	flag.BoolVar(&logStatusMetrics, "status-metrics", false, "display status metrics")
	flag.BoolVar(&withProfile, "with-profile", false, "enable profile per test")

	rand.Seed(time.Now().UnixNano())

	testSuitePid = uint32(utils.Getpid())
}

//nolint:deadcode,unused
func checkKernelCompatibility(tb testing.TB, why string, skipCheck func(kv *kernel.Version) bool) {
	tb.Helper()
	kv, err := kernel.NewKernelVersion()
	if err != nil {
		tb.Errorf("failed to get kernel version: %s", err)
		return
	}

	if skipCheck(kv) {
		tb.Skipf("kernel version not supported: %s", why)
	}
}

func (tm *testModule) StartActivityDumpComm(t *testing.T, comm string, outputDir string, formats []string) ([]string, error) {
	monitor := tm.probe.GetMonitor()
	if monitor == nil {
		return nil, errors.New("No monitor")
	}
	p := &api.ActivityDumpParams{
		Comm:              comm,
		Timeout:           1,
		DifferentiateArgs: true,
		Storage: &api.StorageRequestParams{
			LocalStorageDirectory:    outputDir,
			LocalStorageFormats:      formats,
			LocalStorageCompression:  false,
			RemoteStorageFormats:     []string{},
			RemoteStorageCompression: false,
		},
	}
	mess, err := monitor.DumpActivity(p)
	if err != nil || mess == nil || len(mess.Storage) < 1 {
		t.Errorf("failed to start activity dump: %s", err)
		return nil, err
	}

	var files []string
	for _, s := range mess.Storage {
		files = append(files, s.File)
	}
	return files, nil
}

func (tm *testModule) StopActivityDumpComm(t *testing.T, comm string) error {
	monitor := tm.probe.GetMonitor()
	if monitor == nil {
		return errors.New("No monitor")
	}
	p := &api.ActivityDumpStopParams{
		Comm: comm,
	}
	_, err := monitor.StopActivityDump(p)
	if err != nil {
		t.Errorf("failed to start activity dump: %s", err)
		return err
	}
	return nil
}

func (tm *testModule) DecodeActivityDump(t *testing.T, path string) (*sprobe.ActivityDump, error) {
	monitor := tm.probe.GetMonitor()
	if monitor == nil {
		return nil, errors.New("No monitor")
	}

	adm := monitor.GetActivityDumpManager()
	if adm == nil {
		return nil, errors.New("No activity dump manager")
	}

	ad := sprobe.NewActivityDump(adm)
	if ad == nil {
		return nil, errors.New("Creation of new activity dump fails")
	}

	if err := ad.Decode(path); err != nil {
		return nil, err
	}

	return ad, nil
}
