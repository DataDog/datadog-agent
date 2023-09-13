// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests || stresstests

// Package tests holds tests related files
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
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"text/template"
	"time"
	"unsafe"

	"github.com/cihub/seelog"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/go-multierror"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	spconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	rulesmodule "github.com/DataDog/datadog-agent/pkg/security/rules"
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
	//nolint:deadcode,unused
	testSuitePid uint32
)

const (
	getEventTimeout                 = 10 * time.Second
	filelessExecutionFilenamePrefix = "memfd:"
)

type stringSlice []string

const testConfig = `---
log_level: DEBUG
system_probe_config:
  enabled: true
  sysprobe_socket: /tmp/test-sysprobe.sock
  enable_kernel_header_download: true
  enable_runtime_compiler: true

event_monitoring_config:
  socket: /tmp/test-event-monitor.sock
  runtime_compilation:
    enabled: true
  remote_tagger: false
  custom_sensitive_words:
    - "*custom*"
  network:
    enabled: true
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
  remote_configuration:
    enabled: false
  socket: /tmp/test-runtime-security.sock
  sbom:
    enabled: {{ .SBOMEnabled }}
  activity_dump:
    enabled: {{ .EnableActivityDump }}
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
    traced_event_types:   {{range .ActivityDumpTracedEventTypes}}
    - {{.}}
    {{end}}
    local_storage:
      output_directory: {{ .ActivityDumpLocalStorageDirectory }}
      compression: {{ .ActivityDumpLocalStorageCompression }}
      formats: {{range .ActivityDumpLocalStorageFormats}}
      - {{.}}
      {{end}}
{{end}}
  security_profile:
    enabled: {{ .EnableSecurityProfile }}
{{if .EnableSecurityProfile}}
    dir: {{ .SecurityProfileDir }}
    watch_dir: {{ .SecurityProfileWatchDir }}
    anomaly_detection:
      minimum_stable_period: {{.AnomalyDetectionMinimumStablePeriod}}
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
`

const testPolicy = `---
version: 1.2.3

macros:
{{range $Macro := .Macros}}
  - id: {{$Macro.ID}}
    expression: >-
      {{$Macro.Expression}}
{{end}}

rules:
{{range $Rule := .Rules}}
  - id: {{$Rule.ID}}
    version: {{$Rule.Version}}
    expression: >-
      {{$Rule.Expression}}
    tags:
{{- range $Tag, $Val := .Tags}}
      {{$Tag}}: {{$Val}}
{{- end}}
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
	testDir                             string
	disableFilters                      bool
	disableApprovers                    bool
	enableActivityDump                  bool
	activityDumpRateLimiter             int
	activityDumpTagRules                bool
	activityDumpDuration                time.Duration
	activityDumpLoadControllerPeriod    time.Duration
	activityDumpCleanupPeriod           time.Duration
	activityDumpLoadControllerTimeout   time.Duration
	activityDumpTracedCgroupsCount      int
	activityDumpTracedEventTypes        []string
	activityDumpLocalStorageDirectory   string
	activityDumpLocalStorageCompression bool
	activityDumpLocalStorageFormats     []string
	enableSecurityProfile               bool
	securityProfileDir                  string
	securityProfileWatchDir             bool
	anomalyDetectionMinimumStablePeriod time.Duration
	anomalyDetectionWarmupPeriod        time.Duration
	disableDiscarders                   bool
	eventsCountThreshold                int
	disableERPCDentryResolution         bool
	disableMapDentryResolution          bool
	envsWithValue                       []string
	disableAbnormalPathCheck            bool
	disableRuntimeSecurity              bool
	enableSBOM                          bool
	preStartCallback                    func(test *testModule)
	tagsResolver                        tags.Resolver
	snapshotRuleMatchHandler            func(*testModule, *model.Event, *rules.Rule)
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
		to.activityDumpTagRules == opts.activityDumpTagRules &&
		to.activityDumpDuration == opts.activityDumpDuration &&
		to.activityDumpLoadControllerPeriod == opts.activityDumpLoadControllerPeriod &&
		to.activityDumpTracedCgroupsCount == opts.activityDumpTracedCgroupsCount &&
		to.activityDumpLoadControllerTimeout == opts.activityDumpLoadControllerTimeout &&
		reflect.DeepEqual(to.activityDumpTracedEventTypes, opts.activityDumpTracedEventTypes) &&
		to.activityDumpLocalStorageDirectory == opts.activityDumpLocalStorageDirectory &&
		to.activityDumpLocalStorageCompression == opts.activityDumpLocalStorageCompression &&
		reflect.DeepEqual(to.activityDumpLocalStorageFormats, opts.activityDumpLocalStorageFormats) &&
		to.enableSecurityProfile == opts.enableSecurityProfile &&
		to.securityProfileDir == opts.securityProfileDir &&
		to.securityProfileWatchDir == opts.securityProfileWatchDir &&
		to.anomalyDetectionMinimumStablePeriod == opts.anomalyDetectionMinimumStablePeriod &&
		to.anomalyDetectionWarmupPeriod == opts.anomalyDetectionWarmupPeriod &&
		to.disableDiscarders == opts.disableDiscarders &&
		to.disableFilters == opts.disableFilters &&
		to.eventsCountThreshold == opts.eventsCountThreshold &&
		to.disableERPCDentryResolution == opts.disableERPCDentryResolution &&
		to.disableMapDentryResolution == opts.disableMapDentryResolution &&
		reflect.DeepEqual(to.envsWithValue, opts.envsWithValue) &&
		to.disableAbnormalPathCheck == opts.disableAbnormalPathCheck &&
		to.disableRuntimeSecurity == opts.disableRuntimeSecurity &&
		to.enableSBOM == opts.enableSBOM &&
		to.snapshotRuleMatchHandler == nil && opts.snapshotRuleMatchHandler == nil
}

type testModule struct {
	sync.RWMutex
	secconfig     *secconfig.Config
	opts          testOpts
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
}

var testMod *testModule

type onRuleHandler func(*model.Event, *rules.Rule)
type onProbeEventHandler func(*model.Event)
type onCustomSendEventHandler func(*rules.Rule, *events.CustomEvent)
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
func assertTriggeredRule(tb testing.TB, r *rules.Rule, id string) bool {
	tb.Helper()
	return assert.Equal(tb, id, r.ID, "wrong triggered rule")
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
func assertFieldEqual(tb testing.TB, e *model.Event, field string, value interface{}, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	return assert.Equal(tb, value, fieldValue, msgAndArgs...)
}

//nolint:deadcode,unused
func assertFieldNotEqual(tb testing.TB, e *model.Event, field string, value interface{}, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	return assert.NotEqual(tb, value, fieldValue, msgAndArgs...)
}

//nolint:deadcode,unused
func assertFieldNotEmpty(tb testing.TB, e *model.Event, field string, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	return assert.NotEmpty(tb, fieldValue, msgAndArgs...)
}

//nolint:deadcode,unused
func assertFieldContains(tb testing.TB, e *model.Event, field string, value interface{}, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	return assert.Contains(tb, fieldValue, value, msgAndArgs...)
}

//nolint:deadcode,unused
func assertFieldIsOneOf(tb testing.TB, e *model.Event, field string, possibleValues interface{}, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	return assert.Contains(tb, possibleValues, fieldValue, msgAndArgs...)
}

//nolint:deadcode,unused
func assertFieldStringArrayIndexedOneOf(tb *testing.T, e *model.Event, field string, index int, values []string, msgAndArgs ...interface{}) bool {
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
func validateProcessContextLineage(tb testing.TB, event *model.Event, probe *sprobe.Probe) {
	eventJSON, err := serializers.MarshalEvent(event, probe.GetResolvers())
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
		tb.Errorf("should have a process context with ancestors, got %+v (%s)", json, spew.Sdump(data))
		tb.Error(string(eventJSON))
		return
	}

	var prevPID, prevPPID float64

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
		prevPID = pid

		if pid != 1 {
			ppid, ok := pce["ppid"].(float64)
			if !ok {
				tb.Errorf("invalid pid, %+v", pce)
				tb.Error(string(eventJSON))
				return
			}

			prevPPID = ppid
		}
	}

	if prevPID != 1 {
		tb.Errorf("invalid process tree, last ancestor should be pid 1, %+v", json)
		tb.Error(string(eventJSON))
	}
}

//nolint:deadcode,unused
func validateProcessContextSECL(tb testing.TB, event *model.Event, probe *sprobe.Probe) {
	// Process file name values cannot be blank
	nameFields := []string{
		"process.file.name",
		"process.ancestors.file.name",
		"process.parent.file.path",
		"process.parent.file.name",
	}

	nameFieldValid, hasPath := checkProcessContextFieldsForBlankValues(tb, event, nameFields)

	// Process path values can be blank if the process was a fileless execution
	pathFields := []string{
		"process.file.path",
		"process.ancestors.file.path",
	}

	pathFieldValid := true
	if hasPath {
		pathFieldValid, _ = checkProcessContextFieldsForBlankValues(tb, event, pathFields)
	}

	valid := nameFieldValid && pathFieldValid

	if !valid {
		eventJSON, err := serializers.MarshalEvent(event, probe.GetResolvers())
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
func validateProcessContext(tb testing.TB, event *model.Event, probe *sprobe.Probe) {
	if event.ProcessContext.IsKworker {
		return
	}

	validateProcessContextLineage(tb, event, probe)
	validateProcessContextSECL(tb, event, probe)
}

//nolint:deadcode,unused
func validateEvent(tb testing.TB, validate func(event *model.Event, rule *rules.Rule), probe *sprobe.Probe) func(event *model.Event, rule *rules.Rule) {
	return func(event *model.Event, rule *rules.Rule) {
		validateProcessContext(tb, event, probe)
		validate(event, rule)
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

		tm.validateExecSchema(tb, event)
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

func genTestConfigs(dir string, opts testOpts, testDir string) (*emconfig.Config, *secconfig.Config, error) {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		return nil, nil, err
	}

	if opts.eventsCountThreshold == 0 {
		opts.eventsCountThreshold = 100000000
	}

	if opts.activityDumpRateLimiter == 0 {
		opts.activityDumpRateLimiter = 500
	}

	if opts.activityDumpTracedCgroupsCount == 0 {
		opts.activityDumpTracedCgroupsCount = 5
	}

	if opts.activityDumpDuration == 0 {
		opts.activityDumpDuration = testActivityDumpDuration
	}

	if len(opts.activityDumpTracedEventTypes) == 0 {
		opts.activityDumpTracedEventTypes = []string{"exec", "open", "bind", "dns", "syscalls"}
	}

	if opts.activityDumpLocalStorageDirectory == "" {
		opts.activityDumpLocalStorageDirectory = "/tmp/activity_dumps"
	}

	if opts.securityProfileDir == "" {
		opts.securityProfileDir = "/tmp/activity_dumps/profiles"
	}

	erpcDentryResolutionEnabled := true
	if opts.disableERPCDentryResolution {
		erpcDentryResolutionEnabled = false
	}

	mapDentryResolutionEnabled := true
	if opts.disableMapDentryResolution {
		mapDentryResolutionEnabled = false
	}

	runtimeSecurityEnabled := true
	if opts.disableRuntimeSecurity {
		runtimeSecurityEnabled = false
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPoliciesDir":                     dir,
		"DisableApprovers":                    opts.disableApprovers,
		"DisableDiscarders":                   opts.disableDiscarders,
		"EnableActivityDump":                  opts.enableActivityDump,
		"ActivityDumpRateLimiter":             opts.activityDumpRateLimiter,
		"ActivityDumpTagRules":                opts.activityDumpTagRules,
		"ActivityDumpDuration":                opts.activityDumpDuration,
		"ActivityDumpLoadControllerPeriod":    opts.activityDumpLoadControllerPeriod,
		"ActivityDumpLoadControllerTimeout":   opts.activityDumpLoadControllerTimeout,
		"ActivityDumpCleanupPeriod":           opts.activityDumpCleanupPeriod,
		"ActivityDumpTracedCgroupsCount":      opts.activityDumpTracedCgroupsCount,
		"ActivityDumpTracedEventTypes":        opts.activityDumpTracedEventTypes,
		"ActivityDumpLocalStorageDirectory":   opts.activityDumpLocalStorageDirectory,
		"ActivityDumpLocalStorageCompression": opts.activityDumpLocalStorageCompression,
		"ActivityDumpLocalStorageFormats":     opts.activityDumpLocalStorageFormats,
		"EnableSecurityProfile":               opts.enableSecurityProfile,
		"SecurityProfileDir":                  opts.securityProfileDir,
		"SecurityProfileWatchDir":             opts.securityProfileWatchDir,
		"AnomalyDetectionMinimumStablePeriod": opts.anomalyDetectionMinimumStablePeriod,
		"AnomalyDetectionWarmupPeriod":        opts.anomalyDetectionWarmupPeriod,
		"EventsCountThreshold":                opts.eventsCountThreshold,
		"ErpcDentryResolutionEnabled":         erpcDentryResolutionEnabled,
		"MapDentryResolutionEnabled":          mapDentryResolutionEnabled,
		"LogPatterns":                         logPatterns,
		"LogTags":                             logTags,
		"EnvsWithValue":                       opts.envsWithValue,
		"RuntimeSecurityEnabled":              runtimeSecurityEnabled,
		"SBOMEnabled":                         opts.enableSBOM,
	}); err != nil {
		return nil, nil, err
	}

	ddConfigName, sysprobeConfigName, err := func() (string, string, error) {
		ddConfig, err := os.OpenFile(path.Join(testDir, "datadog.yaml"), os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return "", "", err
		}
		defer ddConfig.Close()

		sysprobeConfig, err := os.Create(path.Join(testDir, "system-probe.yaml"))
		if err != nil {
			return "", "", err
		}
		defer sysprobeConfig.Close()

		_, err = io.Copy(sysprobeConfig, buffer)
		if err != nil {
			return "", "", err
		}
		return ddConfig.Name(), sysprobeConfig.Name(), nil
	}()
	if err != nil {
		return nil, nil, err
	}

	err = spconfig.SetupOptionalDatadogConfigWithDir(testDir, ddConfigName)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to set up datadog.yaml configuration: %s", err)
	}

	spconfig, err := spconfig.New(sysprobeConfigName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	emconfig := emconfig.NewConfig(spconfig)

	secconfig, err := secconfig.NewConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	secconfig.Probe.ERPCDentryResolutionEnabled = !opts.disableERPCDentryResolution
	secconfig.Probe.MapDentryResolutionEnabled = !opts.disableMapDentryResolution

	return emconfig, secconfig, nil
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

	if _, err = setTestPolicy(st.root, macroDefs, ruleDefs); err != nil {
		return nil, err
	}

	var cmdWrapper cmdWrapper
	if testEnvironment == DockerEnvironment {
		cmdWrapper = newStdCmdWrapper()
	} else {
		wrapper, err := newDockerCmdWrapper(st.Root(), st.Root(), "ubuntu")
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
		if testMod.tracePipe, err = testMod.startTracing(); err != nil {
			return testMod, err
		}

		if opts.preStartCallback != nil {
			opts.preStartCallback(testMod)
		}

		if !opts.disableRuntimeSecurity {
			if err = testMod.reloadPolicies(); err != nil {
				return testMod, err
			}
		}

		if ruleDefs != nil && logStatusMetrics {
			t.Logf("%s entry stats: %s\n", t.Name(), GetStatusMetrics(testMod.probe))
		}
		return testMod, nil
	} else if testMod != nil {
		testMod.cleanup()
	}

	emconfig, secconfig, err := genTestConfigs(st.root, opts, st.root)
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
		ProbeOpts: probe.Opts{
			StatsdClient:              statsdClient,
			DontDiscardRuntime:        true,
			PathResolutionEnabled:     true,
			SyscallsMapMonitorEnabled: true,
		},
	}
	if opts.tagsResolver != nil {
		emopts.ProbeOpts.TagsResolver = opts.tagsResolver
	} else {
		emopts.ProbeOpts.TagsResolver = NewFakeResolver()
	}
	testMod.eventMonitor, err = eventmonitor.NewEventMonitor(emconfig, secconfig, emopts)
	if err != nil {
		return nil, err
	}
	testMod.probe = testMod.eventMonitor.Probe

	var ruleSetloadedErr *multierror.Error
	if !opts.disableRuntimeSecurity {
		cws, err := module.NewCWSConsumer(testMod.eventMonitor, secconfig.RuntimeSecurity, module.Opts{EventSender: testMod})
		if err != nil {
			return nil, fmt.Errorf("failed to create module: %w", err)
		}
		testMod.cws = cws
		testMod.ruleEngine = cws.GetRuleEngine()

		testMod.eventMonitor.RegisterEventConsumer(cws)

		testMod.ruleEngine.SetRulesetLoadedCallback(func(es *rules.EvaluationSet, err *multierror.Error) {
			ruleSetloadedErr = err
			log.Infof("Adding test module as listener")
			for _, ruleSet := range es.RuleSets {
				ruleSet.AddListener(testMod)
			}
		})
	}

	// listen to probe event
	if err := testMod.probe.AddEventHandler(model.UnknownEventType, testMod); err != nil {
		return nil, err
	}

	testMod.probe.AddNewNotifyDiscarderPushedCallback(testMod.NotifyDiscarderPushedCallback)

	if err := testMod.eventMonitor.Init(); err != nil {
		return nil, fmt.Errorf("failed to init module: %w", err)
	}

	kv, _ := kernel.NewKernelVersion()

	if os.Getenv("DD_TESTS_RUNTIME_COMPILED") == "1" && secconfig.Probe.RuntimeCompilationEnabled && !testMod.eventMonitor.Probe.IsRuntimeCompiled() && !kv.IsSuseKernel() {
		return nil, errors.New("failed to runtime compile module")
	}

	if opts.preStartCallback != nil {
		opts.preStartCallback(testMod)
	}

	if testMod.tracePipe, err = testMod.startTracing(); err != nil {
		return nil, err
	}

	if opts.snapshotRuleMatchHandler != nil {
		testMod.RegisterRuleEventHandler(func(e *model.Event, r *rules.Rule) {
			opts.snapshotRuleMatchHandler(testMod, e, r)
		})
		defer testMod.RegisterRuleEventHandler(nil)
	}
	if err := testMod.eventMonitor.Start(); err != nil {
		return nil, fmt.Errorf("failed to start module: %w", err)
	}

	if ruleSetloadedErr.ErrorOrNil() != nil {
		defer testMod.Close()
		return nil, ruleSetloadedErr.ErrorOrNil()
	}

	if logStatusMetrics {
		t.Logf("%s entry stats: %s\n", t.Name(), GetStatusMetrics(testMod.probe))
	}

	return testMod, nil
}

func (tm *testModule) HandleEvent(event *model.Event) {
	tm.eventHandlers.RLock()
	defer tm.eventHandlers.RUnlock()

	if tm.eventHandlers.onProbeEvent != nil {
		tm.eventHandlers.onProbeEvent(event)
	}
}

func (tm *testModule) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {}

func (tm *testModule) SendEvent(rule *rules.Rule, event events.Event, extTagsCb func() []string, service string) {
	tm.eventHandlers.RLock()
	defer tm.eventHandlers.RUnlock()

	switch ev := event.(type) {
	case *events.CustomEvent:
		if tm.eventHandlers.onCustomSendEvent != nil {
			tm.eventHandlers.onCustomSendEvent(rule, ev)
		}
	}
}

func (tm *testModule) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	tm.cmdWrapper.Run(t, name, fnc)
}

func (tm *testModule) reloadPolicies() error {
	log.Debugf("reload policies with testDir: %s", tm.Root())
	policiesDir := tm.Root()

	provider, err := rules.NewPoliciesDirProvider(policiesDir, false)
	if err != nil {
		return err
	}

	if err := tm.ruleEngine.LoadPolicies([]rules.PolicyProvider{provider}, true); err != nil {
		return fmt.Errorf("failed to reload test module: %w", err)
	}

	return nil
}

func (tm *testModule) Root() string {
	return tm.st.root
}

func (tm *testModule) RuleMatch(rule *rules.Rule, event eval.Event) bool {
	tm.eventHandlers.RLock()
	callback := tm.eventHandlers.onRuleMatch
	tm.eventHandlers.RUnlock()

	if callback != nil {
		callback(event.(*model.Event), rule)
	}

	return true
}

func (tm *testModule) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
}

func (tm *testModule) RegisterDiscarderPushedHandler(cb onDiscarderPushedHandler) {
	tm.eventHandlers.Lock()
	tm.eventHandlers.onDiscarderPushed = cb
	tm.eventHandlers.Unlock()
}

func (tm *testModule) NotifyDiscarderPushedCallback(eventType string, event *model.Event, field string) {
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
		return tm.NewTimeoutError()
	case <-ctx.Done():
		return nil
	}
}

//nolint:deadcode,unused
func (tm *testModule) marshalEvent(ev *model.Event) (string, error) {
	b, err := serializers.MarshalEvent(ev, tm.probe.GetResolvers())
	return string(b), err
}

//nolint:deadcode,unused
func (tm *testModule) debugEvent(ev *model.Event) string {
	b, err := tm.marshalEvent(ev)
	if err != nil {
		return err.Error()
	}
	return string(b)
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
	eventStreamMonitor := monitor.GetEventStreamMonitor()
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

// ErrTimeout is used to indicate that a test timed out
type ErrTimeout struct {
	msg string
}

func (et ErrTimeout) Error() string {
	return et.msg
}

// NewTimeoutError returns a new timeout error with the metrics collected during the test
func (tm *testModule) NewTimeoutError() ErrTimeout {
	var msg strings.Builder

	msg.WriteString("timeout, details: ")
	msg.WriteString(GetStatusMetrics(tm.probe))
	msg.WriteString(spew.Sdump(ddebpf.GetProbeStats()))

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

	if err := tm.GetSignal(tb, action, validateEvent(tb, cb, tm.probe)); err != nil {
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

	tm.RegisterRuleEventHandler(func(e *model.Event, r *rules.Rule) {
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
		return tm.NewTimeoutError()
	case <-ctx.Done():
		return nil
	}
}

func (tm *testModule) RegisterRuleEventHandler(cb onRuleHandler) {
	tm.eventHandlers.Lock()
	tm.eventHandlers.onRuleMatch = cb
	tm.eventHandlers.Unlock()
}

func (tm *testModule) GetCustomEventSent(tb testing.TB, action func() error, cb func(rule *rules.Rule, event *events.CustomEvent) bool, timeout time.Duration, eventType model.EventType) error {
	tb.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	message := make(chan ActionMessage, 1)

	tm.RegisterCustomSendEventHandler(func(rule *rules.Rule, event *events.CustomEvent) {
		if event.GetEventType() != eventType {
			return
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
	case <-time.After(timeout):
		return tm.NewTimeoutError()
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

func (tm *testModule) GetProbeEvent(action func() error, cb func(event *model.Event) bool, timeout time.Duration, eventTypes ...model.EventType) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	message := make(chan ActionMessage, 1)

	tm.RegisterProbeEventHandler(func(event *model.Event) {
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
		return tm.NewTimeoutError()
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
	if !tm.opts.disableRuntimeSecurity {
		tm.eventMonitor.SendStats()
	}

	if !tm.opts.disableAbnormalPathCheck {
		tm.validateAbnormalPaths()
	}

	// make sure we don't leak syscalls
	tm.validateSyscallsInFlight()

	if tm.tracePipe != nil {
		tm.tracePipe.Stop()
		tm.tracePipe = nil
	}

	tm.statsdClient.Flush()

	if logStatusMetrics {
		tm.t.Logf("%s exit stats: %s\n", tm.t.Name(), GetStatusMetrics(tm.probe))
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
//
//nolint:deadcode,unused
func waitForProbeEvent(test *testModule, action func() error, key string, value interface{}, eventType model.EventType) error {
	return test.GetProbeEvent(action, func(event *model.Event) bool {
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
	flag.StringVar(&testEnvironment, "env", HostEnvironment, "environment used to run the test suite: ex: host, docker")
	flag.StringVar(&logLevelStr, "loglevel", seelog.WarnStr, "log level")
	flag.Var(&logPatterns, "logpattern", "List of log pattern")
	flag.Var(&logTags, "logtag", "List of log tag")
	flag.BoolVar(&logStatusMetrics, "status-metrics", false, "display status metrics")
	flag.BoolVar(&withProfile, "with-profile", false, "enable profile per test")

	rand.Seed(time.Now().UnixNano())

	testSuitePid = utils.Getpid()
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

func (tm *testModule) StartActivityDumpComm(comm string, outputDir string, formats []string) ([]string, error) {
	managers := tm.probe.GetProfileManagers()
	if managers == nil {
		return nil, errors.New("No monitor")
	}
	p := &api.ActivityDumpParams{
		Comm:              comm,
		Timeout:           "1m",
		DifferentiateArgs: true,
		Storage: &api.StorageRequestParams{
			LocalStorageDirectory:    outputDir,
			LocalStorageFormats:      formats,
			LocalStorageCompression:  false,
			RemoteStorageFormats:     []string{},
			RemoteStorageCompression: false,
		},
	}
	mess, err := managers.DumpActivity(p)
	if err != nil || mess == nil || len(mess.Storage) < 1 {
		return nil, fmt.Errorf("failed to start activity dump: err:%v message:%v len:%v", err, mess, len(mess.Storage))
	}

	var files []string
	for _, s := range mess.Storage {
		files = append(files, s.File)
	}
	return files, nil
}

func (tm *testModule) StopActivityDump(name, containerID, comm string) error {
	managers := tm.probe.GetProfileManagers()
	if managers == nil {
		return errors.New("No monitor")
	}
	p := &api.ActivityDumpStopParams{
		Name:        name,
		ContainerID: containerID,
		Comm:        comm,
	}
	_, err := managers.StopActivityDump(p)
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
	managers := tm.probe.GetProfileManagers()
	if managers == nil {
		return nil, errors.New("No monitor")
	}
	p := &api.ActivityDumpListParams{}
	mess, err := managers.ListActivityDumps(p)
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
	managers := tm.probe.GetProfileManagers()
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

func DecodeSecurityProfile(path string) (*profile.SecurityProfile, error) {
	protoProfile, err := profile.LoadProfileFromFile(path)
	if err != nil {
		return nil, err
	} else if protoProfile == nil {
		return nil, errors.New("Profile parsing error")
	}

	newProfile := profile.NewSecurityProfile(cgroupModel.WorkloadSelector{},
		[]model.EventType{
			model.ExecEventType,
			model.DNSEventType,
		})
	if newProfile == nil {
		return nil, errors.New("Profile creation")
	}
	profile.ProtoToSecurityProfile(newProfile, nil, protoProfile)
	return newProfile, nil
}

func (tm *testModule) StartADocker() (*dockerCmdWrapper, error) {
	// we use alpine to use nslookup on some tests, and validate all busybox specificities
	docker, err := newDockerCmdWrapper(tm.st.Root(), tm.st.Root(), "alpine")
	if err != nil {
		return nil, err
	}

	_, err = docker.start()
	if err != nil {
		return nil, err
	}

	return docker, nil
}

func (tm *testModule) StartADockerGetDump() (*dockerCmdWrapper, *activityDumpIdentifier, error) {
	dockerInstance, err := tm.StartADocker()
	if err != nil {
		return nil, nil, err
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump
	dumps, err := tm.ListActivityDumps()
	if err != nil {
		_, _ = dockerInstance.stop()
		return nil, nil, err
	}
	dump := findLearningContainerID(dumps, dockerInstance.containerID)
	if dump == nil {
		_, _ = dockerInstance.stop()
		return nil, nil, errors.New("ContainerID not found on activity dump list")
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
	if dump == nil {
		return false
	}
	return true
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
func (tm *testModule) addAllEventTypesOnDump(dockerInstance *dockerCmdWrapper, id *activityDumpIdentifier, syscallTester string) {
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
}

//nolint:deadcode,unused
func (tm *testModule) triggerLoadControllerReducer(dockerInstance *dockerCmdWrapper, id *activityDumpIdentifier) {
	managers := tm.probe.GetProfileManagers()
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
func searchForDns(ad *dump.ActivityDump) bool {
	for _, node := range ad.ActivityTree.ProcessNodes {
		if len(node.DNSNames) > 0 {
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
func (tm *testModule) getADFromDumpId(id *activityDumpIdentifier) (*dump.ActivityDump, error) {
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
	ad, err := tm.getADFromDumpId(id)
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

	ad, err := tm.getADFromDumpId(id)
	if err != nil {
		return res, err
	}

	if searchForBind(ad) {
		res = append(res, "bind")
	}
	if searchForDns(ad) {
		res = append(res, "dns")
	}
	if searchForSyscalls(ad) {
		res = append(res, "syscalls")
	}
	if searchForOpen(ad) {
		res = append(res, "open")
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
		_ = tm.StopActivityDump(dump.Name, "", "")
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

func IsDedicatedNode(env string) bool {
	_, present := os.LookupEnv(env)
	return present
}

// for test purpose only
type ProcessNodeAndParent struct {
	Node   *activity_tree.ProcessNode
	Parent *ProcessNodeAndParent
}

// for test purpose only
func NewProcessNodeAndParent(node *activity_tree.ProcessNode, parent *ProcessNodeAndParent) *ProcessNodeAndParent {
	return &ProcessNodeAndParent{
		Node:   node,
		Parent: parent,
	}
}

// for test purpose only
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
	ad, err := tm.getADFromDumpId(dumpID)
	if err != nil {
		return nil, err
	}

	selector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", ad.Tags), utils.GetTagValue("image_tag", ad.Tags))
	return &selector, err
}

func (tm *testModule) SetProfileStatus(selector *cgroupModel.WorkloadSelector, newStatus model.Status) error {
	managers := tm.probe.GetProfileManagers()
	if managers == nil {
		return errors.New("No monitor")
	}

	spm := managers.GetSecurityProfileManager()
	if spm == nil {
		return errors.New("No security profile manager")
	}

	profile := spm.GetProfile(*selector)
	if profile == nil || profile.Status == 0 {
		return errors.New("No profile found for given selector")
	}

	profile.Lock()
	profile.Status = newStatus
	profile.Unlock()
	return nil
}
