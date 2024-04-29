// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package runner

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/process"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHostName = "test-host"

func setProcessEndpointsForTest(config ddconfig.Config, eps ...apicfg.Endpoint) {
	additionalEps := make(map[string][]string)
	for i, ep := range eps {
		if i == 0 {
			config.SetWithoutSource("api_key", ep.APIKey)
			config.SetWithoutSource("process_config.process_dd_url", ep.Endpoint)
		} else {
			additionalEps[ep.Endpoint.String()] = append(additionalEps[ep.Endpoint.String()], ep.APIKey)
		}
	}
	config.SetWithoutSource("process_config.additional_endpoints", additionalEps)
}

func setProcessEventsEndpointsForTest(config ddconfig.Config, eps ...apicfg.Endpoint) {
	additionalEps := make(map[string][]string)
	for i, ep := range eps {
		if i == 0 {
			config.SetWithoutSource("api_key", ep.APIKey)
			config.SetWithoutSource("process_config.events_dd_url", ep.Endpoint)
		} else {
			additionalEps[ep.Endpoint.String()] = append(additionalEps[ep.Endpoint.String()], ep.APIKey)
		}
	}
	config.SetWithoutSource("process_config.events_additional_endpoints", additionalEps)
}

func TestSendConnectionsMessage(t *testing.T) {
	m := &process.CollectorConnections{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.ConnectionsCheckName,
		data: [][]process.MessageBody{{m}},
	}

	cfg := ddconfig.Mock(t)
	runCollectorTest(t, check, &endpointConfig{}, cfg, func(c *CheckRunner, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/connections", req.uri)

		assert.Equal(t, testHostName, req.headers.Get(headers.HostHeader))
		apiEps, err := endpoint.GetAPIEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, apiEps[0].APIKey, req.headers.Get("DD-Api-Key"))

		// process-events specific headers should not be set
		assert.Equal(t, "", req.headers.Get(headers.EVPOriginHeader))
		assert.Equal(t, "", req.headers.Get(headers.EVPOriginVersionHeader))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		cc, ok := reqBody.Body.(*process.CollectorConnections)
		require.True(t, ok)

		assert.Equal(t, testHostName, cc.HostName)
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
		name: checks.ContainerCheckName,
		data: [][]process.MessageBody{{m}},
	}

	cfg := ddconfig.Mock(t)
	runCollectorTest(t, check, &endpointConfig{}, cfg, func(c *CheckRunner, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/container", req.uri)

		assert.Equal(t, testHostName, req.headers.Get(headers.HostHeader))
		eps, err := endpoint.GetAPIEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, eps[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "1", req.headers.Get(headers.ContainerCountHeader))

		// process-events specific headers should not be set
		assert.Equal(t, "", req.headers.Get(headers.EVPOriginHeader))
		assert.Equal(t, "", req.headers.Get(headers.EVPOriginVersionHeader))

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
		name: checks.ProcessCheckName,
		data: [][]process.MessageBody{{m}},
	}

	cfg := ddconfig.Mock(t)
	runCollectorTest(t, check, &endpointConfig{}, cfg, func(c *CheckRunner, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/collector", req.uri)

		assert.Equal(t, testHostName, req.headers.Get(headers.HostHeader))
		eps, err := endpoint.GetAPIEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, eps[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "1", req.headers.Get(headers.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get(headers.TimestampHeader))

		// process-events specific headers should not be set
		assert.Equal(t, "", req.headers.Get(headers.EVPOriginHeader))
		assert.Equal(t, "", req.headers.Get(headers.EVPOriginVersionHeader))

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
		name: checks.DiscoveryCheckName,
		data: [][]process.MessageBody{{m}},
	}

	cfg := ddconfig.Mock(t)
	runCollectorTest(t, check, &endpointConfig{}, cfg, func(c *CheckRunner, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v1/discovery", req.uri)

		assert.Equal(t, testHostName, req.headers.Get(headers.HostHeader))
		eps, err := endpoint.GetAPIEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, eps[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "0", req.headers.Get(headers.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get(headers.TimestampHeader))

		// process-events specific headers should not be set
		assert.Equal(t, "", req.headers.Get(headers.EVPOriginHeader))
		assert.Equal(t, "", req.headers.Get(headers.EVPOriginVersionHeader))

		reqBody, err := process.DecodeMessage(req.body)
		require.NoError(t, err)

		b, ok := reqBody.Body.(*process.CollectorProcDiscovery)
		require.True(t, ok)
		assert.Equal(t, m, b)
	})
}

func TestSendProcessEventMessage(t *testing.T) {
	m := &process.CollectorProcEvent{
		Hostname:  testHostName,
		GroupId:   1,
		GroupSize: 1,
		Events: []*process.ProcessEvent{
			{
				Type: process.ProcEventType_exec,
				Pid:  42,
				Command: &process.Command{
					Exe:  "/usr/bin/curl",
					Args: []string{"curl", "localhost:6062/debug/vars"},
					Ppid: 1,
				},
			},
		},
	}

	check := &testCheck{
		name: checks.ProcessEventsCheckName,
		data: [][]process.MessageBody{{m}},
	}

	cfg := ddconfig.Mock(t)
	runCollectorTest(t, check, &endpointConfig{}, cfg, func(c *CheckRunner, ep *mockEndpoint) {
		req := <-ep.Requests

		assert.Equal(t, "/api/v2/proclcycle", req.uri)

		assert.Equal(t, testHostName, req.headers.Get(headers.HostHeader))
		eps, err := endpoint.GetEventsAPIEndpoints(cfg)
		assert.NoError(t, err)
		assert.Equal(t, eps[0].APIKey, req.headers.Get("DD-Api-Key"))
		assert.Equal(t, "0", req.headers.Get(headers.ContainerCountHeader))
		assert.Equal(t, "1", req.headers.Get("X-DD-Agent-Attempts"))
		assert.NotEmpty(t, req.headers.Get(headers.TimestampHeader))
		assert.Equal(t, headers.ProtobufContentType, req.headers.Get(headers.ContentTypeHeader))

		agentVersion, err := version.Agent()
		require.NoError(t, err)
		assert.Equal(t, agentVersion.GetNumber(), req.headers.Get(headers.ProcessVersionHeader))

		// Check events-specific headers
		assert.Equal(t, "process-agent", req.headers.Get(headers.EVPOriginHeader))
		assert.Equal(t, version.AgentVersion, req.headers.Get(headers.EVPOriginVersionHeader))

		// ProcessEvents payloads are encoded as plain protobuf
		msg := &process.CollectorProcEvent{}
		err = proto.Unmarshal(req.body, msg)
		require.NoError(t, err)
		assert.Equal(t, m, msg)
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
		name: checks.ProcessCheckName,
		data: [][]process.MessageBody{{m}},
	}

	cfg := ddconfig.Mock(t)
	runCollectorTest(t, check, &endpointConfig{ErrorCount: 1}, cfg, func(c *CheckRunner, ep *mockEndpoint) {
		requests := []request{
			<-ep.Requests,
			<-ep.Requests,
		}

		timestamps := make(map[string]struct{})
		for _, req := range requests {
			assert.Equal(t, testHostName, req.headers.Get(headers.HostHeader))
			eps, err := endpoint.GetAPIEndpoints(cfg)
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
		name: checks.RTProcessCheckName,
		data: [][]process.MessageBody{{m}},
	}

	runCollectorTest(t, check, &endpointConfig{ErrorCount: 1}, ddconfig.Mock(t), func(c *CheckRunner, ep *mockEndpoint) {
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

func TestQueueSpaceNotAvailable(t *testing.T) {
	m := &process.CollectorRealTime{
		HostName: testHostName,
		GroupId:  1,
	}

	check := &testCheck{
		name: checks.RTProcessCheckName,
		data: [][]process.MessageBody{{m}},
	}

	mockConfig := ddconfig.Mock(t)
	mockConfig.SetWithoutSource("process_config.process_queue_bytes", 1)

	runCollectorTest(t, check, &endpointConfig{ErrorCount: 1}, mockConfig, func(_ *CheckRunner, ep *mockEndpoint) {
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
		name: checks.RTProcessCheckName,
		data: [][]process.MessageBody{{m1}, {m2}},
	}

	mockConfig := ddconfig.Mock(t)
	mockConfig.SetWithoutSource("process_config.process_queue_bytes", 50) // This should be enough for one message, but not both if the space isn't released

	runCollectorTest(t, check, &endpointConfig{ErrorCount: 1}, ddconfig.Mock(t), func(_ *CheckRunner, ep *mockEndpoint) {
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
		name: checks.ConnectionsCheckName,
		data: [][]process.MessageBody{{m}},
	}

	apiKeys := []string{"apiKeyI", "apiKeyII", "apiKeyIII"}

	runCollectorTestWithAPIKeys(t, check, &endpointConfig{}, apiKeys, ddconfig.Mock(t), func(_ *CheckRunner, ep *mockEndpoint) {
		for _, expectedAPIKey := range apiKeys {
			request := <-ep.Requests
			assert.Equal(t, expectedAPIKey, request.headers.Get("DD-Api-Key"))
		}
	})
}

func runCollectorTest(t *testing.T, check checks.Check, epConfig *endpointConfig, mockConfig ddconfig.Config, tc func(c *CheckRunner, ep *mockEndpoint)) {
	runCollectorTestWithAPIKeys(t, check, epConfig, []string{"apiKey"}, mockConfig, tc)
}

func runCollectorTestWithAPIKeys(t *testing.T, check checks.Check, epConfig *endpointConfig, apiKeys []string, mockConfig ddconfig.Config, tc func(c *CheckRunner, ep *mockEndpoint)) {
	ep := newMockEndpoint(t, epConfig)
	collectorAddr, eventsAddr := ep.start()
	defer ep.stop()

	eps := make([]apicfg.Endpoint, 0, len(apiKeys))
	for _, key := range apiKeys {
		eps = append(eps, apicfg.Endpoint{APIKey: key, Endpoint: collectorAddr})
	}
	setProcessEndpointsForTest(mockConfig, eps...)

	eventsEps := make([]apicfg.Endpoint, 0, len(apiKeys))
	for _, key := range apiKeys {
		eventsEps = append(eventsEps, apicfg.Endpoint{APIKey: key, Endpoint: eventsAddr})
	}
	setProcessEventsEndpointsForTest(mockConfig, eventsEps...)

	hostInfo := &checks.HostInfo{
		HostName: testHostName,
	}
	c, err := NewRunnerWithChecks(mockConfig, nil, hostInfo, []checks.Check{check}, true, nil)
	assert.NoError(t, err)
	err = check.Init(nil, hostInfo, true)
	assert.NoError(t, err)
	deps := newSubmitterDepsWithConfig(t, mockConfig)
	submitter, err := NewSubmitter(mockConfig, deps.Log, deps.Forwarders, hostInfo.HostName)
	c.Submitter = submitter
	require.NoError(t, err)

	err = submitter.Start()
	require.NoError(t, err)
	defer submitter.Stop()

	err = c.Run()
	require.NoError(t, err)
	defer c.Stop()

	tc(c, ep)
}

type testCheck struct {
	name string
	data [][]process.MessageBody
}

func (t *testCheck) Init(_ *checks.SysProbeConfig, _ *checks.HostInfo, _ bool) error {
	return nil
}

func (t *testCheck) Name() string {
	return t.name
}

func (t *testCheck) IsEnabled() bool {
	return true
}

func (t *testCheck) SupportsRunOptions() bool {
	return false
}

func (t *testCheck) Realtime() bool {
	return false
}

func (t *testCheck) ShouldSaveLastRun() bool {
	return false
}

func (t *testCheck) Run(_ func() int32, _ *checks.RunOptions) (checks.RunResult, error) {
	if len(t.data) > 0 {
		result := checks.StandardRunResult(t.data[0])
		t.data = t.data[1:]
		return result, nil
	}
	return checks.StandardRunResult{}, nil
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
	t               *testing.T
	collectorServer *http.Server
	eventsServer    *http.Server
	stopper         sync.WaitGroup
	Requests        chan request
	errorCount      int
	errorsSent      int
	closeOnce       sync.Once
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
	collectorMux.HandleFunc("/api/v1/connections", m.handle)
	collectorMux.HandleFunc("/api/v1/container", m.handle)
	collectorMux.HandleFunc("/api/v1/discovery", m.handle)

	eventsMux := http.NewServeMux()
	eventsMux.HandleFunc("/api/v2/proclcycle", m.handleEvents)

	m.collectorServer = &http.Server{Addr: "127.0.0.1:", Handler: collectorMux}
	m.eventsServer = &http.Server{Addr: "127.0.0.1:", Handler: eventsMux}

	return m
}

// start starts the http endpoints and returns (collector server url)
func (m *mockEndpoint) start() (*url.URL, *url.URL) {
	addrC := make(chan net.Addr, 1)

	m.stopper.Add(1)
	go func() {
		defer m.stopper.Done()

		listener, err := net.Listen("tcp", "127.0.0.1:")
		require.NoError(m.t, err)

		addrC <- listener.Addr()

		_ = m.collectorServer.Serve(listener)
	}()

	collectorAddr := <-addrC

	m.stopper.Add(1)
	go func() {
		defer m.stopper.Done()

		listener, err := net.Listen("tcp", "127.0.0.1:")
		require.NoError(m.t, err)

		addrC <- listener.Addr()

		_ = m.eventsServer.Serve(listener)
	}()

	eventsAddr := <-addrC

	close(addrC)

	collectorEndpoint, err := url.Parse(fmt.Sprintf("http://%s", collectorAddr.String()))
	require.NoError(m.t, err)

	eventsEndpoint, err := url.Parse(fmt.Sprintf("http://%s", eventsAddr.String()))
	require.NoError(m.t, err)

	return collectorEndpoint, eventsEndpoint
}

func (m *mockEndpoint) stop() {
	err := m.collectorServer.Close()
	require.NoError(m.t, err)

	err = m.eventsServer.Close()
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
	body, err := io.ReadAll(req.Body)
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

func (m *mockEndpoint) handleEvents(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	require.NoError(m.t, err)

	err = req.Body.Close()
	require.NoError(m.t, err)

	m.Requests <- request{headers: req.Header, body: body, uri: req.RequestURI}

	if m.errorCount != m.errorsSent {
		w.WriteHeader(http.StatusInternalServerError)
		m.errorsSent++
		return
	}

	w.WriteHeader(http.StatusAccepted)

	// process-events endpoint returns an empty body for valid posted payloads
	_, err = w.Write([]byte{})
	require.NoError(m.t, err)
}
