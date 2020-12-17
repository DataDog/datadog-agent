// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//+build functionaltests

package tests

import (
	"bytes"
	"flag"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strings"
	"syscall"
	"testing"
	"text/template"
	"time"
	"unsafe"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	eventChanLength     = 10000
	discarderChanLength = 10000
	logger              seelog.LoggerInterface
)

const grpcAddr = "127.0.0.1:18787"

const testConfig = `---
log_level: DEBUG
system_probe_config:
  enabled: true
  sysprobe_socket: /tmp/test-sysprobe.sock

runtime_security_config:
  enabled: true
  socket: /tmp/test-security-probe.sock
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

var testEnvironment string
var useReload bool

const (
	HostEnvironment   = "host"
	DockerEnvironment = "docker"
)

type testEvent struct {
	eval.Event
	rule *eval.Rule
}

type testOpts struct {
	testDir           string
	disableFilters    bool
	disableApprovers  bool
	disableDiscarders bool
	wantProbeEvents   bool
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
	ruleSet *rules.RuleSet
	events  chan *sprobe.Event
}

func (h *testEventHandler) HandleEvent(event *sprobe.Event) {
	e := event.Clone()
	select {
	case h.events <- &e:
		break
	default:
		log.Debugf("dropped probe event %+v")
	}
	h.ruleSet.Evaluate(event)
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

func setTestConfig(dir string, opts testOpts) error {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		return err
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPoliciesDir": dir,
	}); err != nil {
		return err
	}

	aconfig.Datadog.SetConfigType("yaml")
	return aconfig.Datadog.ReadConfig(buffer)
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
	defer func() {
		if testMod == nil {
			return
		}

		if opts.wantProbeEvents {
			ruleSet := testMod.module.GetRuleSet()
			handler := &testEventHandler{
				events:  make(chan *sprobe.Event, 16384),
				ruleSet: ruleSet,
			}
			testMod.probeHandler = handler
			testMod.probe.SetEventHandler(handler)
		} else {
			testMod.probeHandler = nil
			testMod.probe.SetEventHandler(testMod.module)
		}
	}()

	st, err := newSimpleTest(macros, rules, opts.testDir)
	if err != nil {
		return nil, err
	}

	if err := setTestConfig(st.root, opts); err != nil {
		return nil, err
	}

	cfgFilename, err := setTestPolicy(st.root, macros, rules)
	if err != nil {
		return nil, err
	}
	defer os.Remove(cfgFilename)

	if useReload && testMod != nil {
		if opts.disableApprovers == testMod.opts.disableApprovers &&
			opts.disableDiscarders == testMod.opts.disableDiscarders &&
			opts.disableFilters == testMod.opts.disableFilters {
			testMod.reset()
			testMod.st = st
			return testMod, testMod.reloadConfiguration()
		}

		testMod.cleanup()
	}

	agentConfig := pconfig.NewDefaultAgentConfig(false)
	config, err := config.NewConfig(agentConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config")
	}

	mod, err := module.NewModule(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create module")
	}

	testMod = &testModule{
		config:     config,
		opts:       opts,
		st:         st,
		module:     mod.(*module.Module),
		probe:      mod.(*module.Module).GetProbe(),
		events:     make(chan testEvent, eventChanLength),
		discarders: make(chan *testDiscarder, discarderChanLength),
	}

	if err := mod.Register(nil); err != nil {
		return nil, errors.Wrap(err, "failed to register module")
	}

	rs := mod.(*module.Module).GetRuleSet()
	rs.AddListener(testMod)

	return testMod, nil
}

func (tm *testModule) reset() {
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

func (tm *testModule) RuleMatch(rule *rules.Rule, event eval.Event) {
	e := event.(*sprobe.Event).Clone()
	te := testEvent{Event: &e, rule: rule.Rule}
	select {
	case tm.events <- te:
	default:
		log.Warnf("Discarding rule match %+v", te)
	}
}

func (tm *testModule) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field eval.Field, eventType eval.EventType) {
	e := event.(*sprobe.Event).Clone()
	discarder := &testDiscarder{event: &e, field: field, eventType: eventType}
	select {
	case tm.discarders <- discarder:
	default:
		log.Warnf("Discarding discarder %+v", discarder)
	}
}

func (tm *testModule) GetEvent() (*sprobe.Event, *eval.Rule, error) {
	timeout := time.After(3 * time.Second)

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

func (tm *testModule) GetProbeEvent(timeout time.Duration, eventType ...eval.EventType) (*sprobe.Event, error) {
	if tm.probeHandler == nil {
		return nil, errors.New("could not get probe events without using the 'wantProbeEvents' test option")
	}

	t := time.After(timeout)

	for {
		select {
		case event := <-tm.probeHandler.events:
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

func (tm *testModule) Path(filename string) (string, unsafe.Pointer, error) {
	return tm.st.Path(filename)
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

func (t *simpleTest) Path(filename string) (string, unsafe.Pointer, error) {
	filename = path.Join(t.root, filename)
	filenamePtr, err := syscall.BytePtrFromString(filename)
	if err != nil {
		return "", nil, err
	}
	return filename, unsafe.Pointer(filenamePtr), nil
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

func newSimpleTest(macros []*rules.MacroDefinition, rules []*rules.RuleDefinition, testDir string) (*simpleTest, error) {
	var err error

	if !logInitilialized {
		var logLevel seelog.LogLevel = seelog.InfoLvl
		if testing.Verbose() {
			logLevel = seelog.TraceLvl
		}

		constraints, err := seelog.NewMinMaxConstraints(logLevel, seelog.CriticalLvl)
		if err != nil {
			return nil, err
		}

		formatter, err := seelog.NewFormatter("%Ns [%LEVEL] %Func %Line %Msg\n")
		if err != nil {
			return nil, err
		}

		dispatcher, err := seelog.NewSplitDispatcher(formatter, []interface{}{os.Stderr})
		if err != nil {
			return nil, err
		}

		specificConstraints, _ := seelog.NewListConstraints([]seelog.LogLevel{})
		ex, _ := seelog.NewLogLevelException("*.Snapshot", "*", specificConstraints)
		exceptions := []*seelog.LogLevelException{ex}

		logger := seelog.NewAsyncLoopLogger(seelog.NewLoggerConfig(constraints, exceptions, dispatcher))

		err = seelog.ReplaceLogger(logger)
		if err != nil {
			return nil, err
		}
		log.SetupLogger(logger, logLevel.String())

		logInitilialized = true
	}

	t := &simpleTest{
		root: testDir,
	}

	if testDir == "" {
		t.root, err = ioutil.TempDir("", "test-secagent-root")
		if err != nil {
			return nil, err
		}
		t.toRemove = true
	}

	if err := t.load(macros, rules); err != nil {
		return nil, err
	}

	return t, nil
}

func testContainerPath(t *testing.T, event *sprobe.Event, fieldPath string) {
	if testEnvironment != DockerEnvironment {
		return
	}

	path, err := event.GetFieldValue(fieldPath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(path.(string), "docker") {
		t.Errorf("incorrect container_path, should contain `docker`: %s", path)
	}
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
	flag.StringVar(&testEnvironment, "env", HostEnvironment, "environment used to run the test suite: ex: host, docker")
	flag.BoolVar(&useReload, "reload", true, "reload rules instead of stopping/starting the agent for every test")
}
