// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && (functionaltests || stresstests)

// Package tests holds tests related files
package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/cihub/seelog"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/go-multierror"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	rulesmodule "github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/rules/bundled"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	utilkernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	logger seelog.LoggerInterface
)

const testConfig = `---
log_level: DEBUG
system_probe_config:
  sysprobe_socket: /tmp/test-sysprobe.sock
  enable_kernel_header_download: true
  enable_runtime_compiler: true

event_monitoring_config:
  socket: /tmp/test-event-monitor.sock
  remote_tagger: false
  custom_sensitive_words:
    - "*custom*"
  network:
    enabled: true
    ingress:
      enabled: {{ .NetworkIngressEnabled }}
    raw_packet:
      enabled: {{ .NetworkRawPacketEnabled}}
  flush_discarder_window: 0
{{if .DisableFilters}}
  enable_kernel_filters: false
{{end}}
{{if .DisableApprovers}}
  enable_approvers: false
{{end}}
{{if .DisableDiscarders}}
  enable_discarders: false
{{end}}
  erpc_dentry_resolution_enabled: {{ .ErpcDentryResolutionEnabled }}
  map_dentry_resolution_enabled: {{ .MapDentryResolutionEnabled }}
  envs_with_value:
  {{range .EnvsWithValue}}
    - {{.}}
  {{end}}

runtime_security_config:
  enabled: {{ .RuntimeSecurityEnabled }}
  internal_monitoring:
    enabled: true
{{ if gt .EventServerRetention 0 }}
  event_server:
    retention: {{ .EventServerRetention }}
{{ end }}
  remote_configuration:
    enabled: false
  on_demand:
    enabled: true
    rate_limiter:
      enabled: {{ .OnDemandRateLimiterEnabled}}
  socket: /tmp/test-runtime-security.sock
  sbom:
    enabled: {{ .SBOMEnabled }}
    host:
      enabled: {{ .HostSBOMEnabled }}
  activity_dump:
    enabled: {{ .EnableActivityDump }}
    syscall_monitor:
      period: {{ .ActivityDumpSyscallMonitorPeriod }}
{{if .EnableActivityDump}}
    rate_limiter: {{ .ActivityDumpRateLimiter }}
    tag_rules:
      enabled: {{ .ActivityDumpTagRules }}
    dump_duration: {{ .ActivityDumpDuration }}
    {{if .ActivityDumpLoadControllerPeriod }}
    load_controller_period: {{ .ActivityDumpLoadControllerPeriod }}
    {{end}}
    {{if .ActivityDumpCleanupPeriod }}
    cleanup_period: {{ .ActivityDumpCleanupPeriod }}
    {{end}}
    {{if .ActivityDumpLoadControllerTimeout }}
    min_timeout: {{ .ActivityDumpLoadControllerTimeout }}
    {{end}}
    traced_cgroups_count: {{ .ActivityDumpTracedCgroupsCount }}
    cgroup_differentiate_args: {{ .ActivityDumpCgroupDifferentiateArgs }}
    auto_suppression:
      enabled: {{ .ActivityDumpAutoSuppressionEnabled }}
    traced_event_types: {{range .ActivityDumpTracedEventTypes}}
    - {{. -}}
    {{- end}}
    local_storage:
      output_directory: {{ .ActivityDumpLocalStorageDirectory }}
      compression: {{ .ActivityDumpLocalStorageCompression }}
      formats: {{range .ActivityDumpLocalStorageFormats}}
      - {{. -}}
      {{- end}}
{{end}}
  security_profile:
    enabled: {{ .EnableSecurityProfile }}
{{if .EnableSecurityProfile}}
    max_image_tags: {{ .SecurityProfileMaxImageTags }}
    dir: {{ .SecurityProfileDir }}
    watch_dir: {{ .SecurityProfileWatchDir }}
    auto_suppression:
      enabled: {{ .EnableAutoSuppression }}
      event_types: {{range .AutoSuppressionEventTypes}}
      - {{. -}}
      {{- end}}
    anomaly_detection:
      enabled: {{ .EnableAnomalyDetection }}
      event_types: {{range .AnomalyDetectionEventTypes}}
      - {{. -}}
      {{- end}}
      default_minimum_stable_period: {{.AnomalyDetectionDefaultMinimumStablePeriod}}
      minimum_stable_period:
        exec: {{.AnomalyDetectionMinimumStablePeriodExec}}
        dns: {{.AnomalyDetectionMinimumStablePeriodDNS}}
      workload_warmup_period: {{.AnomalyDetectionWarmupPeriod}}
{{end}}

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
  ebpfless:
    enabled: {{.EBPFLessEnabled}}
  hash_resolver:
    enabled: true
  enforcement:
    exclude_binaries:
      - {{ .EnforcementExcludeBinary }}
    rule_source_allowed:
      - file
    disarmer:
      container:
        enabled: {{.EnforcementDisarmerContainerEnabled}}
        max_allowed: {{.EnforcementDisarmerContainerMaxAllowed}}
        period: {{.EnforcementDisarmerContainerPeriod}}
      executable:
        enabled: {{.EnforcementDisarmerExecutableEnabled}}
        max_allowed: {{.EnforcementDisarmerExecutableMaxAllowed}}
        period: {{.EnforcementDisarmerExecutablePeriod}}
`

const (
	// HostEnvironment for the Host environment
	HostEnvironment = "host"
	// DockerEnvironment for the docker container environment
	DockerEnvironment = "docker"
)

type testModule struct {
	sync.RWMutex
	secconfig     *secconfig.Config
	opts          tmOpts
	st            *simpleTest
	t             testing.TB
	eventMonitor  *eventmonitor.EventMonitor
	cws           *module.CWSConsumer
	probe         *sprobe.Probe
	eventHandlers eventHandlers
	cmdWrapper    cmdWrapper
	statsdClient  *statsdclient.StatsdClient
	proFile       *os.File
	ruleEngine    *rulesmodule.RuleEngine
	tracePipe     *tracePipeLogger
	msgSender     *fakeMsgSender
}

var testMod *testModule
var commonCfgDir string

type onRuleHandler func(*model.Event, *rules.Rule)
type onProbeEventHandler func(*model.Event)
type onCustomSendEventHandler func(*rules.Rule, *events.CustomEvent)
type onSendEventHandler func(*rules.Rule, *model.Event)
type onDiscarderPushedHandler func(event eval.Event, field eval.Field, eventType eval.EventType) bool

type eventHandlers struct {
	sync.RWMutex
	onRuleMatch       onRuleHandler
	onProbeEvent      onProbeEventHandler
	onCustomSendEvent onCustomSendEventHandler
	onSendEvent       onSendEventHandler
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

// whichNonFatal is "which" which returns an error instead of fatal
//
//nolint:deadcode,unused
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
func assertInode(tb testing.TB, actualInode, expectedInode uint64, msgAndArgs ...interface{}) bool {
	tb.Helper()

	if ebpfLessEnabled {
		return true
	}

	if len(msgAndArgs) == 0 {
		msgAndArgs = append(msgAndArgs, "wrong inode")
	}
	return assert.Equal(tb, strconv.FormatUint(uint64(expectedInode), 8), strconv.FormatUint(uint64(actualInode), 8), msgAndArgs...)
}

//nolint:deadcode,unused
func assertRights(tb testing.TB, actualMode, expectedMode uint16, msgAndArgs ...interface{}) bool {
	tb.Helper()
	return assertMode(tb, uint32(actualMode)&01777, uint32(expectedMode), msgAndArgs...)
}

//nolint:deadcode,unused
func assertNearTimeObject(tb testing.TB, eventTime time.Time) bool {
	tb.Helper()
	now := time.Now()
	if eventTime.After(now) || eventTime.Before(now.Add(-1*time.Hour)) {
		tb.Errorf("expected time close to %s, got %s", now, eventTime)
		return false
	}
	return true
}

//nolint:deadcode,unused
func assertNearTime(tb testing.TB, ns uint64) bool {
	tb.Helper()
	return assertNearTimeObject(tb, time.Unix(0, int64(ns)))
}

//nolint:deadcode,unused
func assertNotTriggeredRule(tb testing.TB, r *rules.Rule, id string) bool {
	tb.Helper()
	return assert.NotEqual(tb, id, r.ID, "wrong triggered rule")
}

//nolint:deadcode,unused
func assertReturnValue(tb testing.TB, retval, expected int64) bool {
	tb.Helper()
	return assert.Equal(tb, expected, retval, "wrong return value")
}

//nolint:deadcode,unused
func validateProcessContextLineage(tb testing.TB, event *model.Event) {
	eventJSON, err := serializers.MarshalEvent(event, nil)
	if err != nil {
		tb.Errorf("failed to marshal event: %v", err)
		return
	}

	var data interface{}
	if err := json.Unmarshal(eventJSON, &data); err != nil {
		tb.Error(err)
		tb.Error(string(eventJSON))
		return
	}

	json, err := jsonpath.JsonPathLookup(data, "$.process.ancestors")
	if err != nil {
		if event.Origin != "ebpfless" { // first exec event can't have ancestors
			tb.Errorf("should have a process context with ancestors, got %+v (%s)", json, spew.Sdump(data))
			tb.Error(string(eventJSON))
		}
		return
	}

	var prevPID, prevPPID float64
	var prevArgs []interface{}

	for _, entry := range json.([]interface{}) {
		pce, ok := entry.(map[string]interface{})
		if !ok {
			tb.Errorf("invalid process cache entry, %+v", entry)
			tb.Error(string(eventJSON))
			return
		}

		pid, ok := pce["pid"].(float64)
		if !ok || pid == 0 {
			tb.Errorf("invalid pid, %+v", pce)
			tb.Error(string(eventJSON))
			return
		}

		// check lineage, exec should have the exact same pid, fork pid/ppid relationship
		if prevPID != 0 && pid != prevPID && pid != prevPPID {
			tb.Errorf("invalid process tree, parent/child broken (%f -> %f/%f), %+v", pid, prevPID, prevPPID, json)
			tb.Error(string(eventJSON))
			return
		}

		if pid != 1 {
			ppid, ok := pce["ppid"].(float64)
			if !ok {
				// could happen in ebpfless, because we don't have complete lineage
				if event.Origin != "ebpfless" {
					tb.Errorf("invalid pid, %+v", pce)
					tb.Error(string(eventJSON))
				}
				return
			}

			prevPPID = ppid
		}

		// check that parent/child ancestors have deduplicated args
		args, ok := pce["args"].([]interface{})
		if ok && len(args) > 0 {
			if pid != prevPID && prevArgs != nil {
				if reflect.DeepEqual(args, prevArgs) {
					tb.Errorf("invalid process tree, same parent/child args (%d/%d) %+q", int(pid), int(prevPID), args)
					tb.Error(string(eventJSON))
					return
				}
			}
			prevArgs = args
		} else {
			prevArgs = nil
		}

		prevPID = pid
	}

	if event.Origin != "ebpfless" && prevPID != 1 {
		tb.Errorf("invalid process tree, last ancestor should be pid 1, %+v", json)
		tb.Error(string(eventJSON))
	}
}

//nolint:deadcode,unused
func validateProcessContextSECL(tb testing.TB, event *model.Event) {
	// Process file name values cannot be blank
	nameFields := []string{
		"process.file.name",
	}
	if event.Origin != "ebpfless" {
		nameFields = append(nameFields,
			"process.ancestors.file.name",
			"process.parent.file.name",
		)
	}

	nameFieldValid, hasPath := checkProcessContextFieldsForBlankValues(tb, event, nameFields)

	// Process path values can be blank if the process was a fileless execution
	pathFields := []string{
		"process.file.path",
	}
	if event.Origin != "ebpfless" {
		pathFields = append(pathFields,
			"process.parent.file.path",
			"process.ancestors.file.path",
		)
	}

	pathFieldValid := true
	if hasPath {
		pathFieldValid, _ = checkProcessContextFieldsForBlankValues(tb, event, pathFields)
	}

	valid := nameFieldValid && pathFieldValid

	if !valid {
		eventJSON, err := serializers.MarshalEvent(event, nil)
		if err != nil {
			tb.Errorf("failed to marshal event: %v", err)
			return
		}
		tb.Error(string(eventJSON))
	}
}

func checkProcessContextFieldsForBlankValues(tb testing.TB, event *model.Event, fieldNamesToCheck []string) (bool, bool) {
	validField := true
	hasPath := true

	for _, field := range fieldNamesToCheck {
		fieldValue, err := event.GetFieldValue(field)
		if err != nil {
			tb.Errorf("failed to get field '%s': %s", field, err)
			validField = false
		}

		switch value := fieldValue.(type) {
		case string:
			if len(value) == 0 {
				tb.Errorf("empty value for '%s'", field)
				validField = false
			}

			if strings.HasSuffix(field, ".name") && strings.HasPrefix(value, filelessExecutionFilenamePrefix) {
				hasPath = false
			}
		case []string:
			for _, v := range value {
				if len(v) == 0 {
					tb.Errorf("empty value for '%s'", field)
					validField = false
				}
				if strings.HasSuffix(field, ".name") && strings.HasPrefix(v, filelessExecutionFilenamePrefix) {
					hasPath = false
				}
			}
		default:
			tb.Errorf("unknown type value for '%s'", field)
			validField = false
		}
	}

	return validField, hasPath
}

//nolint:deadcode,unused
func validateSyscallContext(tb testing.TB, event *model.Event, jsonPath string) {
	if ebpfLessEnabled {
		return
	}

	eventJSON, err := serializers.MarshalEvent(event, nil)
	if err != nil {
		tb.Errorf("failed to marshal event: %v", err)
		return
	}

	var data interface{}
	if err := json.Unmarshal(eventJSON, &data); err != nil {
		tb.Error(err)
		tb.Error(string(eventJSON))
		return
	}

	json, err := jsonpath.JsonPathLookup(data, jsonPath)
	if err != nil {
		tb.Errorf("should have a syscall context, got %+v (%s)", json, spew.Sdump(data))
		tb.Error(string(eventJSON))
	}
}

//nolint:deadcode,unused
func validateProcessContext(tb testing.TB, event *model.Event) {
	if event.ProcessContext.IsKworker {
		return
	}

	validateProcessContextLineage(tb, event)
	validateProcessContextSECL(tb, event)
}

//nolint:deadcode,unused
func validateEvent(tb testing.TB, validate func(event *model.Event, rule *rules.Rule)) func(event *model.Event, rule *rules.Rule) {
	return func(event *model.Event, rule *rules.Rule) {
		validate(event, rule)
		validateProcessContext(tb, event)
	}
}

//nolint:deadcode,unused
func (tm *testModule) validateExecEvent(tb *testing.T, kind wrapperType, validate func(event *model.Event, rule *rules.Rule)) func(event *model.Event, rule *rules.Rule) {
	return func(event *model.Event, rule *rules.Rule) {
		validate(event, rule)

		if kind == dockerWrapperType {
			assertFieldNotEmpty(tb, event, "exec.container.id", "exec container id not found")
			assertFieldNotEmpty(tb, event, "process.container.id", "process container id not found")
		}

		if event.Origin != "ebpfless" {
			tm.validateExecSchema(tb, event)
		}
	}
}

func newTestModule(t testing.TB, macroDefs []*rules.MacroDefinition, ruleDefs []*rules.RuleDefinition, fopts ...optFunc) (*testModule, error) {
	return newTestModuleWithOnDemandProbes(t, nil, macroDefs, ruleDefs, fopts...)
}

func newTestModuleWithOnDemandProbes(t testing.TB, onDemandHooks []rules.OnDemandHookPoint, macroDefs []*rules.MacroDefinition, ruleDefs []*rules.RuleDefinition, fopts ...optFunc) (*testModule, error) {
	var opts tmOpts
	for _, opt := range fopts {
		opt(&opts)
	}

	prevEbpfLessEnabled := ebpfLessEnabled
	defer func() {
		ebpfLessEnabled = prevEbpfLessEnabled
	}()
	ebpfLessEnabled = ebpfLessEnabled || opts.staticOpts.ebpfLessEnabled

	if commonCfgDir == "" {
		cd, err := os.MkdirTemp("", "test-cfgdir")
		if err != nil {
			fmt.Println(err)
		}
		commonCfgDir = cd
		os.Chdir(commonCfgDir)
	}

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

	if opts.dynamicOpts.disableBundledRules {
		ruleDefs = append(ruleDefs, &rules.RuleDefinition{
			ID:       bundled.NeedRefreshSBOMRuleID,
			Disabled: true,
			Combine:  rules.OverridePolicy,
		})
	}

	st, err := newSimpleTest(t, macroDefs, ruleDefs, opts.dynamicOpts.testDir)
	if err != nil {
		return nil, err
	}

	if _, err = setTestPolicy(commonCfgDir, onDemandHooks, macroDefs, ruleDefs); err != nil {
		return nil, err
	}

	var cmdWrapper cmdWrapper
	if testEnvironment == DockerEnvironment || ebpfLessEnabled {
		cmdWrapper = newStdCmdWrapper()
	} else {
		wrapper, err := newDockerCmdWrapper(st.Root(), st.Root(), "ubuntu", "")
		if err == nil {
			cmdWrapper = newMultiCmdWrapper(wrapper, newStdCmdWrapper())
		} else {
			// docker not present run only on host
			cmdWrapper = newStdCmdWrapper()
		}
	}

	if testMod != nil && ebpfLessEnabled {
		testMod.st = st
		testMod.cmdWrapper = cmdWrapper
		testMod.t = t
		testMod.opts.dynamicOpts = opts.dynamicOpts
		testMod.opts.staticOpts = opts.staticOpts

		if opts.staticOpts.preStartCallback != nil {
			opts.staticOpts.preStartCallback(testMod)
		}

		if !opts.staticOpts.disableRuntimeSecurity {
			if err = testMod.reloadPolicies(); err != nil {
				return testMod, err
			}
		}
		return testMod, nil

	} else if !opts.forceReload && testMod != nil && opts.staticOpts.Equal(testMod.opts.staticOpts) {
		testMod.st = st
		testMod.cmdWrapper = cmdWrapper
		testMod.t = t
		testMod.opts.dynamicOpts = opts.dynamicOpts

		if !disableTracePipe && !ebpfLessEnabled {
			if testMod.tracePipe, err = testMod.startTracing(); err != nil {
				return testMod, err
			}
		}

		if opts.staticOpts.preStartCallback != nil {
			opts.staticOpts.preStartCallback(testMod)
		}

		if !opts.staticOpts.disableRuntimeSecurity {
			if err = testMod.reloadPolicies(); err != nil {
				return testMod, err
			}
		}

		if ruleDefs != nil && logStatusMetrics {
			t.Logf("%s entry stats: %s", t.Name(), GetEBPFStatusMetrics(testMod.probe))
		}
		return testMod, nil
	} else if testMod != nil {
		testMod.cleanup()
	}

	emconfig, secconfig, err := genTestConfigs(commonCfgDir, opts.staticOpts)
	if err != nil {
		return nil, err
	}

	t.Log("Instantiating a new security module")

	statsdClient := statsdclient.NewStatsdClient()

	testMod = &testModule{
		secconfig:     secconfig,
		opts:          opts,
		st:            st,
		t:             t,
		cmdWrapper:    cmdWrapper,
		statsdClient:  statsdClient,
		proFile:       proFile,
		eventHandlers: eventHandlers{},
	}

	emopts := eventmonitor.Opts{
		StatsdClient: statsdClient,
		ProbeOpts: sprobe.Opts{
			StatsdClient:             statsdClient,
			DontDiscardRuntime:       true,
			PathResolutionEnabled:    true,
			EnvsVarResolutionEnabled: !opts.staticOpts.disableEnvVarsResolution,
			SyscallsMonitorEnabled:   true,
			TTYFallbackEnabled:       true,
			EBPFLessEnabled:          ebpfLessEnabled,
		},
	}
	if opts.staticOpts.tagsResolver != nil {
		emopts.ProbeOpts.TagsResolver = opts.staticOpts.tagsResolver
	} else {
		emopts.ProbeOpts.TagsResolver = NewFakeResolverDifferentImageNames()
	}

	if opts.staticOpts.discardRuntime {
		emopts.ProbeOpts.DontDiscardRuntime = false
	}

	testMod.eventMonitor, err = eventmonitor.NewEventMonitor(emconfig, secconfig, emopts, nil)
	if err != nil {
		return nil, err
	}
	testMod.probe = testMod.eventMonitor.Probe

	var ruleSetloadedErr *multierror.Error
	if !opts.staticOpts.disableRuntimeSecurity {
		msgSender := newFakeMsgSender(testMod)

		cws, err := module.NewCWSConsumer(testMod.eventMonitor, secconfig.RuntimeSecurity, nil, module.Opts{EventSender: testMod, MsgSender: msgSender})
		if err != nil {
			return nil, fmt.Errorf("failed to create module: %w", err)
		}
		// disable containers telemetry
		cws.PrepareForFunctionalTests()

		testMod.cws = cws
		testMod.ruleEngine = cws.GetRuleEngine()
		testMod.msgSender = msgSender

		testMod.eventMonitor.RegisterEventConsumer(cws)

		testMod.ruleEngine.SetRulesetLoadedCallback(func(rs *rules.RuleSet, err *multierror.Error) {
			ruleSetloadedErr = err
			log.Infof("Adding test module as listener")
			rs.AddListener(testMod)
		})
	}

	// listen to probe event
	if err := testMod.probe.AddEventHandler(testMod); err != nil {
		return nil, err
	}

	testMod.probe.AddDiscarderPushedCallback(testMod.NotifyDiscarderPushedCallback)

	if err := testMod.eventMonitor.Init(); err != nil {
		return nil, fmt.Errorf("failed to init module: %w", err)
	}

	if opts.staticOpts.preStartCallback != nil {
		opts.staticOpts.preStartCallback(testMod)
	}

	if !disableTracePipe && !ebpfLessEnabled {
		if testMod.tracePipe, err = testMod.startTracing(); err != nil {
			return nil, err
		}
	}

	if opts.staticOpts.snapshotRuleMatchHandler != nil {
		testMod.RegisterRuleEventHandler(func(e *model.Event, r *rules.Rule) {
			opts.staticOpts.snapshotRuleMatchHandler(testMod, e, r)
		})
		t.Cleanup(func() {
			testMod.RegisterRuleEventHandler(nil)
		})
	}

	if err := testMod.eventMonitor.Start(); err != nil {
		return nil, fmt.Errorf("failed to start module: %w", err)
	}

	if ruleSetloadedErr.ErrorOrNil() != nil {
		testMod.Close()
		return nil, ruleSetloadedErr.ErrorOrNil()
	}

	if logStatusMetrics {
		t.Logf("%s entry stats: %s", t.Name(), GetEBPFStatusMetrics(testMod.probe))
	}

	if ebpfLessEnabled && !opts.staticOpts.dontWaitEBPFLessClient {
		t.Logf("EBPFLess mode, waiting for a client to connect")
		err := retry.Do(func() error {
			if testMod.probe.PlatformProbe.(*sprobe.EBPFLessProbe).GetClientsCount() > 0 {
				return nil
			}
			return errors.New("No client connected, aborting")
		}, retry.Delay(time.Second), retry.Attempts(120))
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Second * 2) // sleep another sec to let tests starting before the tracing is ready
		t.Logf("client connected")
	}
	return testMod, nil
}

// GetEBPFStatusMetrics returns a string representation of the perf buffer monitor metrics
func GetEBPFStatusMetrics(probe *sprobe.Probe) string {
	if probe == nil {
		return ""
	}

	p, ok := probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return ""
	}

	monitors := p.GetMonitors()
	if monitors == nil {
		return ""
	}
	eventStreamMonitor := monitors.GetEventStreamMonitor()
	if eventStreamMonitor == nil {
		return ""
	}

	status := map[string]interface{}{
		"kernel-lost": eventStreamMonitor.GetKernelLostCount("events", -1, model.MaxKernelEventType),
		"per-events":  map[string]interface{}{},
	}

	for i := model.UnknownEventType + 1; i < model.MaxKernelEventType; i++ {
		stats, kernelStats := eventStreamMonitor.GetEventStats(i, "events", -1)
		if stats.Count.Load() == 0 && kernelStats.Count.Load() == 0 && kernelStats.Lost.Load() == 0 {
			continue
		}
		status["per-events"].(map[string]interface{})[i.String()] = map[string]uint64{
			"user":        stats.Count.Load(),
			"kernel":      kernelStats.Count.Load(),
			"kernel-lost": kernelStats.Lost.Load(),
		}
	}
	data, _ := json.Marshal(status)

	var out bytes.Buffer
	_ = json.Indent(&out, data, "", "\t")

	return out.String()
}

//nolint:unused
type tracePipeLogger struct {
	*TracePipe
	stop       chan struct{}
	executable string
	tb         testing.TB
}

//nolint:unused
func (l *tracePipeLogger) handleEvent(event *TraceEvent) {
	// for some reason, the event task is resolved to "<...>"
	// so we check that event.PID is the ID of a task of the running process
	taskPath := utilkernel.HostProc(strconv.Itoa(int(utils.Getpid())), "task", event.PID)
	_, err := os.Stat(taskPath)

	if event.Task == l.executable || (event.Task == "<...>" && err == nil) {
		l.tb.Log(strings.TrimSuffix(event.Raw, "\n"))
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
		tb:         tm.t,
	}
	logger.Start()

	time.Sleep(time.Millisecond * 200)

	return logger, nil
}

func (tm *testModule) cleanup() {
	tm.eventMonitor.Close()
}

func (tm *testModule) validateAbnormalPaths() {
	assert.Zero(tm.t, tm.statsdClient.Get("datadog.runtime_security.rules.rate_limiter.allow:rule_id:abnormal_path"), "abnormal error detected")
}

func (tm *testModule) validateSyscallsInFlight() {
	inflight := tm.statsdClient.GetByPrefix("datadog.runtime_security.syscalls_map.event_inflight:event_type:")
	for key, value := range inflight {
		assert.Greater(tm.t, int64(1024), value, "event type: %s leaked: %d", key, value)
	}
}

func (tm *testModule) Close() {
	if !tm.opts.staticOpts.disableRuntimeSecurity {
		tm.eventMonitor.SendStats()
	}

	if !tm.opts.dynamicOpts.disableAbnormalPathCheck {
		tm.validateAbnormalPaths()
	}

	// make sure we don't leak syscalls
	tm.validateSyscallsInFlight()

	if tm.tracePipe != nil {
		tm.tracePipe.Stop()
		tm.tracePipe = nil
	}

	tm.statsdClient.Flush()

	if tm.msgSender != nil {
		tm.msgSender.flush()
	}

	if logStatusMetrics {
		tm.t.Logf("%s exit stats: %s", tm.t.Name(), GetEBPFStatusMetrics(tm.probe))
	}

	if withProfile {
		pprof.StopCPUProfile()
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

// eventKeyValueFilter is used to filter events in `waitForProbeEvent`
type eventKeyValueFilter struct {
	key   string
	value interface{}
}

// waitForProbeEvent returns the first open event with the provided filename.
// WARNING: this function may yield a "fatal error: concurrent map writes" error if the ruleset of testModule does not
// contain a rule on "open.file.path"
//
//nolint:deadcode,unused
func waitForProbeEvent(test *testModule, action func() error, eventType model.EventType, filters ...eventKeyValueFilter) error {
	return test.GetProbeEvent(action, func(event *model.Event) bool {
		for _, filter := range filters {
			if v, _ := event.GetFieldValue(filter.key); v != filter.value {
				return false
			}
		}
		return true
	}, getEventTimeout, eventType)
}

//nolint:deadcode,unused
func waitForOpenProbeEvent(test *testModule, action func() error, filename string) error {
	return waitForProbeEvent(test, action, model.FileOpenEventType, eventKeyValueFilter{
		key:   "open.file.path",
		value: filename,
	})
}

//nolint:deadcode,unused
func waitForIMDSResponseProbeEvent(test *testModule, action func() error, processFileName string) error {
	return waitForProbeEvent(test, action, model.IMDSEventType, []eventKeyValueFilter{
		{
			key:   "process.file.name",
			value: processFileName,
		},
		{
			key:   "imds.type",
			value: "response",
		},
	}...)
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

func (tm *testModule) StopActivityDump(name, containerID string) error {
	p, ok := tm.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return errors.New("not supported")
	}

	managers := p.GetProfileManagers()
	if managers == nil {
		return errors.New("no manager")
	}
	params := &api.ActivityDumpStopParams{
		Name:        name,
		ContainerID: containerID,
	}
	_, err := managers.StopActivityDump(params)
	if err != nil {
		return err
	}
	return nil
}

type activityDumpIdentifier struct {
	Name        string
	ContainerID string
	Timeout     string
	OutputFiles []string
}

func (tm *testModule) ListActivityDumps() ([]*activityDumpIdentifier, error) {
	p, ok := tm.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return nil, errors.New("not supported")
	}

	managers := p.GetProfileManagers()
	if managers == nil {
		return nil, errors.New("No monitor")
	}
	params := &api.ActivityDumpListParams{}
	mess, err := managers.ListActivityDumps(params)
	if err != nil || mess == nil {
		return nil, err
	}

	var dumps []*activityDumpIdentifier
	for _, dump := range mess.Dumps {
		var files []string
		for _, storage := range dump.Storage {
			if storage.Type == "local_storage" {
				files = append(files, storage.File)
			}
		}
		if len(files) == 0 {
			continue // do not add activity dumps without any local storage files
		}

		dumps = append(dumps, &activityDumpIdentifier{
			Name:        dump.Metadata.Name,
			ContainerID: dump.Metadata.ContainerID,
			Timeout:     dump.Metadata.Timeout,
			OutputFiles: files,
		})
	}
	return dumps, nil
}

func (tm *testModule) DecodeActivityDump(path string) (*dump.ActivityDump, error) {
	p, ok := tm.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return nil, errors.New("not supported")
	}

	managers := p.GetProfileManagers()
	if managers == nil {
		return nil, errors.New("No monitor")
	}

	adm := managers.GetActivityDumpManager()
	if adm == nil {
		return nil, errors.New("No activity dump manager")
	}

	ad := dump.NewActivityDump(adm)
	if ad == nil {
		return nil, errors.New("Creation of new activity dump fails")
	}

	if err := ad.Decode(path); err != nil {
		return nil, err
	}

	return ad, nil
}

// DecodeSecurityProfile decode a security profile
func DecodeSecurityProfile(path string) (*profile.SecurityProfile, error) {
	protoProfile, err := profile.LoadProtoFromFile(path)
	if err != nil {
		return nil, err
	} else if protoProfile == nil {
		return nil, errors.New("Profile parsing error")
	}

	newProfile := profile.NewSecurityProfile(
		cgroupModel.WorkloadSelector{},
		[]model.EventType{model.ExecEventType, model.DNSEventType},
		nil,
	)
	if newProfile == nil {
		return nil, errors.New("Profile creation")
	}
	newProfile.LoadFromProto(protoProfile, profile.LoadOpts{})
	return newProfile, nil
}

func (tm *testModule) StartADocker() (*dockerCmdWrapper, error) {
	// we use alpine to use nslookup on some tests, and validate all busybox specificities
	docker, err := newDockerCmdWrapper(tm.st.Root(), tm.st.Root(), "alpine", "")
	if err != nil {
		return nil, err
	}

	_, err = docker.start()
	if err != nil {
		return nil, err
	}

	time.Sleep(1 * time.Second) // a quick sleep to ensure the dump has started
	return docker, nil
}

func (tm *testModule) GetDumpFromDocker(dockerInstance *dockerCmdWrapper) (*activityDumpIdentifier, error) {
	dumps, err := tm.ListActivityDumps()
	if err != nil {
		return nil, err
	}
	dump := findLearningContainerID(dumps, dockerInstance.containerID)
	if dump == nil {
		return nil, errors.New("ContainerID not found on activity dump list")
	}
	return dump, nil
}

func (tm *testModule) StartADockerGetDump() (*dockerCmdWrapper, *activityDumpIdentifier, error) {
	dockerInstance, err := tm.StartADocker()
	if err != nil {
		return nil, nil, err
	}
	dump, err := tm.GetDumpFromDocker(dockerInstance)
	if err != nil {
		_, _ = dockerInstance.stop()
		return nil, nil, err
	}
	return dockerInstance, dump, nil
}

//nolint:deadcode,unused
func findLearningContainerID(dumps []*activityDumpIdentifier, containerID string) *activityDumpIdentifier {
	for _, dump := range dumps {
		if dump.ContainerID == containerID {
			return dump
		}
	}
	return nil
}

//nolint:deadcode,unused
func findLearningContainerName(dumps []*activityDumpIdentifier, name string) *activityDumpIdentifier {
	for _, dump := range dumps {
		if dump.Name == name {
			return dump
		}
	}
	return nil
}

//nolint:deadcode,unused
func (tm *testModule) isDumpRunning(id *activityDumpIdentifier) bool {
	dumps, err := tm.ListActivityDumps()
	if err != nil {
		return false
	}
	dump := findLearningContainerName(dumps, id.Name)
	return dump != nil
}

//nolint:deadcode,unused
func (tm *testModule) findCgroupDump(id *activityDumpIdentifier) *activityDumpIdentifier {
	dumps, err := tm.ListActivityDumps()
	if err != nil {
		return nil
	}
	dump := findLearningContainerID(dumps, id.ContainerID)
	if dump == nil {
		return nil
	}
	return dump
}

//nolint:deadcode,unused
func (tm *testModule) addAllEventTypesOnDump(dockerInstance *dockerCmdWrapper, syscallTester string, goSyscallTester string) {
	// open
	cmd := dockerInstance.Command("touch", []string{filepath.Join(tm.Root(), "open")}, []string{})
	_, _ = cmd.CombinedOutput()

	// dns
	cmd = dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
	_, _ = cmd.CombinedOutput()

	// bind
	cmd = dockerInstance.Command(syscallTester, []string{"bind", "AF_INET", "any", "tcp"}, []string{})
	_, _ = cmd.CombinedOutput()

	// syscalls should be added with previous events

	// imds
	cmd = dockerInstance.Command(goSyscallTester, []string{"-run-imds-test"}, []string{})
	_, _ = cmd.CombinedOutput()
}

//nolint:deadcode,unused
func (tm *testModule) triggerLoadControllerReducer(_ *dockerCmdWrapper, id *activityDumpIdentifier) {
	p, ok := tm.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return
	}

	managers := p.GetProfileManagers()
	if managers == nil {
		return
	}
	adm := managers.GetActivityDumpManager()
	if adm == nil {
		return
	}
	adm.FakeDumpOverweight(id.Name)

	// wait until the dump learning has stopped
	for tm.isDumpRunning(id) {
		time.Sleep(time.Second * 1)
	}
}

//nolint:deadcode,unused
func (tm *testModule) dockerCreateFiles(dockerInstance *dockerCmdWrapper, syscallTester string, directory string, numberOfFiles int) error {
	var files []string
	for i := 0; i < numberOfFiles; i++ {
		files = append(files, filepath.Join(directory, "ad-test-create-"+fmt.Sprintf("%d", i)))
	}
	args := []string{"sleep", "2", ";", "open"}
	args = append(args, files...)
	cmd := dockerInstance.Command(syscallTester, args, []string{})
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

//nolint:deadcode,unused
func (tm *testModule) findNextPartialDump(dockerInstance *dockerCmdWrapper, id *activityDumpIdentifier) (*activityDumpIdentifier, error) {
	for i := 0; i < 10; i++ { // retry during 5sec
		dump := tm.findCgroupDump(id)
		if dump != nil {
			return dump, nil
		}
		cmd := dockerInstance.Command("echo", []string{"trying to trigger the dump"}, []string{})
		_, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		time.Sleep(time.Second * 1)
	}
	return nil, errors.New("Unable to find the next partial dump")
}

//nolint:deadcode,unused
func searchForOpen(ad *dump.ActivityDump) bool {
	for _, node := range ad.ActivityTree.ProcessNodes {
		if len(node.Files) > 0 {
			return true
		}
	}
	return false
}

//nolint:deadcode,unused
func searchForDNS(ad *dump.ActivityDump) bool {
	for _, node := range ad.ActivityTree.ProcessNodes {
		if len(node.DNSNames) > 0 {
			return true
		}
	}
	return false
}

//nolint:deadcode,unused
func searchForIMDS(ad *dump.ActivityDump) bool {
	for _, node := range ad.ActivityTree.ProcessNodes {
		if len(node.IMDSEvents) > 0 {
			return true
		}
	}
	return false
}

//nolint:deadcode,unused
func searchForBind(ad *dump.ActivityDump) bool {
	for _, node := range ad.ActivityTree.ProcessNodes {
		if len(node.Sockets) > 0 {
			return true
		}
	}
	return false
}

//nolint:deadcode,unused
func searchForSyscalls(ad *dump.ActivityDump) bool {
	for _, node := range ad.ActivityTree.ProcessNodes {
		if len(node.Syscalls) > 0 {
			return true
		}
	}
	return false
}

//nolint:deadcode,unused
func (tm *testModule) getADFromDumpID(id *activityDumpIdentifier) (*dump.ActivityDump, error) {
	var fileProtobuf string
	// decode the dump
	for _, file := range id.OutputFiles {
		if filepath.Ext(file) == ".protobuf" {
			fileProtobuf = file
			break
		}
	}
	if len(fileProtobuf) < 1 {
		return nil, errors.New("protobuf output file not found")
	}
	ad, err := tm.DecodeActivityDump(fileProtobuf)
	if err != nil {
		return nil, err
	}
	return ad, nil
}

//nolint:deadcode,unused
func (tm *testModule) findNumberOfExistingDirectoryFiles(id *activityDumpIdentifier, testDir string) (int, error) {
	ad, err := tm.getADFromDumpID(id)
	if err != nil {
		return 0, err
	}

	var total int
	tempPathParts := strings.Split(testDir, "/")
	lastDir := filepath.Base(testDir)

firstLoop:
	for _, node := range ad.ActivityTree.ProcessNodes {
		current := node.Files
		for _, part := range tempPathParts {
			if part == "" {
				continue
			}
			next, found := current[part]
			if !found {
				continue firstLoop
			}
			current = next.Children
			if part == lastDir {
				total += len(current)
				continue firstLoop
			}
		}
	}
	return total, nil
}

//nolint:deadcode,unused
func (tm *testModule) extractAllDumpEventTypes(id *activityDumpIdentifier) ([]string, error) {
	var res []string

	ad, err := tm.getADFromDumpID(id)
	if err != nil {
		return res, err
	}

	if searchForBind(ad) {
		res = append(res, "bind")
	}
	if searchForDNS(ad) {
		res = append(res, "dns")
	}
	if searchForSyscalls(ad) {
		res = append(res, "syscalls")
	}
	if searchForOpen(ad) {
		res = append(res, "open")
	}
	if searchForIMDS(ad) {
		res = append(res, "imds")
	}
	return res, nil
}

func (tm *testModule) StopAllActivityDumps() error {
	dumps, err := tm.ListActivityDumps()
	if err != nil {
		return err
	}
	if len(dumps) == 0 {
		return nil
	}
	for _, dump := range dumps {
		_ = tm.StopActivityDump(dump.Name, "")
	}
	dumps, err = tm.ListActivityDumps()
	if err != nil {
		return err
	}
	if len(dumps) != 0 {
		return errors.New("Didn't manage to stop all activity dumps")
	}
	return nil
}

// IsDedicatedNodeForAD used only for AD
func IsDedicatedNodeForAD() bool {
	_, present := os.LookupEnv(dedicatedADNodeForTestsEnv)
	return present
}

// ProcessNodeAndParent for test purpose only
type ProcessNodeAndParent struct {
	Node   *activity_tree.ProcessNode
	Parent *ProcessNodeAndParent
}

// NewProcessNodeAndParent for test purpose only
func NewProcessNodeAndParent(node *activity_tree.ProcessNode, parent *ProcessNodeAndParent) *ProcessNodeAndParent {
	return &ProcessNodeAndParent{
		Node:   node,
		Parent: parent,
	}
}

// WalkActivityTree for test purpose only
func WalkActivityTree(at *activity_tree.ActivityTree, walkFunc func(node *ProcessNodeAndParent) bool) []*activity_tree.ProcessNode {
	var result []*activity_tree.ProcessNode
	if len(at.ProcessNodes) == 0 {
		return result
	}
	var nodes []*ProcessNodeAndParent
	var node *ProcessNodeAndParent
	for _, n := range at.ProcessNodes {
		nodes = append(nodes, NewProcessNodeAndParent(n, nil))
	}
	node = nodes[0]
	nodes = nodes[1:]

	for node != nil {
		if walkFunc(node) {
			result = append(result, node.Node)
		}

		for _, child := range node.Node.Children {
			nodes = append(nodes, NewProcessNodeAndParent(child, node))
		}
		if len(nodes) > 0 {
			node = nodes[0]
			nodes = nodes[1:]
		} else {
			node = nil
		}
	}
	return result
}

func (tm *testModule) GetADSelector(dumpID *activityDumpIdentifier) (*cgroupModel.WorkloadSelector, error) {
	ad, err := tm.getADFromDumpID(dumpID)
	if err != nil {
		return nil, err
	}

	selector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", ad.Tags), utils.GetTagValue("image_tag", ad.Tags))
	return &selector, err
}

// NewTimeoutError returns a new timeout error with the metrics collected during the test
func (tm *testModule) NewTimeoutError() ErrTimeout {
	var msg strings.Builder

	msg.WriteString("timeout, details: ")
	msg.WriteString(GetEBPFStatusMetrics(tm.probe))
	msg.WriteString(spew.Sdump(ebpftelemetry.GetProbeStats()))

	events := tm.ruleEngine.StopEventCollector()
	if len(events) != 0 {
		msg.WriteString("\nevents evaluated:\n")

		for _, event := range events {
			msg.WriteString(fmt.Sprintf("%s (eval=%v) {\n", event.Type, event.EvalResult))
			for field, value := range event.Fields {
				msg.WriteString(fmt.Sprintf("\t%s=%v,\n", field, value))
			}
			msg.WriteString("}\n")
		}
	}

	return ErrTimeout{msg.String()}
}

func (tm *testModule) WaitSignals(tb testing.TB, action func() error, cbs ...func(event *model.Event, rule *rules.Rule) error) {
	tb.Helper()

	tm.waitSignal(tb, action, func(event *model.Event, rule *rules.Rule) error {
		validateProcessContext(tb, event)

		return tm.mapFilters(cbs...)(event, rule)
	})

}

func addFakePasswd(user string, uid, gid int32) error {
	file, err := os.OpenFile(fakePasswdPath+"_tmp", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil
	}
	defer file.Close()
	_, err = file.WriteString("root:x:0:0:root:/root:/sbin/nologin\n")
	if err != nil {
		return err
	}
	_, err = file.WriteString(fmt.Sprintf("%s:x:%d:%d:%s:/home/%s:/sbin/nologin\n", user, uid, gid, user, user))
	if err != nil {
		return err
	}
	return os.Rename(fakePasswdPath+"_tmp", fakePasswdPath) // to force the cache refresh
}

func addFakeGroup(group string, gid int32) error {
	file, err := os.OpenFile(fakeGroupPath+"_tmp", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil
	}
	defer file.Close()
	_, err = file.WriteString("root:x:0:\n")
	if err != nil {
		return err
	}
	_, err = file.WriteString(fmt.Sprintf("%s:x:%d:\n", group, gid))
	if err != nil {
		return err
	}
	return os.Rename(fakeGroupPath+"_tmp", fakeGroupPath) // to force the cache refresh
}

func removeFakePasswd() error {
	return os.Remove(fakePasswdPath)
}

func removeFakeGroup() error {
	return os.Remove(fakeGroupPath)
}

func (tm *testModule) ListAllProfiles() {
	p, ok := tm.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return
	}

	m := p.GetProfileManagers()
	if m == nil {
		return
	}

	spm := m.GetSecurityProfileManager()
	if spm == nil {
		return
	}
	spm.ListAllProfileStates()
}

func (tm *testModule) SetProfileVersionState(selector *cgroupModel.WorkloadSelector, imageTag string, state model.EventFilteringProfileState) error {
	p, ok := tm.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return errors.New("no ebpf probe")
	}

	m := p.GetProfileManagers()
	if m == nil {
		return errors.New("no profile managers")
	}

	spm := m.GetSecurityProfileManager()
	if spm == nil {
		return errors.New("no security profile managers")
	}

	profile := spm.GetProfile(*selector)
	if profile == nil {
		return errors.New("no profile")
	}

	err := profile.SetVersionState(imageTag, state)
	if err != nil {
		return err
	}
	return nil
}

func (tm *testModule) GetProfileVersions(imageName string) ([]string, error) {
	p, ok := tm.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return []string{}, errors.New("no ebpf probe")
	}

	m := p.GetProfileManagers()
	if m == nil {
		return []string{}, errors.New("no profile managers")
	}

	spm := m.GetSecurityProfileManager()
	if spm == nil {
		return []string{}, errors.New("no security profile managers")
	}

	profile := spm.GetProfile(cgroupModel.WorkloadSelector{Image: imageName, Tag: "*"})
	if profile == nil {
		return []string{}, errors.New("no profile")
	}

	return profile.GetVersions(), nil
}
