// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	"github.com/DataDog/agent-payload/v5/process"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHostName = "test-host"

func setProcessEndpointsForTest(config ddconfig.Config, eps ...apicfg.Endpoint) {
	additionalEps := make(map[string][]string)
	for i, ep := range eps {
		if i == 0 {
			config.Set("api_key", ep.APIKey)
			config.Set("process_config.process_dd_url", ep.Endpoint)
		} else {
			additionalEps[ep.Endpoint.String()] = append(additionalEps[ep.Endpoint.String()], ep.APIKey)
		}
	}
	config.Set("process_config.additional_endpoints", additionalEps)
}

func TestSendConnectionsMessage(t *testing.T) {
	m := &process.CollectorConnections{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.Connections.Name(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(), &endpointConfig{}, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/collector", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(headers.HostHeader))
		apiEps, err := getAPIEndpoints()
		assert.NoError(t, err)
		assert.Equal(t, apiEps[0].APIKey, req.headers.Get("DD-Api-Key"))

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

	runCollectorTest(t, check, config.NewDefaultAgentConfig(), &endpointConfig{}, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/container", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(headers.HostHeader))
		eps, err := getAPIEndpoints()
		assert.NoError(t, err)
		assert.Equal(t, eps[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "1", req.headers.Get(headers.ContainerCountHeader))

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

	runCollectorTest(t, check, config.NewDefaultAgentConfig(), &endpointConfig{}, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/collector", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(headers.HostHeader))
		eps, err := getAPIEndpoints()
		assert.NoError(t, err)
		assert.Equal(t, eps[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "1", req.headers.Get(headers.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get(headers.TimestampHeader))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		_, ok := reqBody.Body.(*process.CollectorProc)
		require.True(t, ok)
	})
}

func TestSendProcessDiscoveryMessage(t *testing.T) {
	m := &process.CollectorProcDiscovery{
		HostName:  testHostName,
		GroupId:   1,
		GroupSize: 1,
		ProcessDiscoveries: []*process.ProcessDiscovery{
			{Pid: 1, NsPid: 2, CreateTime: time.Now().Unix()},
		},
	}

	check := &testCheck{
		name: checks.ProcessDiscovery.Name(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(), &endpointConfig{}, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/discovery", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(headers.HostHeader))
		eps, err := getAPIEndpoints()
		assert.NoError(t, err)
		assert.Equal(t, eps[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "0", req.headers.Get(headers.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get(headers.TimestampHeader))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		b, ok := reqBody.Body.(*process.CollectorProcDiscovery)
		require.True(t, ok)
		assert.Equal(t, m, b)
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

	runCollectorTest(t, check, config.NewDefaultAgentConfig(), &endpointConfig{ErrorCount: 1}, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
		requests := []request{
			<-ep.Requests,
			<-ep.Requests,
		}

		timestamps := make(map[string]struct{})
		for _, req := range requests {
			assert.Equal(t, cfg.HostName, req.headers.Get(headers.HostHeader))
			eps, err := getAPIEndpoints()
			assert.NoError(t, err)
			assert.Equal(t, eps[0].APIKey, req.headers.Get("DD-Api-Key"))
			assert.Equal(t, "1", req.headers.Get(headers.ContainerCountHeader))
			timestamps[req.headers.Get(headers.TimestampHeader)] = struct{}{}

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
		name: checks.Process.RealTimeName(),
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, config.NewDefaultAgentConfig(), &endpointConfig{ErrorCount: 1}, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
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

	cfg := config.NewDefaultAgentConfig()
	cfg.Orchestrator.OrchestrationCollectionEnabled = true

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

	runCollectorTest(t, check, cfg, &endpointConfig{}, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v2/orch", req.uri)

		assert.Equal(t, cfg.HostName, req.headers.Get(headers.HostHeader))
		assert.Equal(t, cfg.Orchestrator.OrchestratorEndpoints[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "0", req.headers.Get(headers.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get(headers.TimestampHeader))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		cp, ok := reqBody.Body.(*process.CollectorPod)
		require.True(t, ok)

		assert.Equal(t, clusterID, req.headers.Get(headers.ClusterIDHeader))
		assert.Equal(t, cfg.HostName, cp.HostName)
	})
}

func TestQueueSpaceNotAvailable(t *testing.T) {
	m := &process.CollectorRealTime{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.Process.RealTimeName(),
		data: [][]process.MessageBody{{m}},
	}

	mockConfig := ddconfig.Mock(t)
	mockConfig.Set("process_config.process_queue_bytes", 1)
	cfg := config.NewDefaultAgentConfig()

	runCollectorTest(t, check, cfg, &endpointConfig{ErrorCount: 1}, mockConfig, func(cfg *config.AgentConfig, ep *mockEndpoint) {
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
		name: checks.Process.RealTimeName(),
		data: [][]process.MessageBody{{m1}, {m2}},
	}

	mockConfig := ddconfig.Mock(t)
	mockConfig.Set("process_config.process_queue_bytes", 50) // This should be enough for one message, but not both if the space isn't released
	cfg := config.NewDefaultAgentConfig()

	runCollectorTest(t, check, cfg, &endpointConfig{ErrorCount: 1}, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
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

func TestMultipleAPIKeys(t *testing.T) {
	m := &process.CollectorConnections{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.Connections.Name(),
		data: [][]process.MessageBody{{m}},
	}

	cfg := config.NewDefaultAgentConfig()
	apiKeys := []string{"apiKeyI", "apiKeyII", "apiKeyIII"}
	orchKeys := []string{"orchKey"}

	runCollectorTestWithAPIKeys(t, check, cfg, &endpointConfig{}, apiKeys, orchKeys, ddconfig.Mock(t), func(cfg *config.AgentConfig, ep *mockEndpoint) {
		for _, expectedAPIKey := range apiKeys {
			request := <-ep.Requests
			assert.Equal(t, expectedAPIKey, request.headers.Get("DD-Api-Key"))
		}
	})
}

func runCollectorTest(t *testing.T, check checks.Check, cfg *config.AgentConfig, epConfig *endpointConfig, mockConfig ddconfig.Config, tc func(cfg *config.AgentConfig, ep *mockEndpoint)) {
	runCollectorTestWithAPIKeys(t, check, cfg, epConfig, []string{"apiKey"}, []string{"orchestratorApiKey"}, mockConfig, tc)
}

func runCollectorTestWithAPIKeys(t *testing.T, check checks.Check, cfg *config.AgentConfig, epConfig *endpointConfig, apiKeys, orchAPIKeys []string, mockConfig ddconfig.Config, tc func(cfg *config.AgentConfig, ep *mockEndpoint)) {
	ep := newMockEndpoint(t, epConfig)
	collectorAddr, orchestratorAddr := ep.start()
	defer ep.stop()

	var eps []apicfg.Endpoint
	for _, key := range apiKeys {
		eps = append(eps, apicfg.Endpoint{APIKey: key, Endpoint: collectorAddr})
	}
	setProcessEndpointsForTest(mockConfig, eps...)

	cfg.Orchestrator.OrchestratorEndpoints = make([]apicfg.Endpoint, len(orchAPIKeys))
	for index, key := range orchAPIKeys {
		cfg.Orchestrator.OrchestratorEndpoints[index] = apicfg.Endpoint{APIKey: key, Endpoint: orchestratorAddr}
	}

	cfg.HostName = testHostName
	cfg.CheckIntervals[check.Name()] = 500 * time.Millisecond

	exit := make(chan struct{})

	c := NewCollectorWithChecks(cfg, []checks.Check{check}, true)

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

func (t *testCheck) Cleanup() {}

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
	collectorMux.HandleFunc("/api/v1/discovery", m.handle)

	orchestratorMux := http.NewServeMux()
	orchestratorMux.HandleFunc("/api/v1/validate", m.handleValidate)
	orchestratorMux.HandleFunc("/api/v2/orch", m.handle)

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
