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
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
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
	module   smodule.Module
	listener net.Listener
}

func newTestModule(macros []*policy.MacroDefinition, rules []*policy.RuleDefinition) (*testModule, error) {
	tmpl, err := template.New("test-config").Parse(testConfig)
	if err != nil {
		return nil, err
	}

	testPolicyFile, err := ioutil.TempFile("", "secagent-policy")
	if err != nil {
		return nil, err
	}
	defer os.Remove(testPolicyFile.Name())

	buffer := new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"TestPolicy": testPolicyFile.Name(),
	}); err != nil {
		return nil, err
	}

	aconfig.Datadog.SetConfigType("yaml")
	if err := aconfig.Datadog.ReadConfig(buffer); err != nil {
		return nil, err
	}

	tmpl, err = template.New("test-policy").Parse(testPolicy)
	if err != nil {
		return nil, err
	}

	buffer = new(bytes.Buffer)
	if err := tmpl.Execute(buffer, map[string]interface{}{
		"Rules":  rules,
		"Macros": macros,
	}); err != nil {
		return nil, err
	}

	_, err = testPolicyFile.Write(buffer.Bytes())
	if err != nil {
		return nil, err
	}

	if err := testPolicyFile.Close(); err != nil {
		return nil, err
	}

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
		module:   module,
		listener: listener,
	}, nil
}

func (tm *testModule) Close() {
	tm.module.Close()
	tm.listener.Close()
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
	module *testModule
	client *testClient
	cancel context.CancelFunc
	root   string
}

func (t *simpleTest) Close() {
	t.module.Close()
	t.client.Close()
	t.cancel()
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

func (t *simpleTest) GetEvent() (*probe.Event, error) {
	event, err := t.client.GetEvent(3 * time.Second)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(event.GetData(), &data); err != nil {
		return nil, err
	}

	var probeEvent probe.Event
	if err := mapstructure.WeakDecode(data, &probeEvent); err != nil {
		return nil, err
	}

	return &probeEvent, nil
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

	t.module, err = newTestModule(macros, rules)
	if err != nil {
		return nil, err
	}

	t.client, err = newTestClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.client.Run(ctx)
	t.cancel = cancel

	return t, nil
}
