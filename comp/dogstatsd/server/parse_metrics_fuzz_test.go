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
	deps := newServerDeps(f)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
	f.Add([]byte("custom_counter:1|c|#protocol:http,bench"))
	f.Add([]byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"))
	f.Add([]byte("metric:1234|g|#onetag|T1657100430"))
	f.Add([]byte("metric:1234|g|@0.21|T1657100440"))
	f.Add([]byte("example.metric:2.39283|d|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f"))
	f.Add([]byte("example.metric:2.39283|d|T1657100540|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f|f:wowthisisacoolfeature"))
	f.Fuzz(func(_ *testing.T, rawSample []byte) {
		_, _ = parser.parseMetricSample(rawSample)
	})
}
