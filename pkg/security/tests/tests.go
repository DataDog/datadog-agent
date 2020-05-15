package tests

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"text/template"
	"time"

	"google.golang.org/grpc"

	smodule "github.com/DataDog/datadog-agent/cmd/system-probe/module"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
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
	case <-time.After(3 * time.Second):
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
}

func (t *simpleTest) Close() {
	t.module.Close()
	t.client.Close()
	t.cancel()
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

	module, err := newTestModule(macros, rules)
	if err != nil {
		return nil, err
	}

	client, err := newTestClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	client.Run(ctx)

	return &simpleTest{
		module: module,
		client: client,
		cancel: cancel,
	}, nil
}
