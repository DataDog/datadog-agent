// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib

package output

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	compressor "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
)

func TestRecorderDrainsOwnedCycles(t *testing.T) {
	recorder := NewRecorder()
	send := func(value string) error {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, "https://recording.invalid/api/v1/series?credential=excluded", bytes.NewBufferString(value))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "excluded")
		response, err := recorder.RoundTrip(req)
		if response != nil {
			response.Body.Close()
		}
		return err
	}
	for range 257 {
		if err := send("owned"); err != nil {
			t.Fatal(err)
		}
	}
	owned := recorder.Drain()
	if len(owned) != 257 || recorder.bytes != 0 || len(recorder.requests) != 0 {
		t.Fatal("drain retained prior cycle data")
	}
	if err := send("next-cycle"); err != nil {
		t.Fatal(err)
	}
	next := recorder.Drain()
	if len(next) != 1 || string(next[0].Body) != "next-cycle" || string(owned[0].Body) != "owned" || owned[0].Path != "/api/v1/series" || owned[0].Headers.Get("Authorization") != "" {
		t.Fatal("cycle ownership or privacy changed")
	}
}

func TestRecorderBoundsRetainedBodyAndHeaderMemory(t *testing.T) {
	recorder := NewRecorder()
	body := strings.Repeat("b", 1<<20)
	host := strings.Repeat("h", 1<<20)
	accepted := 0
	for range 129 {
		req, err := http.NewRequest(http.MethodPost, "https://recording.invalid/api/v1/collector", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(headers.HostHeader, host)
		response, err := recorder.RoundTrip(req)
		if err != nil {
			break
		}
		response.Body.Close()
		accepted++
	}
	// Both the complete body and selected headers consume the budget. Counting
	// only body bytes would retain over 64 of these two-MiB requests.
	if accepted < 2 || accepted > 64 || recorder.bytes > recorderMaxBytes {
		t.Fatalf("recorded bodies and headers escaped the memory bound: %d references", accepted)
	}
	owned := recorder.Drain()
	if len(owned) != accepted || len(recorder.requests) != 0 || recorder.bytes != 0 || string(owned[0].Body) != body || owned[0].Headers.Get(headers.HostHeader) != host {
		t.Fatal("bounded recording did not preserve and release accepted references")
	}
}

func TestRecorderAcceptsCompleteLargeProcessGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	recorder := NewRecorder()
	pipeline, err := New(ctx, nil, "recording-only-no-credential", recorder)
	if err != nil {
		t.Fatal(err)
	}
	defer pipeline.Close()
	const count = 257
	bodies := make([]model.MessageBody, count)
	for i := range bodies {
		bodies[i] = &model.CollectorProc{HostName: "capture-host", GroupId: 31, GroupSize: count, Processes: []*model.Process{{Pid: int32(i + 1)}}}
	}
	if err := pipeline.Group(ctx, time.Unix(20, 123456789), checks.ProcessCheckName, "capture-host", bodies); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	refs := recorder.Drain()
	if len(refs) != count {
		t.Fatalf("complete group lost wire chunks: %d", len(refs))
	}
	seen := map[uint64]bool{}
	var prefix uint64
	for _, ref := range refs {
		requestID, err := strconv.ParseUint(ref.Headers.Get(headers.RequestIDHeader), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		index := requestID & ((1 << 14) - 1)
		if index >= count || seen[index] || (len(seen) != 0 && requestID>>14 != prefix) {
			t.Fatal("large group lost unique ordered request indices")
		}
		seen[index], prefix = true, requestID>>14
		message, err := model.DecodeMessage(ref.Body)
		if err != nil {
			t.Fatal(err)
		}
		body, ok := message.Body.(*model.CollectorProc)
		if !ok || ref.Path != "/api/v1/collector" || body.GroupId != 31 || body.GroupSize != count || len(body.Processes) != 1 || body.Processes[0].Pid != int32(index+1) {
			t.Fatal("large group wire chunk changed its semantic order or completeness")
		}
	}
}

func TestNativeMetricReachesRealForwarderWithoutRecordingCredentials(t *testing.T) {
	cfg := configmock.New(t)
	logger := logmock.New(t)
	cfg.SetInTest("site", "datad0g.com")
	cfg.SetInTest("api_key", "UNIQUE-KEY-MUST-NOT-BE-RECORDED")
	cfg.SetInTest("use_v2_api.series", false)
	options, err := forwarder.NewOptions(cfg, logger, map[string][]utils.APIKeys{"https://app.datad0g.com": {utils.NewAPIKeys("api_key", cfg.GetString("api_key"))}})
	if err != nil {
		t.Fatal(err)
	}
	options.DisableAPIKeyChecking = true
	recorder := NewRecorder()
	options.SetTransport(recorder)
	f := forwarder.NewDefaultForwarder(cfg, logger, options)
	if err := f.Start(); err != nil {
		t.Fatal(err)
	}
	defer f.Stop()
	normalizer := capture.NewNormalizer()
	source, err := normalizer.Series(capture.NewSeriesSource([]*metrics.Serie{{Name: "system.cpu.user", Host: "UNIQUE-RAW-HOST", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1, Value: 5}}}}))
	if err != nil {
		t.Fatal(err)
	}
	s := serializer.NewSerializer(f, nil, compressor.New(), cfg, logger, "capture-host")
	s.RequireCompleteDelivery = true
	if err := s.SendIterableSeries(source); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	references, err := recorder.Wait(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range references {
		reader, err := zlib.NewReader(bytes.NewReader(ref.Body))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte("UNIQUE-RAW-HOST")) || bytes.Contains(body, []byte(cfg.GetString("api_key"))) {
			t.Fatal("native metric changed or a credential entered the payload")
		}
		for _, values := range ref.Headers {
			for _, value := range values {
				if value == cfg.GetString("api_key") {
					t.Fatal("recording retained API key")
				}
			}
		}
	}
}
