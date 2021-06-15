// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build functionaltests stresstests

package tests

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"text/template"
	"time"
	"unsafe"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	eventChanLength     = 10000
	handlerChanLength   = 20000
	discarderChanLength = 10000
	logger              seelog.LoggerInterface
)

const (
	grpcAddr        = "127.0.0.1:18787"
	getEventTimeout = 3 * time.Second
)

const testConfig = `---
log_level: DEBUG
system_probe_config:
  enabled: true
  sysprobe_socket: /tmp/test-sysprobe.sock

runtime_security_config:
  enabled: true
  fim_enabled: true
  remote_tagger: false
  custom_sensitive_words:
    - "*custom*"
  socket: /tmp/test-security-probe.sock
  flush_discarder_window: 0
  load_controller:
    events_count_threshold: {{ .EventsCountThreshold }}
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

  policies:
    dir: {{.TestPoliciesDir}}
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
{{end}}
`

var (
	disableERPCDentryResolution bool
	testEnvironment             string
	useReload                   bool
	logLevelStr                 string
)

const (
	HostEnvironment   = "host"
	DockerEnvironment = "docker"
)

type testEvent struct {
	eval.Event
	rule *eval.Rule
}

type testOpts struct {
	testDir                     string
	disableFilters              bool
	disableApprovers            bool
	disableDiscarders           bool
	wantProbeEvents             bool
	eventsCountThreshold        int
	reuseProbeHandler           bool
	disableERPCDentryResolution bool
	disableMapDentryResolution  bool
}

func (to testOpts) Equal(opts testOpts) bool {
	return to.testDir == opts.testDir &&
		to.disableApprovers == opts.disableApprovers &&
		to.disableDiscarders == opts.disableDiscarders &&
		to.disableFilters == opts.disableFilters &&
		to.eventsCountThreshold == opts.eventsCountThreshold &&
		to.reuseProbeHandler == opts.reuseProbeHandler &&
		to.disableERPCDentryResolution == opts.disableERPCDentryResolution &&
		to.disableMapDentryResolution == opts.disableMapDentryResolution
}

type testModule struct {
	config       *config.Config
	opts         testOpts
	st           *simpleTest
	module       *module.Module
	probe        *sprobe.Probe
	probeHandler *testEventHandler
	listener     net.Listener
	events       chan testEvent
	discarders   chan *testDiscarder
	cmdWrapper   cmdWrapper
}

var testMod *testModule

type testDiscarder struct {
	event     eval.Event
	field     string
	eventType eval.EventType
}

type testProbe struct {
	st         *simpleTest
	probe      *sprobe.Probe
	events     chan *sprobe.Event
	discarders chan *testDiscarder
	rs         *rules.RuleSet
}

type testEventHandler struct {
	events       [2]chan *sprobe.Event
	customEvents [2]chan *module.RuleEvent
	activeChan   uint64
}

func (h *testEventHandler) GetActiveEventsChan() chan *sprobe.Event {
	return h.events[atomic.LoadUint64(&h.activeChan)]
}

func (h *testEventHandler) GetActiveCustomEventsChan() chan *module.RuleEvent {
	return h.customEvents[atomic.LoadUint64(&h.activeChan)]
}

func (h *testEventHandler) ClearEventsChannels() {
	oldIndex := atomic.SwapUint64(&h.activeChan, 1-atomic.LoadUint64(&h.activeChan))
	h.events[oldIndex] = make(chan *sprobe.Event, handlerChanLength)
	h.customEvents[oldIndex] = make(chan *module.RuleEvent, handlerChanLength)
}

func (h *testEventHandler) HandleEvent(event *sprobe.Event) {
	testMod.module.HandleEvent(event)

	e := event.Retain()
	select {
	case h.GetActiveEventsChan() <- &e:
		break
	default:
		log.Tracef("dropped probe event %+v", event)
	}
}

func (h *testEventHandler) HandleCustomEvent(rule *rules.Rule, event *sprobe.CustomEvent) {
	e := event.Clone()
	re := module.RuleEvent{
		RuleID: rule.ID,
		Event:  &e,
	}
	select {
	case h.GetActiveCustomEventsChan() <- &re:
		break
	default:
		log.Tracef("dropped probe custom event %+v")
	}
}

func getInode(t *testing.T, path string) uint64 {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}

	stats, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal(errors.New("Not a syscall.Stat_t"))
	}

	return stats.Ino
}

func assertMode(t *testing.T, actualMode, expectedMode uint32, msgAndArgs ...interface{}) {
	t.Helper()
	if len(msgAndArgs) == 0 {
		msgAndArgs = append(msgAndArgs, "wrong mode")
	}
	assert.Equal(t, strconv.FormatUint(uint64(actualMode), 8), strconv.FormatUint(uint64(expectedMode), 8), msgAndArgs...)
}

func assertRights(t *testing.T, actualMode, expectedMode uint16, msgAndArgs ...interface{}) {
	t.Helper()
	assertMode(t, uint32(actualMode)&01777, uint32(expectedMode), msgAndArgs...)
}

func assertNearTime(t *testing.T, event time.Time) {
	t.Helper()
	now := time.Now()
	if event.After(now) || event.Before(now.Add(-1*time.Hour)) {
		t.Errorf("expected time close to %s, got %s", now, event)
	}
}

func assertTriggeredRule(t *testing.T, r *eval.Rule, id string) {
	t.Helper()
	assert.Equal(t, r.ID, id, "wrong triggered rule")
}

func assertReturnValue(t *testing.T, retval, expected int64) {
	t.Helper()
	assert.Equal(t, retval, expected, "wrong return value")
}

func assertFieldEqual(t *testing.T, e *sprobe.Event, field string, value interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		t.Errorf("failed to get field '%s': %s", field, err)
	} else {
		assert.Equal(t, fieldValue, value, msgAndArgs...)
	}
}

func assertFieldOneOf(t *testing.T, e *sprobe.Event, field string, values []interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	fieldValue, err := e.GetFieldValue(field)
	if err != nil {
		t.Errorf("failed to get field '%s': %s", field, err)
	} else {
		assert.Assert(t, is.Contains(values, fieldValue))
	}
}

func setTestConfig(dir string, opts testOpts) (string, error) {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		return "", err
	}

	if opts.eventsCountThreshold == 0 {
		opts.eventsCountThreshold = 100000000
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
		"TestPoliciesDir":             dir,
		"DisableApprovers":            opts.disableApprovers,
		"DisableDiscarders":           opts.disableDiscarders,
		"EventsCountThreshold":        opts.eventsCountThreshold,
		"ErpcDentryResolutionEnabled": erpcDentryResolutionEnabled,
		"MapDentryResolutionEnabled":  mapDentryResolutionEnabled,
	}); err != nil {
		return "", err
	}

	sysprobeConfig, err := os.Create(path.Join(opts.testDir, "system-probe.yaml"))
	if err != nil {
		return "", err
	}
	defer sysprobeConfig.Close()

	_, err = io.Copy(sysprobeConfig, buffer)
	return sysprobeConfig.Name(), err
}

func setTestPolicy(dir string, macros []*rules.MacroDefinition, rules []*rules.RuleDefinition) (string, error) {
	testPolicyFile, err := ioutil.TempFile(dir, "secagent-policy.*.policy")
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

func newTestModule(macros []*rules.MacroDefinition, rules []*rules.RuleDefinition, opts testOpts) (*testModule, error) {
	logLevel, found := seelog.LogLevelFromString(logLevelStr)
	if !found {
		return nil, fmt.Errorf("invalid log level '%s'", logLevel)
	}

	st, err := newSimpleTest(macros, rules, opts.testDir, logLevel)
	if err != nil {
		return nil, err
	}

	sysprobeConfig, err := setTestConfig(st.root, opts)
	if err != nil {
		return nil, err
	}

	cfgFilename, err := setTestPolicy(st.root, macros, rules)
	if err != nil {
		return nil, err
	}
	defer os.Remove(cfgFilename)

	var cmdWrapper cmdWrapper
	if testEnvironment == DockerEnvironment {
		cmdWrapper = newStdCmdWrapper()
	} else {
		wrapper, err := newDockerCmdWrapper(st.Root())
		if err == nil {
			cmdWrapper = newMultiCmdWrapper(wrapper, newStdCmdWrapper())
		} else {
			// docker not present run only on host
			cmdWrapper = newStdCmdWrapper()
		}
	}

	if useReload && testMod != nil {
		if opts.Equal(testMod.opts) {
			testMod.reset()
			testMod.st = st
			testMod.cmdWrapper = cmdWrapper
			return testMod, testMod.reloadConfiguration()
		}
		testMod.cleanup()
	}

	agentConfig, err := sysconfig.New(sysprobeConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config")
	}
	config, err := config.NewConfig(agentConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config")
	}

	config.ERPCDentryResolutionEnabled = !opts.disableERPCDentryResolution
	config.MapDentryResolutionEnabled = !opts.disableMapDentryResolution

	mod, err := module.NewModule(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create module")
	}

	if opts.disableApprovers {
		config.EnableApprovers = false
	}

	if opts.disableDiscarders {
		config.EnableDiscarders = false
	}

	testMod = &testModule{
		config:     config,
		opts:       opts,
		st:         st,
		module:     mod.(*module.Module),
		probe:      mod.(*module.Module).GetProbe(),
		events:     make(chan testEvent, eventChanLength),
		discarders: make(chan *testDiscarder, discarderChanLength),
		probeHandler: &testEventHandler{
			events:       [2]chan *sprobe.Event{make(chan *sprobe.Event, handlerChanLength), make(chan *sprobe.Event, handlerChanLength)},
			customEvents: [2]chan *module.RuleEvent{make(chan *module.RuleEvent, handlerChanLength), make(chan *module.RuleEvent, handlerChanLength)},
		},
		cmdWrapper: cmdWrapper,
	}

	if err := mod.Register(nil); err != nil {
		return nil, errors.Wrap(err, "failed to register module")
	}

	rs := mod.(*module.Module).GetRuleSet()
	rs.AddListener(testMod)

	testMod.probe.SetEventHandler(testMod.probeHandler)

	return testMod, nil
}

func (tm *testModule) Run(t *testing.T, name string, fnc func(t *testing.T, kind wrapperType, cmd func(bin string, args []string, envs []string) *exec.Cmd)) {
	tm.cmdWrapper.Run(t, name, fnc)
}

func (tm *testModule) reset() {
	tm.probeHandler.ClearEventsChannels()
	tm.events = make(chan testEvent, eventChanLength)
	tm.discarders = make(chan *testDiscarder, discarderChanLength)
}

func (tm *testModule) reloadConfiguration() error {
	log.Debugf("reload configuration with testDir: %s", tm.Root())
	tm.config.PoliciesDir = tm.Root()

	if err := tm.module.Reload(); err != nil {
		return errors.Wrap(err, "failed to reload test module")
	}

	rs := tm.module.GetRuleSet()
	rs.AddListener(tm)
	return nil
}

func (tm *testModule) Root() string {
	return tm.st.root
}

func (tm *testModule) SwapLogLevel(logLevel seelog.LogLevel) (seelog.LogLevel, error) {
	return tm.st.swapLogLevel(logLevel)
}

func (tm *testModule) RuleMatch(rule *rules.Rule, event eval.Event) {
	e := event.(*sprobe.Event).Retain()

	te := testEvent{Event: &e, rule: rule.Rule}
	select {
	case tm.events <- te:
	default:
		log.Warnf("Discarding rule match %+v", te)
	}
}

func (tm *testModule) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	e := event.(*sprobe.Event).Retain()

	discarder := &testDiscarder{event: &e, field: field, eventType: eventType}
	select {
	case tm.discarders <- discarder:
	default:
		log.Tracef("Discarding discarder %+v", discarder)
	}
}

func (tm *testModule) GetEvent() (*sprobe.Event, *eval.Rule, error) {
	timeout := time.After(getEventTimeout)

	for {
		select {
		case event := <-tm.events:
			if e, ok := event.Event.(*sprobe.Event); ok {
				return e, event.rule, nil
			}
			return nil, nil, errors.New("invalid event")
		case <-timeout:
			return nil, nil, errors.New("timeout")
		}
	}
}

func (tm *testModule) GetProbeCustomEvent(timeout time.Duration, eventType ...eval.EventType) (*module.RuleEvent, error) {
	if tm.probeHandler == nil {
		return nil, errors.New("could not get the probe events without using the `wantProbeEvents` test option")
	}

	t := time.After(timeout)

	for {
		select {
		case ruleEvent := <-tm.probeHandler.GetActiveCustomEventsChan():
			if len(eventType) > 0 {
				if ruleEvent.Event.GetType() == eventType[0] {
					return ruleEvent, nil
				}
			} else {
				return ruleEvent, nil
			}
		case <-t:
			return nil, errors.New("timeout")
		}
	}
}

func (tm *testModule) GetProbeEvent(timeout time.Duration, eventType ...eval.EventType) (*sprobe.Event, error) {
	if tm.probeHandler == nil {
		return nil, errors.New("could not get probe events without using the 'wantProbeEvents' test option")
	}

	t := time.After(timeout)

	for {
		select {
		case event := <-tm.probeHandler.GetActiveEventsChan():
			if len(eventType) > 0 {
				if event.GetType() == eventType[0] {
					return event, nil
				}
			} else {
				return event, nil
			}
		case <-t:
			return nil, errors.New("timeout")
		}
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

type tracePipeLogger struct {
	*TracePipe
	stop       chan struct{}
	executable string
}

func (l *tracePipeLogger) handleEvent(event *TraceEvent) {
	// for some reason, the event task is resolved to "<...>"
	// so we check that event.PID is the ID of a task of the running process
	taskPath := filepath.Join(util.HostProc(), strconv.Itoa(int(utils.Getpid())), "task", event.PID)
	_, err := os.Stat(taskPath)

	if event.Task == l.executable || (event.Task == "<...>" && err == nil) {
		log.Debug(event.Raw)
	}
}

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

func (l *tracePipeLogger) Stop() {
	time.Sleep(time.Millisecond * 200)

	l.stop <- struct{}{}
	l.Close()
}

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
	tm.st.Close()
	tm.module.Close()
}

func (tm *testModule) Close() {
	if !useReload {
		tm.cleanup()
	}
}

type simpleTest struct {
	root     string
	toRemove bool
}

func (t *simpleTest) Close() {
	if t.toRemove {
		os.RemoveAll(t.root)
	}
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

var logInitilialized bool

func (t *simpleTest) swapLogLevel(logLevel seelog.LogLevel) (seelog.LogLevel, error) {
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

func newSimpleTest(macros []*rules.MacroDefinition, rules []*rules.RuleDefinition, testDir string, logLevel seelog.LogLevel) (*simpleTest, error) {
	var err error

	t := &simpleTest{
		root: testDir,
	}

	if !logInitilialized {
		if _, err := t.swapLogLevel(logLevel); err != nil {
			return nil, err
		}

		logInitilialized = true
	}

	if testDir == "" {
		t.root, err = ioutil.TempDir("", "test-secagent-root")
		if err != nil {
			return nil, err
		}
		t.toRemove = true
		if err := os.Chmod(t.root, 0o711); err != nil {
			return nil, err
		}
	}

	if err := t.load(macros, rules); err != nil {
		return nil, err
	}

	return t, nil
}

// systemUmask caches the system umask between tests
var systemUmask int

func applyUmask(fileMode int) int {
	if systemUmask == 0 {
		// Get the system umask to compute the right access mode
		systemUmask = unix.Umask(0)
		// the previous line overrides the system umask, change it back
		_ = unix.Umask(systemUmask)
	}
	return fileMode &^ systemUmask
}

func testStringFieldContains(t *testing.T, event *sprobe.Event, fieldPath string, expected string) {
	t.Helper()

	// check container path
	value, err := event.GetFieldValue(fieldPath)
	if err != nil {
		t.Fatal(err)
	}

	switch value.(type) {
	case string:
		if !strings.Contains(value.(string), expected) {
			t.Errorf("expected value `%s` for `%s` not found: %+v", expected, fieldPath, event)
		}
	case []string:
		for _, v := range value.([]string) {
			if strings.Contains(v, expected) {
				return
			}
		}
		t.Errorf("expected value `%s` for `%s` not found in for `%+v`: %+v", expected, fieldPath, value, event)
	}
}

func testContainerPath(t *testing.T, event *sprobe.Event, fieldPath string) {
	t.Helper()

	testStringFieldContains(t, event, fieldPath, "docker")
}

func (tm *testModule) flushChannels(duration time.Duration) {
	timeout := time.After(duration)
	for {
		select {
		case <-tm.discarders:
		case <-tm.probeHandler.GetActiveEventsChan():
		case <-timeout:
			return
		}
	}
}

func waitForDiscarder(test *testModule, key string, value interface{}, eventType model.EventType) (*sprobe.Event, error) {
	timeout := time.After(5 * time.Second)

	for {
		select {
		case discarder := <-test.discarders:
			e := discarder.event.(*sprobe.Event)
			if e == nil || (e != nil && e.GetEventType() != eventType) {
				continue
			}
			v, _ := e.GetFieldValue(key)
			if v == value {
				test.flushChannels(time.Second)
				return e, nil
			}
		case <-timeout:
			return nil, errors.New("timeout")
		}
	}
}

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
func waitForProbeEvent(test *testModule, key string, value interface{}, eventType model.EventType) (*sprobe.Event, error) {
	timeout := time.After(getEventTimeout)

	for {
		select {
		case e := <-test.probeHandler.GetActiveEventsChan():
			if e.GetEventType() != eventType {
				continue
			}
			if v, _ := e.GetFieldValue(key); v == value {
				test.flushChannels(time.Second)
				return e, nil
			}
		case <-timeout:
			return nil, errors.New("timeout")
		}
	}
}

func waitForOpenDiscarder(test *testModule, filename string) (*sprobe.Event, error) {
	return waitForDiscarder(test, "open.file.path", filename, model.FileOpenEventType)
}

func waitForOpenProbeEvent(test *testModule, filename string) (*sprobe.Event, error) {
	return waitForProbeEvent(test, "open.file.path", filename, model.FileOpenEventType)
}

func TestEnv(t *testing.T) {
	if testEnvironment != "" && testEnvironment != HostEnvironment && testEnvironment != DockerEnvironment {
		t.Fatal("invalid environment")
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	retCode := m.Run()
	if useReload && testMod != nil {
		testMod.cleanup()
	}
	os.Exit(retCode)
}

func init() {
	os.Setenv("RUNTIME_SECURITY_TESTSUITE", "true")
	flag.StringVar(&testEnvironment, "env", HostEnvironment, "environment used to run the test suite: ex: host, docker")
	flag.BoolVar(&useReload, "reload", true, "reload rules instead of stopping/starting the agent for every test")
	flag.StringVar(&logLevelStr, "loglevel", seelog.WarnStr, "log level")
}
