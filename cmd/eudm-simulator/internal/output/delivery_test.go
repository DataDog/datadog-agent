// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib

package output

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMultiHostProcessAndConnectionsIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	recorder := NewRecorder()
	p, err := New(ctx, nil, "recording", recorder)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	when := time.Unix(1000, 0)
	for _, host := range []string{"opaque-device-1", "opaque-device-2"} {
		if err := p.Process(ctx, when, &model.CollectorProc{HostName: host, GroupSize: 1, Processes: []*model.Process{{Pid: 100}}}); err != nil {
			t.Fatal(err)
		}
		if err := p.Connections(ctx, when, &model.CollectorConnections{HostName: host, GroupSize: 1, Connections: []*model.Connection{{Pid: 100, Rtt: 10000}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	references, err := recorder.Wait(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	requests := map[string]string{}
	for _, ref := range references {
		decoded, err := model.DecodeMessage(ref.Body)
		if err != nil {
			t.Fatal(err)
		}
		var hostname string
		switch body := decoded.Body.(type) {
		case *model.CollectorProc:
			hostname = body.HostName
		case *model.CollectorConnections:
			hostname = body.HostName
		default:
			t.Fatalf("unexpected Agent message %T", body)
		}
		if ref.Headers.Get(headers.HostHeader) != hostname {
			t.Fatal("header identity differs from payload")
		}
		id := ref.Headers.Get(headers.RequestIDHeader)
		if id == "" {
			t.Fatal("missing Agent request ID")
		}
		requests[hostname] = id
		if ref.Headers.Get(headers.TimestampHeader) != "1000" {
			t.Fatal("Agent timestamp header was lost")
		}
	}
	if requests["opaque-device-1"] == requests["opaque-device-2"] {
		t.Fatal("request IDs share the submitter host instead of the device host")
	}
}

func TestPermanentDeliveryAndEncodingFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := New(ctx, nil, "recording", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("rejected")), Request: req}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Serializer.SendIterableSeries(capture.NewSeriesSource([]*metrics.Serie{{Name: "system.cpu.user", Host: "opaque-device", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1, Value: 5}}}})); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(ctx); err == nil {
		t.Fatal("permanent HTTP rejection succeeded")
	}
	q, err := New(ctx, nil, "recording", NewRecorder())
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	q.Config.Set("serializer_max_series_points_per_payload", 1, configmodel.SourceAgentRuntime)
	err = q.Serializer.SendIterableSeries(capture.NewSeriesSource([]*metrics.Serie{{Name: "system.cpu.user", Host: "opaque-device", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1, Value: 5}, {Ts: 2, Value: 5}}}}))
	if err == nil {
		t.Fatal("oversized metric was silently dropped")
	}
	if err := q.Event(ctx, eventplatform.EventTypeSoftwareInventory, []byte(strings.Repeat("x", 6<<20)), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := q.Wait(ctx); err == nil {
		t.Fatal("oversized software snapshot was silently dropped")
	}
}

func TestNormalForwarderRetriesTransientFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	recorder := NewRecorder()
	var attempts atomic.Int32
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("retry")), Request: req}, nil
		}
		return recorder.RoundTrip(req)
	})
	p, err := New(ctx, map[safety.Destination][]string{}, "recording", transport)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Process(ctx, time.Now(), &model.CollectorProc{HostName: "opaque-device", GroupSize: 1, Processes: []*model.Process{{Pid: 100}}}); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() < 2 {
		t.Fatalf("expected normal retry, got %d attempt(s)", attempts.Load())
	}
}

func TestEventDeliveryRequiresEveryDestination(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var primary, additional atomic.Int32
	p, err := New(ctx, map[safety.Destination][]string{safety.EventPlatform: {"https://software.datad0g.com", "https://additional.datad0g.com"}}, "recording", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if req.URL.Hostname() == "additional.datad0g.com" {
			additional.Add(1)
			status = http.StatusForbidden
		} else {
			primary.Add(1)
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Event(ctx, eventplatform.EventTypeSoftwareInventory, []byte(`{"hostname":"device"}`), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(ctx); err == nil {
		t.Fatal("one destination hid a rejection at another destination")
	}
	if additional.Load() != 1 {
		t.Fatalf("expected one permanent rejection, got %d", additional.Load())
	}
}

func TestDeliveryDeadlineExhaustion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	p, err := New(ctx, nil, "recording", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("retry")), Request: req}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Process(ctx, time.Now(), &model.CollectorProc{HostName: "device", GroupSize: 1}); err == nil {
		t.Fatal("exhausted delivery deadline succeeded")
	}
	if err := p.Wait(ctx); err == nil {
		t.Fatal("unfinished fleet delivery succeeded")
	}
}

func TestEventDeliveryRetriesBeforeCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var attempts atomic.Int32
	p, err := New(ctx, nil, "recording", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if attempts.Add(1) == 1 {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Event(ctx, eventplatform.EventTypeSoftwareInventory, []byte(`{"hostname":"device"}`), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() < 2 {
		t.Fatal("event payload did not retry")
	}
	expected, accepted := p.Events.Tracker.Counts()
	if expected != 1 || accepted != 1 {
		t.Fatal("event retry corrupted completion accounting")
	}
}

func TestStagingTransportRejectsRedirectEscape(t *testing.T) {
	var contacted atomic.Int32
	client := &http.Client{Transport: stagingTransport{base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		contacted.Add(1)
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"https://app.datadoghq.com/intake"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})}}
	response, err := client.Get("https://app.datad0g.com/intake")
	if response != nil {
		response.Body.Close()
	}
	if err == nil || contacted.Load() != 1 {
		t.Fatal("redirect escaped the staging transport")
	}
}

func TestEventRedirectIsNotAcceptedAsDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := New(ctx, nil, "recording", roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"https://app.datadoghq.com/intake"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Event(ctx, eventplatform.EventTypeSoftwareInventory, []byte(`{"hostname":"device"}`), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(ctx); err == nil {
		t.Fatal("redirect response was counted as accepted")
	}
}
