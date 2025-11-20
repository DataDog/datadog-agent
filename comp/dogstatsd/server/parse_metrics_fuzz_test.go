// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"
)

// Run locally with `go test -fuzz=FuzzParseMetricSample -run=FuzzParseMetricSample -tags=test`
func FuzzParseMetricSample(f *testing.F) {
	f.Add([]byte("custom_counter:1|c|#protocol:http,bench"))
	f.Add([]byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"))
	f.Add([]byte("metric:1234|g|#onetag|T1657100430"))
	f.Add([]byte("metric:1234|g|@0.21|T1657100440"))
	f.Add([]byte("example.metric:2.39283|d|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f"))
	f.Add([]byte("example.metric:2.39283|d|T1657100540|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f|f:wowthisisacoolfeature"))
	f.Fuzz(func(t *testing.T, rawSample []byte) {
		// This needs to be done in the fuzz test because there's a log emitted
		// and the fuzzer cannot call `f.Log()` inside here, it must be `t.Log()`, which we don't have access to if it's initialized once.
		deps := newServerDeps(t)
		stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
		parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
		_, _ = parser.parseMetricSample(rawSample)
	})
}
