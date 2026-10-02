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
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	compressor "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
)

func TestSanitizedMetricReachesRealForwarderRecordingTransport(t *testing.T) {
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
	sanitizer, err := capture.NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	s := serializer.NewSerializer(f, nil, compressor.New(), cfg, logger, "capture-host", sanitizer)
	if err := s.SendIterableSeries(capture.NewSeriesSource([]*metrics.Serie{{Name: "system.cpu.user", Host: "UNIQUE-RAW-HOST", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1, Value: 5}}}})); err != nil {
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
		if bytes.Contains(body, []byte("UNIQUE-RAW-HOST")) || !bytes.Contains(body, []byte("capture-host")) {
			t.Fatal("serializer did not receive sanitized host identity")
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
