package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/checks"

	"github.com/DataDog/datadog-agent/pkg/process/util/api"

	"github.com/DataDog/agent-payload/process"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHostName = "test-host"

func TestSendConnectionsMessage(t *testing.T) {
	m := &process.CollectorConnections{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.Connections.Name(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(false), &endpointConfig{}, func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/collector", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
		assert.Equal(t, cfg.APIEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		cc, ok := reqBody.Body.(*process.CollectorConnections)
		require.True(t, ok)

		assert.Equal(t, cfg.HostName, cc.HostName)
		assert.Equal(t, int32(1), cc.GroupId)
	})
}

func TestSendContainerMessage(t *testing.T) {
	m := &process.CollectorContainer{
		HostName: testHostName,
		GroupId:  1,
		Containers: []*process.Container{
			{Id: "1", Name: "foo"},
		},
	}

	check := &testCheck{
		name: checks.Container.Name(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(false), &endpointConfig{}, func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/container", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
		assert.Equal(t, cfg.APIEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "1", req.headers.Get(api.ContainerCountHeader))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		_, ok := reqBody.Body.(*process.CollectorContainer)
		require.True(t, ok)
	})
}

func TestSendProcMessage(t *testing.T) {
	m := &process.CollectorProc{
		HostName: testHostName,
		GroupId:  1,
		Containers: []*process.Container{
			{Id: "1", Name: "foo"},
		},
	}

	check := &testCheck{
		name: checks.Process.Name(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(false), &endpointConfig{}, func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/collector", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
		assert.Equal(t, cfg.APIEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "1", req.headers.Get(api.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get(api.TimestampHeader))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		_, ok := reqBody.Body.(*process.CollectorProc)
		require.True(t, ok)
	})
}

func TestSendProcMessageWithRetry(t *testing.T) {
	m := &process.CollectorProc{
		HostName: testHostName,
		GroupId:  1,
		Containers: []*process.Container{
			{Id: "1", Name: "foo"},
		},
	}

	check := &testCheck{
		name: checks.Process.Name(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(false), &endpointConfig{ErrorCount: 1}, func(cfg *config.AgentConfig, ep *mockEndpoint) {
		requests := []request{
			<-ep.Requests,
			<-ep.Requests,
		}

		timestamps := make(map[string]struct{})
		for _, req := range requests {
			assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
			assert.Equal(t, cfg.APIEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))
			assert.Equal(t, "1", req.headers.Get(api.ContainerCountHeader))
			timestamps[req.headers.Get(api.TimestampHeader)] = struct{}{}

			reqBody, err := process.DecodeMessage(req.body)
			require.NoError(t, err)

			_, ok := reqBody.Body.(*process.CollectorProc)
			require.True(t, ok)
		}

		assert.Len(t, timestamps, 1)
		assert.Equal(t, "1", requests[0].headers.Get("X-DD-Agent-Attempts"))
		assert.Equal(t, "2", requests[1].headers.Get("X-DD-Agent-Attempts"))
	})
}

func TestRTProcMessageNotRetried(t *testing.T) {
	m := &process.CollectorRealTime{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.RTProcess.Name(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(false), &endpointConfig{ErrorCount: 1}, func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		_, ok := reqBody.Body.(*process.CollectorRealTime)
		require.True(t, ok)

		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))

		select {
		case <-ep.Requests:
			t.Fatalf("should not have received another request")
		case <-time.After(2 * time.Second):

		}
	})
}

func TestSendPodMessage(t *testing.T) {
	clusterID := "d801b2b1-4811-11ea-8618-121d4d0938a3"

	orig := os.Getenv("DD_ORCHESTRATOR_CLUSTER_ID")
	_ = os.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", clusterID)
	defer func() { _ = os.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", orig) }()

	m := &process.CollectorPod{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.Pod.Name(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(false), &endpointConfig{}, func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/orchestrator", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
		assert.Equal(t, cfg.OrchestratorEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "0", req.headers.Get(api.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get(api.TimestampHeader))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		cp, ok := reqBody.Body.(*process.CollectorPod)
		require.True(t, ok)

		assert.Equal(t, clusterID, req.headers.Get(api.ClusterIDHeader))
		assert.Equal(t, cfg.HostName, cp.HostName)
	})
}

func TestQueueSpaceNotAvailable(t *testing.T) {
	m := &process.CollectorRealTime{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.RTProcess.Name(),
		data: [][]process.MessageBody{{m}},
	}

	cfg := config.NewDefaultAgentConfig(false)
	cfg.ProcessQueueBytes = 1

	runCollectorTest(t, check, cfg, &endpointConfig{ErrorCount: 1}, func(cfg *config.AgentConfig, ep *mockEndpoint) {
		select {
		case r := <-ep.Requests:
			t.Fatalf("should not have received a request: %+v", r)
		case <-time.After(2 * time.Second):

		}
	})
}

// TestQueueSpaceReleased tests that queue space is released after sending a payload
func TestQueueSpaceReleased(t *testing.T) {
	m1 := &process.CollectorRealTime{
		HostName: testHostName,
		GroupId:  1,
	}

	m2 := &process.CollectorRealTime{
		HostName: testHostName,
		GroupId:  2,
	}

	check := &testCheck{
		name: checks.RTProcess.Name(),
		data: [][]process.MessageBody{{m1}, {m2}},
	}

	cfg := config.NewDefaultAgentConfig(false)
	cfg.ProcessQueueBytes = 50 // This should be enough for one message, but not both if the space isn't released

	runCollectorTest(t, check, cfg, &endpointConfig{ErrorCount: 1}, func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		body, ok := reqBody.Body.(*process.CollectorRealTime)
		require.True(t, ok)

		assert.Equal(t, int32(1), body.GroupId)

		req = <-ep.Requests

		reqBody, err = process.DecodeMessage(req.body)
		require.NoError(t, err)

		body, ok = reqBody.Body.(*process.CollectorRealTime)
		require.True(t, ok)

		assert.Equal(t, int32(2), body.GroupId)
	})
}

func runCollectorTest(t *testing.T, check checks.Check, cfg *config.AgentConfig, epConfig *endpointConfig, tc func(cfg *config.AgentConfig, ep *mockEndpoint)) {
	ep := newMockEndpoint(t, epConfig)
	collectorAddr, orchestratorAddr := ep.start()
	defer ep.stop()

	cfg.APIEndpoints = []api.Endpoint{{APIKey: "apiKey", Endpoint: collectorAddr}}
	cfg.OrchestratorEndpoints = []api.Endpoint{{APIKey: "orchestratorApiKey", Endpoint: orchestratorAddr}}
	cfg.HostName = testHostName
	cfg.CheckIntervals[check.Name()] = 500 * time.Millisecond

	exit := make(chan struct{})

	c := NewCollectorWithChecks(cfg, []checks.Check{check})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := c.run(exit)
		require.NoError(t, err)
	}()

	tc(cfg, ep)

	close(exit)
	wg.Wait()
}

type testCheck struct {
	name string
	data [][]process.MessageBody
}

func (t *testCheck) Init(_ *config.AgentConfig, _ *process.SystemInfo) {
}

func (t *testCheck) Name() string {
	return t.name
}

func (t *testCheck) RealTime() bool {
	return false
}

func (t *testCheck) Run(_ *config.AgentConfig, _ int32) ([]process.MessageBody, error) {
	if len(t.data) > 0 {
		result := t.data[0]
		t.data = t.data[1:]
		return result, nil
	}
	return nil, nil
}

var _ checks.Check = &testCheck{}

type request struct {
	headers http.Header
	uri     string
	body    []byte
}

type endpointConfig struct {
	ErrorCount int
}

type mockEndpoint struct {
	t                  *testing.T
	collectorServer    *http.Server
	orchestratorServer *http.Server
	stopper            sync.WaitGroup
	Requests           chan request
	errorCount         int
	errorsSent         int
	closeOnce          sync.Once
}

func newMockEndpoint(t *testing.T, config *endpointConfig) *mockEndpoint {
	m := &mockEndpoint{
		t:          t,
		errorCount: config.ErrorCount,
		Requests:   make(chan request, 1),
	}

	collectorMux := http.NewServeMux()
	collectorMux.HandleFunc("/api/v1/validate", m.handleValidate)
	collectorMux.HandleFunc("/api/v1/collector", m.handle)
	collectorMux.HandleFunc("/api/v1/container", m.handle)

	orchestratorMux := http.NewServeMux()
	orchestratorMux.HandleFunc("/api/v1/validate", m.handleValidate)
	orchestratorMux.HandleFunc("/api/v1/orchestrator", m.handle)

	m.collectorServer = &http.Server{Addr: ":", Handler: collectorMux}
	m.orchestratorServer = &http.Server{Addr: ":", Handler: orchestratorMux}

	return m
}

// start starts the http endpoints and returns (collector server url, orchestrator server url)
func (m *mockEndpoint) start() (*url.URL, *url.URL) {
	addrC := make(chan net.Addr, 1)

	m.stopper.Add(1)
	go func() {
		defer m.stopper.Done()

		listener, err := net.Listen("tcp", ":")
		require.NoError(m.t, err)

		addrC <- listener.Addr()

		_ = m.collectorServer.Serve(listener)
	}()

	collectorAddr := <-addrC

	m.stopper.Add(1)
	go func() {
		defer m.stopper.Done()

		listener, err := net.Listen("tcp", ":")
		require.NoError(m.t, err)

		addrC <- listener.Addr()

		_ = m.orchestratorServer.Serve(listener)
	}()

	orchestratorAddr := <-addrC

	close(addrC)

	collectorEndpoint, err := url.Parse(fmt.Sprintf("http://%s", collectorAddr.String()))
	require.NoError(m.t, err)

	orchestratorEndpoint, err := url.Parse(fmt.Sprintf("http://%s", orchestratorAddr.String()))
	require.NoError(m.t, err)

	return collectorEndpoint, orchestratorEndpoint
}

func (m *mockEndpoint) stop() {
	err := m.collectorServer.Close()
	require.NoError(m.t, err)

	err = m.orchestratorServer.Close()
	require.NoError(m.t, err)

	m.stopper.Wait()
	m.closeOnce.Do(func() {
		close(m.Requests)
	})
}

func (m *mockEndpoint) handleValidate(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (m *mockEndpoint) handle(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	require.NoError(m.t, err)

	err = req.Body.Close()
	require.NoError(m.t, err)

	m.Requests <- request{headers: req.Header, body: body, uri: req.RequestURI}

	if m.errorCount != m.errorsSent {
		w.WriteHeader(http.StatusInternalServerError)
		m.errorsSent++
		return
	}

	out, err := process.EncodeMessage(process.Message{
		Header: process.MessageHeader{
			Version:   process.MessageV3, // Intake normally returns v1 but the encoding in agent-payload only handles v3
			Encoding:  process.MessageEncodingProtobuf,
			Type:      process.MessageType(process.TypeResCollector),
			Timestamp: time.Now().Unix(),
		},
		Body: &process.ResCollector{
			Header: &process.ResCollector_Header{
				Type: process.TypeResCollector,
			},
			Message: "",
			Status: &process.CollectorStatus{
				ActiveClients: 0,
				Interval:      2,
			},
		},
	})
	require.NoError(m.t, err)

	w.WriteHeader(http.StatusAccepted)

	_, err = w.Write(out)
	require.NoError(m.t, err)
}
