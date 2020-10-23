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

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	eventChanLength     = 100
	discarderChanLength = 100
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
{{if not .EnableFilters}}
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

const (
	HostEnvironment   = "host"
	DockerEnvironment = "docker"
)

type testEvent struct {
	event eval.Event
	rule  *eval.Rule
}

type testOpts struct {
	enableFilters     bool
	disableApprovers  bool
	disableDiscarders bool
	testDir           string
}

type testModule struct {
	st       *simpleTest
	module   api.Module
	listener net.Listener
	events   chan testEvent
}

type testDiscarder struct {
	event eval.Event
	field string
}

type testProbe struct {
	st         *simpleTest
	probe      *sprobe.Probe
	events     chan *sprobe.Event
	discarders chan *testDiscarder
	rs         *rules.RuleSet
}

type testEventHandler struct {
	ruleSet    *rules.RuleSet
	events     chan *sprobe.Event
	discarders chan *testDiscarder
}

func (h *testEventHandler) HandleEvent(event *sprobe.Event) {
	h.events <- event
	h.ruleSet.Evaluate(event)
}

func (h *testEventHandler) RuleMatch(rule *eval.Rule, event eval.Event) {}

func (h *testEventHandler) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field string) {
	h.discarders <- &testDiscarder{event: event, field: field}
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

func setTestConfig(dir string, macros []*rules.MacroDefinition, rules []*rules.RuleDefinition, opts testOpts) (string, error) {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		return "", err
	}

	testPolicyFile, err := ioutil.TempFile(dir, "secagent-policy.*.policy")
	if err != nil {
		return "", err
	}

	fail := func(err error) error {
		os.Remove(testPolicyFile.Name())
		return err
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPoliciesDir":   path.Dir(testPolicyFile.Name()),
		"EnableFilters":     opts.enableFilters,
		"DisableApprovers":  opts.disableApprovers,
		"DisableDiscarders": opts.disableDiscarders,
	}); err != nil {
		return "", fail(err)
	}

	aconfig.Datadog.SetConfigType("yaml")
	if err := aconfig.Datadog.ReadConfig(buffer); err != nil {
		return "", fail(err)
	}

	tmpl, err = template.New("test-policy").Parse(testPolicy)
	if err != nil {
		return "", fail(err)
	}

	buffer = new(bytes.Buffer)
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
	st, err := newSimpleTest(macros, rules, opts.testDir)
	if err != nil {
		return nil, err
	}

	cfgFilename, err := setTestConfig(st.root, macros, rules, opts)
	if err != nil {
		return nil, err
	}
	defer os.Remove(cfgFilename)

	mod, err := module.NewModule(pconfig.NewDefaultAgentConfig(false))
	if err != nil {
		return nil, err
	}

	testMod := &testModule{
		st:     st,
		module: mod,
		events: make(chan testEvent, eventChanLength),
	}

	rs := mod.(*module.Module).GetRuleSet()
	rs.AddListener(testMod)

	if err := mod.Register(nil); err != nil {
		return nil, err
	}

	return testMod, nil
}

func (tm *testModule) Root() string {
	return tm.st.root
}

func (tm *testModule) RuleMatch(rule *eval.Rule, event eval.Event) {
	tm.events <- testEvent{event: event, rule: rule}
}

func (tm *testModule) EventDiscarderFound(rs *rules.RuleSet, event eval.Event, field string) {
}

func (tm *testModule) GetEvent(eventType ...eval.EventType) (*sprobe.Event, *eval.Rule, error) {
	timeout := time.After(3 * time.Second)

	for {
		select {
		case event := <-tm.events:
			if e, ok := event.event.(*sprobe.Event); ok {
				if len(eventType) > 0 {
					if e.GetType() == eventType[0] {
						return e, event.rule, nil
					}
				} else {
					return e, event.rule, nil
				}
			}
			return nil, nil, errors.New("invalid event")
		case <-timeout:
			return nil, nil, errors.New("timeout")
		}
	}
}

func (tm *testModule) Path(filename string) (string, unsafe.Pointer, error) {
	return tm.st.Path(filename)
}

func (tm *testModule) Close() {
	tm.st.Close()
	tm.module.Close()
	time.Sleep(time.Second)
}

func newTestProbe(macrosDef []*rules.MacroDefinition, rulesDef []*rules.RuleDefinition, opts testOpts) (*testProbe, error) {
	st, err := newSimpleTest(macrosDef, rulesDef, opts.testDir)
	if err != nil {
		return nil, err
	}

	cfgFilename, err := setTestConfig(st.root, macrosDef, rulesDef, opts)
	if err != nil {
		return nil, err
	}
	defer os.Remove(cfgFilename)

	config, err := config.NewConfig(pconfig.NewDefaultAgentConfig(false))
	if err != nil {
		return nil, err
	}

	probe, err := sprobe.NewProbe(config)
	if err != nil {
		return nil, err
	}

	ruleSet := probe.NewRuleSet(rules.NewOptsWithParams(false, sprobe.SECLConstants, sprobe.InvalidDiscarders))

	if err := policy.LoadPolicies(config, ruleSet); err != nil {
		return nil, err
	}

	events := make(chan *sprobe.Event, eventChanLength)
	discarders := make(chan *testDiscarder, discarderChanLength)

	handler := &testEventHandler{events: events, discarders: discarders, ruleSet: ruleSet}
	probe.SetEventHandler(handler)
	ruleSet.AddListener(handler)

	if err := probe.Init(); err != nil {
		return nil, err
	}

	rsa := sprobe.NewRuleSetApplier(config)

	if err := rsa.SelectProbes(ruleSet, probe); err != nil {
		return nil, err
	}

	if err := probe.InitManager(); err != nil {
		return nil, err
	}

	_, err = rsa.Apply(ruleSet, probe)
	if err != nil {
		return nil, err
	}

	// Start and Snapshot are called in the reverse order in the real module. Calling the snapshot before some discarder
	// tests is very noisy and makes the output unreadable. The (very unlikely) risk of missing a crucial mount point or
	// process between the call of Snapshot and Start is acceptable for testing, and not worth delaying the tests or
	// making them unreadable.
	if err := probe.Snapshot(); err != nil {
		return nil, err
	}

	if err := probe.Start(); err != nil {
		return nil, err
	}

	test := &testProbe{
		st:         st,
		probe:      probe,
		events:     events,
		discarders: discarders,
		rs:         ruleSet,
	}

	return test, nil
}

func (tp *testProbe) Root() string {
	return tp.st.root
}

func (tp *testProbe) GetEvent(timeout time.Duration, eventType ...eval.EventType) (*sprobe.Event, error) {
	t := time.After(timeout)

	for {
		select {
		case event := <-tp.events:
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

func (tp *testProbe) Path(filename string) (string, unsafe.Pointer, error) {
	return tp.st.Path(filename)
}

func (tp *testProbe) Close() {
	tp.st.Close()
	tp.probe.Close()
	time.Sleep(10 * time.Second)
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
		log.SetupDatadogLogger(logger, logLevel.String())

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
			return nil, err
		}
	}

	for _, macro := range macros {
		if macro.Expression, err = executeExpressionTemplate(macro.Expression); err != nil {
			return nil, err
		}
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
}

func init() {
	flag.StringVar(&testEnvironment, "env", HostEnvironment, "environment used to run the test suite: ex: host, docker")
}
