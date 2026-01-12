// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"
)

// Run locally with `go test -fuzz=FuzzParseServiceCheck -run=FuzzParseServiceCheck -tags=test`
func FuzzParseServiceCheck(f *testing.F) {

	f.Add([]byte("_sc|agent.up|0"))
	f.Add([]byte("_sc|agent.up|0|d:12345|h:some.host|#tag1,tag2:test"))
	f.Add([]byte("_sc|agent.up|0|d:12345|h:some.host|#tag1,tag2:test|m:some message"))
	f.Add([]byte("_sc|agent.up|0|#tag1,tag2:test,tag3"))
	f.Add([]byte("_sc|agent.up|0|d:21|h:localhost|h:localhost2|d:22"))

	f.Fuzz(func(t *testing.T, rawServiceCheck []byte) {
		// This needs to be done in the fuzz test because there's a log emitted
		// and the fuzzer cannot call `f.Log()` inside here, it must be `t.Log()`, which we don't have access to if it's initialized once.
		deps := newServerDeps(t)
		stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
		parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)
		_, _ = parser.parseServiceCheck(rawServiceCheck)
	})
}
