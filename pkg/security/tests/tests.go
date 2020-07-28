package tests

import (
	"bytes"
	"io/ioutil"
	"net"
	"os"
	"path"
	"syscall"
	"testing"
	"text/template"
	"time"
	"unsafe"

	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	smodule "github.com/DataDog/datadog-agent/cmd/system-probe/module"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
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
)

const grpcAddr = "127.0.0.1:18787"

const testConfig = `---
log_level: DEBUG
system_probe_config:
  enabled: true
  sysprobe_socket: /tmp/test-sysprobe.sock

runtime_security_config:
  debug: true
  socket: /tmp/test-security-probe.sock
{{if not .EnableFilters}}
  enable_kernel_filters: false
{{end}}
  policies:
    - name: test-policy
      files:
        - {{.TestPolicy}}
`

const testPolicy = `---
{{range $Macro := .Macros}}
- macro:
    id: {{$Macro.ID}}
    expression: >-
      {{$Macro.Expression}}
{{end}}

{{range $Rule := .Rules}}
- rule:
    id: {{$Rule.ID}}
    expression: >-
      {{$Rule.Expression}}
{{end}}
`

type testEvent struct {
	event eval.Event
	rule  *eval.Rule
}

type testOpts struct {
	enableFilters bool
}

type testModule struct {
	st       *simpleTest
	module   smodule.Module
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

func (h *testEventHandler) EventDiscarderFound(event eval.Event, field string) {
	h.discarders <- &testDiscarder{event: event, field: field}
}

func setTestConfig(macros []*policy.MacroDefinition, rules []*policy.RuleDefinition, opts testOpts) (string, error) {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		return "", err
	}

	testPolicyFile, err := ioutil.TempFile("", "secagent-policy")
	if err != nil {
		return "", err
	}

	fail := func(err error) error {
		os.Remove(testPolicyFile.Name())
		return err
	}

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPolicy":    testPolicyFile.Name(),
		"EnableFilters": opts.enableFilters,
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

func newTestModule(macros []*policy.MacroDefinition, rules []*policy.RuleDefinition, opts testOpts) (*testModule, error) {
	st, err := newSimpleTest(macros, rules)
	if err != nil {
		return nil, err
	}

	cfgFilename, err := setTestConfig(macros, rules, opts)
	if err != nil {
		return nil, err
	}
	defer os.Remove(cfgFilename)

	mod, err := module.NewModule(nil)
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

func (tm *testModule) EventDiscarderFound(event eval.Event, field string) {
}

func (tm *testModule) GetEvent() (*sprobe.Event, *eval.Rule, error) {
	timeout := time.After(3 * time.Second)

	select {
	case event := <-tm.events:
		if e, ok := event.event.(*sprobe.Event); ok {
			return e, event.rule, nil
		}
		return nil, nil, errors.New("invalid event")
	case <-timeout:
		return nil, nil, errors.New("timeout")
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

func newTestProbe(macros []*policy.MacroDefinition, rules []*policy.RuleDefinition, opts testOpts) (*testProbe, error) {
	st, err := newSimpleTest(macros, rules)
	if err != nil {
		return nil, err
	}

	cfgFilename, err := setTestConfig(macros, rules, opts)
	if err != nil {
		return nil, err
	}
	defer os.Remove(cfgFilename)

	config, err := config.NewConfig()
	if err != nil {
		return nil, err
	}

	probe, err := sprobe.NewProbe(config)
	if err != nil {
		return nil, err
	}

	if err := probe.Start(); err != nil {
		return nil, err
	}

	ruleSet, err := module.LoadPolicies(config, probe)
	if err != nil {
		return nil, err
	}

	if err := probe.Snapshot(); err != nil {
		return nil, err
	}

	events := make(chan *sprobe.Event, eventChanLength)
	discarders := make(chan *testDiscarder, discarderChanLength)

	handler := &testEventHandler{events: events, discarders: discarders, ruleSet: ruleSet}
	probe.SetEventHandler(handler)
	ruleSet.AddListener(handler)

	if err := probe.Start(); err != nil {
		return nil, err
	}

	if _, err := probe.ApplyRuleSet(ruleSet, false); err != nil {
		return nil, err
	}

	return &testProbe{
		st:         st,
		probe:      probe,
		events:     events,
		discarders: discarders,
	}, nil
}

func (tp *testProbe) Root() string {
	return tp.st.root
}

func (tp *testProbe) GetEvent(timeout time.Duration) (*sprobe.Event, error) {
	select {
	case event := <-tp.events:
		return event, nil
	case <-time.After(timeout):
		return nil, errors.New("timeout")
	}
}

func (tp *testProbe) Path(filename string) (string, unsafe.Pointer, error) {
	return tp.st.Path(filename)
}

func (tp *testProbe) Close() {
	tp.st.Close()
	tp.probe.Stop()
	time.Sleep(time.Second)
}

type simpleTest struct {
	root string
}

func (t *simpleTest) Close() {
	os.RemoveAll(t.root)
}

func (t *simpleTest) Root() string {
	return t.root
}

func (t *simpleTest) Path(filename string) (string, unsafe.Pointer, error) {
	filename = path.Join(t.root, filename)
	filenamePtr, err := syscall.BytePtrFromString(filename)
	if err != nil {
		return "", nil, err
	}
	return filename, unsafe.Pointer(filenamePtr), nil
}

func newSimpleTest(macros []*policy.MacroDefinition, rules []*policy.RuleDefinition) (*simpleTest, error) {
	if testing.Verbose() {
		logger, err := seelog.LoggerFromWriterWithMinLevelAndFormat(os.Stderr, seelog.DebugLvl, "%Ns [%LEVEL] %Msg\n")
		if err != nil {
			return nil, err
		}
		err = seelog.ReplaceLogger(logger)
		if err != nil {
			return nil, err
		}
		log.SetupDatadogLogger(logger, "debug")
	}

	root, err := ioutil.TempDir("", "test-secagent-root")
	if err != nil {
		return nil, err
	}

	t := &simpleTest{
		root: root,
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
