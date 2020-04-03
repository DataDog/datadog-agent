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

func TestSendConnectionsMessage(t *testing.T) {
	runCollectorTest(t, func(payloads chan checkPayload, cfg *config.AgentConfig, ep *mockEndpoint) {
		m := &process.CollectorConnections{
			HostName: cfg.HostName,
			GroupId:  1,
		}

		payloads <- checkPayload{
			name:     checks.Connections.Name(),
			messages: []process.MessageBody{m},
		}

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
	runCollectorTest(t, func(payloads chan checkPayload, cfg *config.AgentConfig, ep *mockEndpoint) {
		m := &process.CollectorContainer{
			HostName: cfg.HostName,
			GroupId:  1,
			Containers: []*process.Container{
				{Id: "1", Name: "foo"},
			},
		}

		payloads <- checkPayload{
			name:     checks.Container.Name(),
			messages: []process.MessageBody{m},
		}

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
	runCollectorTest(t, func(payloads chan checkPayload, cfg *config.AgentConfig, ep *mockEndpoint) {
		m := &process.CollectorProc{
			HostName: cfg.HostName,
			GroupId:  1,
			Containers: []*process.Container{
				{Id: "1", Name: "foo"},
			},
		}

		payloads <- checkPayload{
			name:     checks.Process.Name(),
			messages: []process.MessageBody{m},
		}

		req := <-ep.Requests

		assert.Equal(t, "/api/v1/collector", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
		assert.Equal(t, cfg.APIEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "1", req.headers.Get(api.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get("X-DD-Agent-Timestamp"))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		_, ok := reqBody.Body.(*process.CollectorProc)
		require.True(t, ok)
	})
}

func TestSendProcMessageWithRetry(t *testing.T) {
	runCollectorTest(t, func(payloads chan checkPayload, cfg *config.AgentConfig, ep *mockEndpoint) {
		ep.ErrorCount = 1

		m := &process.CollectorProc{
			HostName: cfg.HostName,
			GroupId:  1,
			Containers: []*process.Container{
				{Id: "1", Name: "foo"},
			},
		}

		payloads <- checkPayload{
			name:     checks.Process.Name(),
			messages: []process.MessageBody{m},
		}

		requests := []request{
			<-ep.Requests,
			<-ep.Requests,
		}

		timestamps := make(map[string]struct{})
		for _, req := range requests {
			assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
			assert.Equal(t, cfg.APIEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))
			assert.Equal(t, "1", req.headers.Get(api.ContainerCountHeader))
			timestamps[req.headers.Get("X-DD-Agent-Timestamp")] = struct{}{}

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
	runCollectorTest(t, func(payloads chan checkPayload, cfg *config.AgentConfig, ep *mockEndpoint) {
		ep.ErrorCount = 1

		m := &process.CollectorRealTime{
			HostName: cfg.HostName,
			GroupId:  1,
		}

		payloads <- checkPayload{
			name:     checks.RTProcess.Name(),
			messages: []process.MessageBody{m},
		}

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
	runCollectorTest(t, func(payloads chan checkPayload, cfg *config.AgentConfig, ep *mockEndpoint) {
		m := &process.CollectorPod{
			HostName: cfg.HostName,
			GroupId:  1,
		}

		payloads <- checkPayload{
			name:     checks.Pod.Name(),
			messages: []process.MessageBody{m},
		}

		clusterID := "d801b2b1-4811-11ea-8618-121d4d0938a3"

		orig := os.Getenv("DD_ORCHESTRATOR_CLUSTER_ID")
		_ = os.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", clusterID)
		defer func() { _ = os.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", orig) }()

		req := <-ep.Requests

		assert.Equal(t, "/api/v1/orchestrator", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
		assert.Equal(t, cfg.OrchestratorEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "0", req.headers.Get(api.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get("X-DD-Agent-Timestamp"))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		cp, ok := reqBody.Body.(*process.CollectorPod)
		require.True(t, ok)

		assert.Equal(t, clusterID, req.headers.Get(api.ClusterIDHeader))
		assert.Equal(t, cfg.HostName, cp.HostName)
	})
}

func runCollectorTest(t *testing.T, tc func(payloads chan checkPayload, cfg *config.AgentConfig, ep *mockEndpoint)) {
	ep := newMockEndpoint(t)
	collectorAddr, orchestratorAddr := ep.start()
	defer ep.stop()

	cfg := config.NewDefaultAgentConfig(false)
	cfg.APIEndpoints = []api.Endpoint{{APIKey: "apiKey", Endpoint: collectorAddr}}
	cfg.OrchestratorEndpoints = []api.Endpoint{{APIKey: "orchestratorApiKey", Endpoint: orchestratorAddr}}
	cfg.HostName = "test-host"

	exit := make(chan bool)

	c, err := NewCollector(cfg)
	require.NoError(t, err)

	go func() {
		err := c.run(exit)
		require.NoError(t, err)
	}()
	defer func() { close(exit) }()

	tc(c.send, cfg, ep)
}

type request struct {
	headers http.Header
	uri     string
	body    []byte
}

type mockEndpoint struct {
	t                  *testing.T
	collectorServer    *http.Server
	orchestratorServer *http.Server
	stopper            sync.WaitGroup
	Requests           chan request
	ErrorCount         int
	errorsSent         int
	closeOnce          sync.Once
}

func newMockEndpoint(t *testing.T) *mockEndpoint {
	m := &mockEndpoint{
		t:        t,
		Requests: make(chan request, 1),
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

	if m.ErrorCount != m.errorsSent {
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
