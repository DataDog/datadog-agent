package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"sync/atomic"
	"syscall"
	"testing"
	"text/template"
	"time"
	"unsafe"

	"github.com/cihub/seelog"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	smodule "github.com/DataDog/datadog-agent/cmd/system-probe/module"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const grpcAddr = "127.0.0.1:18787"

const testConfig = `---
log_level: DEBUG
system_probe_config:
  enabled: true
  sysprobe_socket: /tmp/test-sysprobe.sock

security_agent:
  debug: true
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

type testModule struct {
	client   *testClient
	cancel   context.CancelFunc
	st       *simpleTest
	module   smodule.Module
	listener net.Listener
}

type testProbe struct {
	st     *simpleTest
	probe  *sprobe.Probe
	events chan *sprobe.Event
}

type testEventHandler struct {
	events chan *sprobe.Event
}

func (h *testEventHandler) HandleEvent(event *sprobe.Event) {
	h.events <- event
}

func setTestConfig(macros []*policy.MacroDefinition, rules []*policy.RuleDefinition) (string, error) {
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
		"TestPolicy": testPolicyFile.Name(),
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

func newTestModule(macros []*policy.MacroDefinition, rules []*policy.RuleDefinition) (*testModule, error) {
	st, err := newSimpleTest(macros, rules)
	if err != nil {
		return nil, err
	}

	cfgFilename, err := setTestConfig(macros, rules)
	if err != nil {
		return nil, err
	}
	defer os.Remove(cfgFilename)

	client, err := newTestClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	client.Run(ctx)

	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return nil, err
	}

	module, err := module.NewModule(nil)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
	if err := module.Register(grpcServer); err != nil {
		return nil, err
	}

	go grpcServer.Serve(listener)

	return &testModule{
		client:   client,
		cancel:   cancel,
		st:       st,
		module:   module,
		listener: listener,
	}, nil
}

func (tm *testModule) Root() string {
	return tm.st.root
}

func (tm *testModule) GetEvent() (*sprobe.Event, error) {
	event, err := tm.client.GetEvent(3 * time.Second)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(event.GetData(), &data); err != nil {
		return nil, err
	}

	var probeEvent sprobe.Event
	if err := mapstructure.WeakDecode(data, &probeEvent); err != nil {
		return nil, err
	}

	return &probeEvent, nil
}

func (tm *testModule) Path(filename string) (string, unsafe.Pointer, error) {
	return tm.st.Path(filename)
}

func (tm *testModule) Close() {
	tm.st.Close()
	tm.module.Close()
	tm.listener.Close()
}

func newTestProbe(macros []*policy.MacroDefinition, rules []*policy.RuleDefinition) (*testProbe, error) {
	st, err := newSimpleTest(macros, rules)
	if err != nil {
		return nil, err
	}

	cfgFilename, err := setTestConfig(macros, rules)
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

	ruleSet, err := module.LoadPolicies(config, probe)
	if err != nil {
		return nil, err
	}

	if err := probe.ApplyRuleSet(ruleSet); err != nil {
		return nil, err
	}

	events := make(chan *sprobe.Event, 100)

	handler := &testEventHandler{events: events}
	probe.SetEventHandler(handler)

	if err := probe.Start(); err != nil {
		return nil, err
	}

	return &testProbe{
		st:     st,
		probe:  probe,
		events: events,
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
}

type testClient struct {
	conn      *grpc.ClientConn
	apiClient api.SecurityModuleClient
	events    chan *api.SecurityEventMessage
	running   atomic.Value
}

func newTestClient() (*testClient, error) {
	conn, err := grpc.Dial(grpcAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	apiClient := api.NewSecurityModuleClient(conn)
	events := make(chan *api.SecurityEventMessage, 10)

	return &testClient{
		conn:      conn,
		apiClient: apiClient,
		events:    events,
	}, nil
}

func (tc *testClient) Run(ctx context.Context) {
	go func() {
		tc.running.Store(true)

		for tc.running.Load() == true {
			stream, err := tc.apiClient.GetEvents(ctx, &api.GetParams{})
			if err != nil {
				return
			}

			for {
				in, err := stream.Recv()

				if err == io.EOF || in == nil {
					break
				}

				tc.events <- in
			}
		}
	}()
}

func (tc *testClient) GetEvent(timeout time.Duration) (*api.SecurityEventMessage, error) {
	select {
	case event := <-tc.events:
		return event, nil
	case <-time.After(timeout):
		return nil, errors.New("timeout")
	}
}

func (tc *testClient) Close() {
	tc.running.Store(false)
	tc.conn.Close()
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
		log.SetupDatadogLogger(logger, "DEBUG")
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
