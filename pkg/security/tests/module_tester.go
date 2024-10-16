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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"
	"unsafe"

	"gopkg.in/yaml.v3"

	spconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"

	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/rules/bundled"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/stretchr/testify/assert"
)

// ActionMessage is used to send a message from an action function to its callback
type ActionMessage int
type stringSlice []string

const (
	// Continue means that the callback should execute normally
	Continue ActionMessage = iota
	// Skip means that the callback should skip the test
	Skip
)
const (
	getEventTimeout                 = 10 * time.Second
	filelessExecutionFilenamePrefix = "memfd:"
)

var (
	errSkipEvent = errors.New("skip event")
)

func (s *stringSlice) String() string {
	return strings.Join(*s, " ")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (tm *testModule) HandleEvent(event *model.Event) {
	tm.eventHandlers.RLock()
	defer tm.eventHandlers.RUnlock()

	if tm.eventHandlers.onProbeEvent != nil {
		tm.eventHandlers.onProbeEvent(event)
	}
}

func (tm *testModule) HandleCustomEvent(_ *rules.Rule, _ *events.CustomEvent) {}

func (tm *testModule) SendEvent(rule *rules.Rule, event events.Event, extTagsCb func() []string, service string) {
	tm.eventHandlers.RLock()
	defer tm.eventHandlers.RUnlock()

	// forward to the API server
	if tm.cws != nil {
		tm.cws.APIServer().SendEvent(rule, event, extTagsCb, service)
	}

	switch ev := event.(type) {
	case *events.CustomEvent:
		if tm.eventHandlers.onCustomSendEvent != nil {
			tm.eventHandlers.onCustomSendEvent(rule, ev)
		}
	case *model.Event:
		if tm.eventHandlers.onSendEvent != nil {
			tm.eventHandlers.onSendEvent(rule, ev)
		}
	}
}

func (tm *testModule) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	tm.cmdWrapper.Run(t, name, fnc)
}

func (tm *testModule) reloadPolicies() error {
	log.Debugf("reload policies with cfgDir: %s", commonCfgDir)

	bundledPolicyProvider := bundled.NewPolicyProvider(tm.eventMonitor.Probe.Config.RuntimeSecurity)
	policyDirProvider, err := rules.NewPoliciesDirProvider(commonCfgDir, false)
	if err != nil {
		return err
	}

	if err := tm.ruleEngine.LoadPolicies([]rules.PolicyProvider{bundledPolicyProvider, policyDirProvider}, true); err != nil {
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

func (tm *testModule) EventDiscarderFound(_ *rules.RuleSet, _ eval.Event, _ eval.Field, _ eval.EventType) {
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

// ErrTimeout is used to indicate that a test timed out
type ErrTimeout struct {
	msg string
}

func (et ErrTimeout) Error() string {
	return et.msg
}

// ErrSkipTest is used to notify that a test should be skipped
type ErrSkipTest struct {
	msg string
}

func (err ErrSkipTest) Error() string {
	return err.msg
}

func (tm *testModule) mapFilters(filters ...func(event *model.Event, rule *rules.Rule) error) func(event *model.Event, rule *rules.Rule) error {
	return func(event *model.Event, rule *rules.Rule) error {
		for _, filter := range filters {
			if err := filter(event, rule); err != nil {
				return err
			}
		}
		return nil
	}
}

func (tm *testModule) waitSignal(tb testing.TB, action func() error, cb func(*model.Event, *rules.Rule) error) {
	tb.Helper()

	if err := tm.getSignal(tb, action, cb); err != nil {
		if _, ok := err.(ErrSkipTest); ok {
			tb.Skip(err)
		} else {
			tb.Fatal(err)
		}
	}
}

func (tm *testModule) GetSignal(tb testing.TB, action func() error, cb onRuleHandler) error {
	return tm.getSignal(tb, action, func(event *model.Event, rule *rules.Rule) error {
		cb(event, rule)
		return nil
	})
}

func (tm *testModule) getSignal(tb testing.TB, action func() error, cb func(*model.Event, *rules.Rule) error) error {
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
				if err := cb(e, r); err != nil {
					if errors.Is(err, errSkipEvent) {
						message <- Continue
						return
					}
					tb.Error(err)
				}
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

func (tm *testModule) GetEventSent(tb testing.TB, action func() error, cb func(rule *rules.Rule, event *model.Event) bool, timeout time.Duration, ruleID eval.RuleID) error {
	tb.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	message := make(chan ActionMessage, 1)

	tm.RegisterSendEventHandler(func(rule *rules.Rule, event *model.Event) {
		if rule.ID != ruleID {
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
	defer tm.RegisterSendEventHandler(nil)

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

func (tm *testModule) RegisterSendEventHandler(cb onSendEventHandler) {
	tm.eventHandlers.Lock()
	tm.eventHandlers.onSendEvent = cb
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

func (tm *testModule) WaitSignal(tb testing.TB, action func() error, cb onRuleHandler) {
	tb.Helper()

	tm.waitSignal(tb, action, func(event *model.Event, rule *rules.Rule) error {
		validateProcessContext(tb, event)
		cb(event, rule)
		return nil
	})
}

//nolint:deadcode,unused
func (tm *testModule) marshalEvent(ev *model.Event) (string, error) {
	b, err := serializers.MarshalEvent(ev, nil)
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

//nolint:deadcode,unused
func assertTriggeredRule(tb testing.TB, r *rules.Rule, id string) bool {
	tb.Helper()
	return assert.Equal(tb, id, r.ID, "wrong triggered rule")
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
func assertFieldEqualCaseInsensitve(tb testing.TB, e *model.Event, field string, value interface{}, msgAndArgs ...interface{}) bool {
	tb.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		tb.Errorf("failed to get field '%s': %s", field, err)
		return false
	}
	if reflect.TypeOf(value).Kind() == reflect.String {
		value = strings.ToLower(value.(string))
	}
	if reflect.TypeOf(fieldValue).Kind() == reflect.String {
		fieldValue = strings.ToLower(fieldValue.(string))
	}
	eq := assert.Equal(tb, value, fieldValue, msgAndArgs...)
	if !eq {
		tb.Logf("expected value %s\n", value.(string))
		tb.Logf("actual value %s\n", fieldValue.(string))
	}
	return eq
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

	tb.Errorf("failed to get field '%s' as an array: %v", field, msgAndArgs)
	return false
}

func setTestPolicy(dir string, onDemandProbes []rules.OnDemandHookPoint, macroDefs []*rules.MacroDefinition, ruleDefs []*rules.RuleDefinition) (string, error) {
	testPolicyFile, err := os.Create(path.Join(dir, "secagent-policy.policy"))
	if err != nil {
		return "", err
	}

	fail := func(err error) error {
		os.Remove(testPolicyFile.Name())
		return err
	}

	policyDef := &rules.PolicyDef{
		Version:            "1.2.3",
		Macros:             macroDefs,
		Rules:              ruleDefs,
		OnDemandHookPoints: onDemandProbes,
	}

	testPolicy, err := yaml.Marshal(policyDef)
	if err != nil {
		return "", fail(err)
	}

	_, err = testPolicyFile.Write(testPolicy)
	if err != nil {
		return "", fail(err)
	}

	if err := testPolicyFile.Close(); err != nil {
		return "", fail(err)
	}

	return testPolicyFile.Name(), nil
}

func genTestConfigs(cfgDir string, opts testOpts) (*emconfig.Config, *secconfig.Config, error) {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		return nil, nil, err
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

	if opts.securityProfileMaxImageTags <= 0 {
		opts.securityProfileMaxImageTags = 3
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

	if opts.activityDumpSyscallMonitorPeriod == time.Duration(0) {
		opts.activityDumpSyscallMonitorPeriod = 60 * time.Second
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPoliciesDir":                            cfgDir,
		"DisableApprovers":                           opts.disableApprovers,
		"DisableDiscarders":                          opts.disableDiscarders,
		"EnableActivityDump":                         opts.enableActivityDump,
		"ActivityDumpRateLimiter":                    opts.activityDumpRateLimiter,
		"ActivityDumpTagRules":                       opts.activityDumpTagRules,
		"ActivityDumpDuration":                       opts.activityDumpDuration,
		"ActivityDumpLoadControllerPeriod":           opts.activityDumpLoadControllerPeriod,
		"ActivityDumpLoadControllerTimeout":          opts.activityDumpLoadControllerTimeout,
		"ActivityDumpCleanupPeriod":                  opts.activityDumpCleanupPeriod,
		"ActivityDumpTracedCgroupsCount":             opts.activityDumpTracedCgroupsCount,
		"ActivityDumpCgroupDifferentiateArgs":        opts.activityDumpCgroupDifferentiateArgs,
		"ActivityDumpAutoSuppressionEnabled":         opts.activityDumpAutoSuppressionEnabled,
		"ActivityDumpTracedEventTypes":               opts.activityDumpTracedEventTypes,
		"ActivityDumpLocalStorageDirectory":          opts.activityDumpLocalStorageDirectory,
		"ActivityDumpLocalStorageCompression":        opts.activityDumpLocalStorageCompression,
		"ActivityDumpLocalStorageFormats":            opts.activityDumpLocalStorageFormats,
		"ActivityDumpSyscallMonitorPeriod":           opts.activityDumpSyscallMonitorPeriod,
		"EnableSecurityProfile":                      opts.enableSecurityProfile,
		"SecurityProfileMaxImageTags":                opts.securityProfileMaxImageTags,
		"SecurityProfileDir":                         opts.securityProfileDir,
		"SecurityProfileWatchDir":                    opts.securityProfileWatchDir,
		"EnableAutoSuppression":                      opts.enableAutoSuppression,
		"AutoSuppressionEventTypes":                  opts.autoSuppressionEventTypes,
		"EnableAnomalyDetection":                     opts.enableAnomalyDetection,
		"AnomalyDetectionEventTypes":                 opts.anomalyDetectionEventTypes,
		"AnomalyDetectionDefaultMinimumStablePeriod": opts.anomalyDetectionDefaultMinimumStablePeriod,
		"AnomalyDetectionMinimumStablePeriodExec":    opts.anomalyDetectionMinimumStablePeriodExec,
		"AnomalyDetectionMinimumStablePeriodDNS":     opts.anomalyDetectionMinimumStablePeriodDNS,
		"AnomalyDetectionWarmupPeriod":               opts.anomalyDetectionWarmupPeriod,
		"ErpcDentryResolutionEnabled":                erpcDentryResolutionEnabled,
		"MapDentryResolutionEnabled":                 mapDentryResolutionEnabled,
		"LogPatterns":                                logPatterns,
		"LogTags":                                    logTags,
		"EnvsWithValue":                              opts.envsWithValue,
		"RuntimeSecurityEnabled":                     runtimeSecurityEnabled,
		"SBOMEnabled":                                opts.enableSBOM,
		"HostSBOMEnabled":                            opts.enableHostSBOM,
		"EBPFLessEnabled":                            ebpfLessEnabled,
		"FIMEnabled":                                 opts.enableFIM, // should only be enabled/disabled on windows
		"NetworkIngressEnabled":                      opts.networkIngressEnabled,
		"NetworkRawPacketEnabled":                    opts.networkRawPacketEnabled,
		"OnDemandRateLimiterEnabled":                 !opts.disableOnDemandRateLimiter,
		"EnforcementExcludeBinary":                   opts.enforcementExcludeBinary,
		"EnforcementDisarmerContainerEnabled":        opts.enforcementDisarmerContainerEnabled,
		"EnforcementDisarmerContainerMaxAllowed":     opts.enforcementDisarmerContainerMaxAllowed,
		"EnforcementDisarmerContainerPeriod":         opts.enforcementDisarmerContainerPeriod,
		"EnforcementDisarmerExecutableEnabled":       opts.enforcementDisarmerExecutableEnabled,
		"EnforcementDisarmerExecutableMaxAllowed":    opts.enforcementDisarmerExecutableMaxAllowed,
		"EnforcementDisarmerExecutablePeriod":        opts.enforcementDisarmerExecutablePeriod,
		"EventServerRetention":                       opts.eventServerRetention,
	}); err != nil {
		return nil, nil, err
	}

	ddConfigName, sysprobeConfigName, err := func() (string, string, error) {
		ddConfig, err := os.OpenFile(path.Join(cfgDir, "datadog.yaml"), os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return "", "", err
		}
		defer ddConfig.Close()

		sysprobeConfig, err := os.Create(path.Join(cfgDir, "system-probe.yaml"))
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

	err = spconfig.SetupOptionalDatadogConfigWithDir(cfgDir, ddConfigName)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to set up datadog.yaml configuration: %s", err)
	}

	_, err = spconfig.New(sysprobeConfigName, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	emconfig := emconfig.NewConfig()

	secconfig, err := secconfig.NewConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	secconfig.Probe.ERPCDentryResolutionEnabled = !opts.disableERPCDentryResolution
	secconfig.Probe.MapDentryResolutionEnabled = !opts.disableMapDentryResolution

	return emconfig, secconfig, nil
}

type fakeMsgSender struct {
	sync.Mutex

	testMod *testModule
	msgs    map[eval.RuleID]*api.SecurityEventMessage
}

func (fs *fakeMsgSender) Send(msg *api.SecurityEventMessage, _ func(*api.SecurityEventMessage)) {
	fs.Lock()
	defer fs.Unlock()

	msgStruct := struct {
		AgentContext events.AgentContext `json:"agent"`
	}{}

	if err := json.Unmarshal(msg.Data, &msgStruct); err != nil {
		fs.testMod.t.Fatal(err)
	}

	fs.msgs[msgStruct.AgentContext.RuleID] = msg
}

func (fs *fakeMsgSender) getMsg(ruleID eval.RuleID) *api.SecurityEventMessage {
	fs.Lock()
	defer fs.Unlock()

	return fs.msgs[ruleID]
}

func (fs *fakeMsgSender) flush() {
	fs.Lock()
	defer fs.Unlock()

	fs.msgs = make(map[eval.RuleID]*api.SecurityEventMessage)
}

func newFakeMsgSender(testMod *testModule) *fakeMsgSender {
	return &fakeMsgSender{
		testMod: testMod,
		msgs:    make(map[eval.RuleID]*api.SecurityEventMessage),
	}
}

func jsonPathValidation(testMod *testModule, data []byte, fnc func(testMod *testModule, obj interface{})) {
	var obj interface{}

	if err := json.Unmarshal(data, &obj); err != nil {
		testMod.t.Error(err)
		return
	}

	fnc(testMod, obj)
}
