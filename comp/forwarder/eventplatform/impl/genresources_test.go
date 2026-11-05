// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eventplatformimpl

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	gpuresources "github.com/DataDog/datadog-agent/pkg/gpu/resources"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/genresources"
	"google.golang.org/protobuf/proto"
)

type resourceSubmitter struct {
	forwarder eventplatform.Forwarder
	err       error
}

func (suite *EventPlatformForwarderTestSuite) TestGenResourcesQueueFull() {
	pipeline := &passthroughPipeline{in: make(chan *message.Message, 1), eventPlatformReceiver: suite.receiver}
	f := &defaultEventPlatformForwarder{pipelines: map[string]*passthroughPipeline{eventplatform.EventTypeGenResources: pipeline}}
	msg := message.NewMessage([]byte("resource"), nil, "", 0)
	suite.Require().NoError(f.SendEventPlatformEvent(msg, eventplatform.EventTypeGenResources))
	suite.ErrorContains(f.SendEventPlatformEvent(msg, eventplatform.EventTypeGenResources), "channel is full")
	suite.Len(f.Purge()[eventplatform.EventTypeGenResources], 1)
}

func (s *resourceSubmitter) EventPlatformEvent(payload []byte, track string) {
	s.err = s.forwarder.SendEventPlatformEvent(message.NewMessage(payload, nil, "", time.Now().UnixNano()), track)
}

func (suite *EventPlatformForwarderTestSuite) TestGPUResourcesProtobufHTTP() {
	type request struct {
		body    []byte
		path    string
		headers http.Header
		err     error
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		select {
		case requests <- request{body: body, path: r.URL.Path, headers: r.Header.Clone(), err: err}:
		default:
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	suite.T().Cleanup(server.Close)
	suite.config.SetInTest("api_key", "test-gpu-api-key")
	suite.config.SetInTest("genresources.logs_dd_url", strings.TrimPrefix(server.URL, "http://"))
	suite.config.SetInTest("genresources.logs_no_ssl", true)
	suite.config.SetInTest("genresources.use_compression", false)
	ctx := client.NewDestinationsContext()
	pipeline, err := newHTTPPassthroughPipeline(suite.config, suite.receiver, suite.compression, getGenResourcesPipelines()[0], ctx, 0, "canonical-host", secretsnoopimpl.NewComponent().Comp)
	suite.Require().NoError(err)
	f := &defaultEventPlatformForwarder{pipelines: map[string]*passthroughPipeline{eventplatform.EventTypeGenResources: pipeline}, destinationsCtx: ctx}
	f.Start()
	suite.T().Cleanup(func() { ctx.Stop(); f.Stop() })
	sender := &resourceSubmitter{forwarder: f}
	now := time.Unix(1700000000, 0).UTC()
	publisher, err := gpuresources.NewPublisher(gpuresources.Options{TTL: 15 * time.Minute, MaxPerRun: 64})
	suite.Require().NoError(err)
	suite.Require().NoError(publisher.Publish(now, "canonical-host", []gpuresources.Device{{UUID: "GPU-12345678-1234-5678-1234-111111111111", Model: "tesla_t4"}}, sender))
	suite.Require().NoError(sender.err)
	select {
	case received := <-requests:
		suite.Require().NoError(received.err)
		suite.Equal("/api/v2/genresources", received.path)
		suite.Equal(logshttp.ProtobufContentType, received.headers.Get("Content-Type"))
		suite.Equal("test-gpu-api-key", received.headers.Get("DD-API-KEY"))
		var event genresources.GenericResourceEvent
		suite.Require().NoError(proto.Unmarshal(received.body, &event))
		suite.Equal("agent", event.Source)
		suite.Equal("gpu_device", event.Resource.Type)
		suite.Equal(now, event.Resource.SeenAt.AsTime())
		suite.Equal(now.Add(15*time.Minute), event.Resource.ExpireAt.AsTime())
		var fields map[string]any
		suite.Require().NoError(json.Unmarshal(event.Resource.FieldsJson, &fields))
		suite.Equal("canonical-host", fields["hostname"])
		suite.Equal(event.Resource.Key, fields["gpu_uuid"])
	case <-time.After(5 * time.Second):
		suite.FailNow("timed out waiting for GPU resource HTTP request")
	}
}
