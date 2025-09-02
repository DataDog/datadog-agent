// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"
)

// Run locally with `go test -fuzz=FuzzParseEvent -run=FuzzParseEvent -tags=test`
func FuzzParseEvent(f *testing.F) {
	deps := newServerDeps(f)
	stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
	parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)

	f.Add([]byte("_e{10,9}:test title|test text"))
	f.Add([]byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"))
	f.Add([]byte("_e{10,9}:test title|test text|s:this is the source"))
	f.Add([]byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
	f.Add([]byte("_e{10,0}:test title||t:warning"))
	f.Fuzz(func(_ *testing.T, rawEvent []byte) {
		_, err := parser.parseEvent(rawEvent)
		if err != nil {
			// we expect errors, we just don't want to panic(), or timeout
			return
		}
	})
}
