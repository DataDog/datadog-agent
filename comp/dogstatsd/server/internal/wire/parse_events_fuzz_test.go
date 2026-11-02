// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"testing"
)

// Run locally with `go test -fuzz=FuzzParseEvent -run=FuzzParseEvent -tags=test`
func FuzzParseEvent(f *testing.F) {

	f.Add([]byte("_e{10,9}:test title|test text"))
	f.Add([]byte("_e{10,24}:test|title|test\\line1\\nline2\\nline3"))
	f.Add([]byte("_e{10,9}:test title|test text|s:this is the source"))
	f.Add([]byte("_e{10,9}:test title|test text|t:warning|d:12345|p:low|h:some.host|k:aggKey|s:source test|#tag1,tag2:test"))
	f.Add([]byte("_e{10,0}:test title||t:warning"))
	f.Fuzz(func(t *testing.T, rawEvent []byte) {
		// This needs to be done in the fuzz test because there's a log emitted
		// and the fuzzer cannot call `f.Log()` inside here, it must be `t.Log()`, which we don't have access to if it's initialized once.
		parser := testParser(t, testOptions())
		_, err := parser.ParseEvent(rawEvent)
		if err != nil {
			// we expect errors, we just don't want to panic(), or timeout
			return
		}
	})
}
