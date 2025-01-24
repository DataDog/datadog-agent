// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextension defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type testComponent struct {
	*pkgagent.Agent
}

func (c testComponent) SetOTelAttributeTranslator(attrstrans *attributes.Translator) {
	c.Agent.OTLPReceiver.SetOTelAttributeTranslator(attrstrans)
}

func (c testComponent) ReceiveOTLPSpans(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header) source.Source {
	return c.Agent.OTLPReceiver.ReceiveResourceSpans(ctx, rspans, httpHeader)
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
	ext, err := NewExtension(&Config{}, component.BuildInfo{}, testComponent{}, &logger{})
	assert.NoError(t, err)

	_, ok := ext.(*ddExtension)
	assert.True(t, ok)
}

func TestAgentExtension(t *testing.T) {
	// fake intake
	got := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		assert.NoError(t, err)

		fmt.Println(string(b))
		fmt.Println(reflect.TypeOf(b))

		assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", req.Header.Get("DD-Api-Key"))
		got <- req.Header.Get("User-Agent")
		rw.WriteHeader(http.StatusAccepted)
	}))
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
	ext, err := NewExtension(&Config{}, component.BuildInfo{}, testComponent{traceagent}, &logger{})
	assert.NoError(t, err)

	ext, ok := ext.(*ddExtension)
	assert.True(t, ok)

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
