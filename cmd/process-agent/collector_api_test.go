package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util/api"

	"github.com/DataDog/agent-payload/process"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendMessage(t *testing.T) {
	ep := newMockEndpoint(t)
	addr := ep.start()
	defer ep.stop()

	cfg := config.NewDefaultAgentConfig(false)
	cfg.APIEndpoints = []api.Endpoint{{APIKey: "apiKey", Endpoint: addr}}
	cfg.HostName = "test-host"

	exit := make(chan bool)

	c, err := NewCollector(cfg)
	require.NoError(t, err)

	go c.run(exit)
	defer func() { close(exit) }()

	m := &process.CollectorConnections{
		HostName: cfg.HostName,
		GroupId:  1,
	}

	c.send <- checkPayload{
		endpoint: "/api/v1/collector",
		name:     "connections",
		messages: []process.MessageBody{m},
	}

	req := <-ep.Requests

	assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
	assert.Equal(t, cfg.APIEndpoints[0].APIKey, req.headers.Get(api.APIKeyHeader))

	reqBody, err := process.DecodeMessage(req.body)
	require.NoError(t, err)

	cc, ok := reqBody.Body.(*process.CollectorConnections)
	require.True(t, ok)

	assert.Equal(t, cfg.HostName, cc.HostName)
	assert.Equal(t, int32(1), cc.GroupId)
}

func TestSendContainerMessage(t *testing.T) {
	ep := newMockEndpoint(t)
	addr := ep.start()
	defer ep.stop()

	cfg := config.NewDefaultAgentConfig(false)
	cfg.APIEndpoints = []api.Endpoint{{APIKey: "apiKey", Endpoint: addr}}
	cfg.HostName = "test-host"

	exit := make(chan bool)

	c, err := NewCollector(cfg)
	require.NoError(t, err)

	go c.run(exit)
	defer func() { close(exit) }()

	m := &process.CollectorContainer{
		HostName: cfg.HostName,
		GroupId:  1,
		Containers: []*process.Container{
			{Id: "1", Name: "foo"},
		},
	}

	c.send <- checkPayload{
		endpoint: "/api/v1/collector",
		name:     "container",
		messages: []process.MessageBody{m},
	}

	req := <-ep.Requests

	assert.Equal(t, cfg.HostName, req.headers.Get(api.HostHeader))
	assert.Equal(t, cfg.APIEndpoints[0].APIKey, req.headers.Get(api.APIKeyHeader))
	assert.Equal(t, "1", req.headers.Get(api.ContainerCountHeader))

	reqBody, err := process.DecodeMessage(req.body)
	require.NoError(t, err)

	_, ok := reqBody.Body.(*process.CollectorContainer)
	require.True(t, ok)
}

type request struct {
	headers http.Header
	body    []byte
}

type mockEndpoint struct {
	t        *testing.T
	server   *http.Server
	stopper  sync.WaitGroup
	Requests chan request
}

func newMockEndpoint(t *testing.T) *mockEndpoint {
	m := &mockEndpoint{
		t:        t,
		Requests: make(chan request, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/collector", m.handle)

	m.server = &http.Server{Addr: ":", Handler: mux}

	return m
}

func (m *mockEndpoint) start() *url.URL {
	addrC := make(chan net.Addr)

	m.stopper.Add(1)
	go func() {
		defer m.stopper.Done()

		ln, err := net.Listen("tcp", ":")
		require.NoError(m.t, err)

		addrC <- ln.Addr()

		_ = m.server.Serve(ln)
	}()

	addr := <-addrC

	endpoint, err := url.Parse(fmt.Sprintf("http://%s", addr.String()))
	require.NoError(m.t, err)
	return endpoint
}

func (m *mockEndpoint) stop() {
	err := m.server.Close()
	require.NoError(m.t, err)

	m.stopper.Wait()
	close(m.Requests)
}

func (m *mockEndpoint) handle(w http.ResponseWriter, req *http.Request) {
	body, err := ioutil.ReadAll(req.Body)
	require.NoError(m.t, err)

	err = req.Body.Close()
	require.NoError(m.t, err)

	m.Requests <- request{headers: req.Header, body: body}

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
