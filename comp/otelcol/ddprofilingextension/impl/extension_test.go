// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextension defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	ossconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/ptrace"

	log "github.com/DataDog/datadog-agent/comp/core/log/impl"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	otlpattributes "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	otlpsource "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
)

type testComponent struct {
	*pkgagent.Agent
}

func (c testComponent) SetOTelAttributeTranslator(attrstrans *otlpattributes.Translator) {
	c.Agent.OTLPReceiver.SetOTelAttributeTranslator(attrstrans)
}

func (c testComponent) ReceiveOTLPSpans(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header, hostFromAttributesHandler otlpattributes.HostFromAttributesHandler) (otlpsource.Source, error) {
	return c.Agent.OTLPReceiver.ReceiveResourceSpans(ctx, rspans, httpHeader, hostFromAttributesHandler)
}

func (c testComponent) SendStatsPayload(p *pb.StatsPayload) {
	c.Agent.StatsWriter.SendPayload(p)
}

func (c testComponent) GetHTTPHandler(endpoint string) http.Handler {
	c.Agent.Receiver.BuildHandlers()
	if v, ok := c.Agent.Receiver.Handlers[endpoint]; ok {
		return v
	}
	return nil
}

type hostWithExtensions struct {
	component.Host
	exts map[component.ID]component.Component
}

func newHostWithExtensions(exts map[component.ID]component.Component) component.Host {
	return &hostWithExtensions{
		Host: componenttest.NewNopHost(),
		exts: exts,
	}
}

func (h *hostWithExtensions) GetExtensions() map[component.ID]component.Component {
	return h.exts
}

func TestNewExtension(t *testing.T) {
	ext, err := NewExtension(&Config{}, component.BuildInfo{}, testComponent{}, log.NewTemporaryLoggerWithoutInit(), nil)
	assert.NoError(t, err)

	_, ok := ext.(*ddExtension)
	require.True(t, ok)
}

func testServer(t *testing.T, got chan string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", req.Header.Get("DD-Api-Key"))
		got <- req.Header.Get("User-Agent")
		rw.WriteHeader(http.StatusAccepted)
	}))
}

func TestAgentExtension(t *testing.T) {
	// fake intake
	got := make(chan string, 1)
	server := testServer(t, got)
	defer server.Close()

	// create agent
	tcfg := config.New()
	tcfg.ReceiverEnabled = false
	tcfg.Endpoints[0].APIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tcfg.DecoderTimeout = 10000
	tcfg.ProfilingProxy = config.ProfilingProxyConfig{DDURL: server.URL}
	ctx := context.Background()
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{}, gzip.NewComponent())

	// create extension
	ext, err := NewExtension(&Config{
		ProfilerOptions: ProfilerOptions{
			Period: 1,
		},
	}, component.BuildInfo{}, testComponent{traceagent}, log.NewTemporaryLoggerWithoutInit(), nil)
	assert.NoError(t, err)

	ext, ok := ext.(*ddExtension)
	require.True(t, ok)

	host := newHostWithExtensions(
		map[component.ID]component.Component{
			component.MustNewIDWithName("ddprofiling", "custom"): nil,
		},
	)

	err = ext.Start(context.Background(), host)
	assert.NoError(t, err)

	timeout := time.After(15 * time.Second)
	select {
	case out := <-got:
		assert.Equal(t, "Go-http-client/1.1", out)
	case <-timeout:
		t.Fatal("Timed out")
	}
	err = ext.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestOSSExtension(t *testing.T) {
	// fake intake
	got := make(chan string, 1)
	server := testServer(t, got)
	defer server.Close()

	// create extension
	os.Setenv("DD_PROFILING_URL", server.URL)
	defer os.Unsetenv("DD_PROFILING_URL")
	ext, err := NewExtension(&Config{
		API: ossconfig.APIConfig{
			Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		ProfilerOptions: ProfilerOptions{
			Period: 1,
		},
	}, component.BuildInfo{}, nil, log.NewTemporaryLoggerWithoutInit(), &fargateSourceProvider{})
	assert.NoError(t, err)

	host := newHostWithExtensions(
		map[component.ID]component.Component{
			component.MustNewIDWithName("ddprofiling", "custom"): nil,
		},
	)

	err = ext.Start(context.Background(), host)
	assert.NoError(t, err)

	extt, ok := ext.(*ddExtension)
	assert.True(t, ok)
	assert.Equal(t, nil, extt.traceAgent)
	var unitializedHTTPServer *http.Server
	assert.Equal(t, unitializedHTTPServer, extt.server)

	timeout := time.After(15 * time.Second)
	select {
	case out := <-got:
		assert.Equal(t, "Go-http-client/1.1", out)
	case <-timeout:
		t.Fatal("Timed out")
	}
	err = ext.Shutdown(context.Background())
	assert.NoError(t, err)
}

type fargateSourceProvider struct{}

func (*fargateSourceProvider) Source(_ context.Context) (otlpsource.Source, error) {
	return otlpsource.Source{
		Kind:       "task_arn",
		Identifier: "arn:aws:ecs:us-east-1:123456789012:cluster/default",
	}, nil
}

func TestOSSExtensionFargate(t *testing.T) {
	// fake intake
	got := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		got <- req.Header.Get(additionalTagsHeader)
		rw.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	// create extension
	os.Setenv("DD_PROFILING_URL", server.URL)
	defer os.Unsetenv("DD_PROFILING_URL")
	ext, err := NewExtension(&Config{
		API: ossconfig.APIConfig{
			Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		ProfilerOptions: ProfilerOptions{
			Period: 1,
		},
	}, component.BuildInfo{}, nil, log.NewTemporaryLoggerWithoutInit(), &fargateSourceProvider{})
	assert.NoError(t, err)

	host := newHostWithExtensions(
		map[component.ID]component.Component{
			component.MustNewIDWithName("ddprofiling", "custom"): nil,
		},
	)

	err = ext.Start(context.Background(), host)
	assert.NoError(t, err)

	extt, ok := ext.(*ddExtension)
	assert.True(t, ok)
	assert.Equal(t, nil, extt.traceAgent)
	var unitializedHTTPServer *http.Server
	assert.Equal(t, unitializedHTTPServer, extt.server)

	timeout := time.After(15 * time.Second)
	select {
	case out := <-got:
		assert.Equal(t, "agent_version:7.64.0,source:oss-ddprofilingextension,orchestrator:fargate_ecs,task_arn:arn:aws:ecs:us-east-1:123456789012:cluster/default", out)
	case <-timeout:
		t.Fatal("Timed out")
	}
	err = ext.Shutdown(context.Background())
	assert.NoError(t, err)
}

type hostSourceProvider struct{}

func (*hostSourceProvider) Source(_ context.Context) (otlpsource.Source, error) {
	return otlpsource.Source{
		Kind:       "host",
		Identifier: "i-123456789",
	}, nil
}

func TestOSSExtensionHost(t *testing.T) {
	// fake intake
	got := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		got <- req.Header.Get(additionalTagsHeader)
		rw.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	// create extension
	os.Setenv("DD_PROFILING_URL", server.URL)
	defer os.Unsetenv("DD_PROFILING_URL")
	ext, err := NewExtension(&Config{
		API: ossconfig.APIConfig{
			Key: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		ProfilerOptions: ProfilerOptions{
			Period: 1,
		},
	}, component.BuildInfo{}, nil, log.NewTemporaryLoggerWithoutInit(), &hostSourceProvider{})
	assert.NoError(t, err)

	host := newHostWithExtensions(
		map[component.ID]component.Component{
			component.MustNewIDWithName("ddprofiling", "custom"): nil,
		},
	)

	err = ext.Start(context.Background(), host)
	assert.NoError(t, err)

	extt, ok := ext.(*ddExtension)
	assert.True(t, ok)
	assert.Equal(t, nil, extt.traceAgent)
	var unitializedHTTPServer *http.Server
	assert.Equal(t, unitializedHTTPServer, extt.server)

	timeout := time.After(15 * time.Second)
	select {
	case out := <-got:
		assert.Equal(t, "agent_version:7.64.0,source:oss-ddprofilingextension,host:i-123456789", out)
	case <-timeout:
		t.Fatal("Timed out")
	}
	err = ext.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestOSSExtensionNoAPIKey(t *testing.T) {
	ext, err := NewExtension(&Config{}, component.BuildInfo{}, nil, log.NewTemporaryLoggerWithoutInit(), nil)
	assert.NoError(t, err)

	host := newHostWithExtensions(
		map[component.ID]component.Component{
			component.MustNewIDWithName("ddprofiling", "custom"): nil,
		},
	)

	err = ext.Start(context.Background(), host)
	assert.Error(t, errAPIKeyMissing, err)
}
