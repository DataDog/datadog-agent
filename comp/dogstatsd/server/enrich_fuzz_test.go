// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// Run locally with `go test -fuzz=FuzzParseEventWithEnrich -run=FuzzParseEventWithEnrich -tags=test`
func FuzzParseEventWithEnrich(f *testing.F) {

	f.Add([]byte("_e{10,9}:test title|test text"), "origin", uint32(1), true, true)
	f.Add([]byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"), "origin", uint32(1), true, true)
	f.Add([]byte("_e{10,9}:test title|test text|s:this is the source"), "origin", uint32(1), true, true)
	f.Add([]byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"), "origin", uint32(1), true, true)
	f.Add([]byte("_e{10,0}:test title||t:warning"), "origin", uint32(1), true, true)
	f.Fuzz(func(t *testing.T, rawEvent []byte, origin string, processID uint32, serverlessMode bool, entityIDPrecedenceEnabled bool) {
		// This needs to be done in the fuzz test because there's a log emitted
		// and the fuzzer cannot call `f.Log()` inside here, it must be `t.Log()`, which we don't have access to if it's initialized once.
		deps := newServerDeps(t)
		stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
		parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
		parsed, err := parser.parseEvent(rawEvent)
		if err != nil {
			return
		}
		_ = enrichEvent(parsed, origin, processID, enrichConfig{
			entityIDPrecedenceEnabled: entityIDPrecedenceEnabled,
			serverlessMode:            serverlessMode,
		})
	})
}

// Run locally with `go test -fuzz=FuzzParseMetricWithEnrich -run=FuzzParseMetricWithEnrich -tags=test`
func FuzzParseMetricWithEnrich(f *testing.F) {

	f.Add([]byte("custom_counter:1|c|#protocol:http,bench"), "origin", uint32(1), true, true)
	f.Add([]byte("custom_counter:1|c|#protocol:http,bench"), "origin", uint32(1), true, true)
	f.Fuzz(func(t *testing.T, rawMetric []byte, origin string, processID uint32, serverlessMode bool, entityIDPrecedenceEnabled bool) {
		// This needs to be done in the fuzz test because there's a log emitted
		// and the fuzzer cannot call `f.Log()` inside here, it must be `t.Log()`, which we don't have access to if it's initialized once.
		deps := newServerDeps(t)
		stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
		parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
		filter := utilstrings.NewMatcher([]string{"custom.metric.a", "custom.metric.b"}, false)

		parsed, err := parser.parseMetricSample(rawMetric)
		if err != nil {
			return
		}
		dest := make([]metrics.MetricSample, 0, 1)
		_ = enrichMetricSample(dest, parsed, origin, processID, "", enrichConfig{
			entityIDPrecedenceEnabled: entityIDPrecedenceEnabled,
			serverlessMode:            serverlessMode,
		}, &filter)
	})
}

// Run locally with `go test -fuzz=FuzzParseServiceCheckWithEnrich -run=FuzzParseServiceCheckWithEnrich -tags=test`
func FuzzParseServiceCheckWithEnrich(f *testing.F) {

	f.Add([]byte("_sc|agent.up|0|#tag1,tag2:test,tag3"), "origin", uint32(1), true, true)
	f.Add([]byte("_sc|agent.up|0|d:21|h:localhost|h:localhost2|d:22"), "origin", uint32(1), true, true)
	f.Fuzz(func(t *testing.T, rawServiceCheck []byte, origin string, processID uint32, serverlessMode bool, entityIDPrecedenceEnabled bool) {
		// This needs to be done in the fuzz test because there's a log emitted
		// and the fuzzer cannot call `f.Log()` inside here, it must be `t.Log()`, which we don't have access to if it's initialized once.
		deps := newServerDeps(t)
		stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
		parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)

		parsed, err := parser.parseServiceCheck(rawServiceCheck)
		if err != nil {
			return
		}
		_ = enrichServiceCheck(parsed, origin, processID, enrichConfig{
			entityIDPrecedenceEnabled: entityIDPrecedenceEnabled,
			serverlessMode:            serverlessMode,
		})
	})
}
