// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// The convert-traces feature is enabled by default, so the agent serializes
// tracer payloads in the v1 string-indexed idx format (AgentPayload.IdxTracerPayloads),
// even when traces are submitted in a legacy (v0.4/v0.7/OTLP) format. Span, chunk
// and payload metadata is carried as references into a per-payload string table,
// and tags/meta/metrics live in a single attributes map keyed by string reference.
// The helpers below resolve those references so assertions can read the indexed
// payloads directly.

// idxStr resolves a string-table reference to its value. Reference 0 is the
// empty-string sentinel.
func idxStr(strings []string, ref uint32) string {
	if ref == 0 || int(ref) >= len(strings) {
		return ""
	}
	return strings[ref]
}

// idxStrAttr returns the string value of the attribute named key, and whether a
// string-valued attribute with that key was present.
func idxStrAttr(strings []string, attrs map[uint32]*idx.AnyValue, key string) (string, bool) {
	for k, v := range attrs {
		if idxStr(strings, k) != key {
			continue
		}
		if sv, ok := v.Value.(*idx.AnyValue_StringValueRef); ok {
			return idxStr(strings, sv.StringValueRef), true
		}
		return "", false
	}
	return "", false
}

// idxNumAttr returns the numeric value of the attribute named key, and whether a
// numeric-valued attribute with that key was present.
func idxNumAttr(strings []string, attrs map[uint32]*idx.AnyValue, key string) (float64, bool) {
	for k, v := range attrs {
		if idxStr(strings, k) != key {
			continue
		}
		switch val := v.Value.(type) {
		case *idx.AnyValue_DoubleValue:
			return val.DoubleValue, true
		case *idx.AnyValue_IntValue:
			return float64(val.IntValue), true
		}
		return 0, false
	}
	return 0, false
}

// idxHasAttr reports whether an attribute named key is present, regardless of type.
func idxHasAttr(strings []string, attrs map[uint32]*idx.AnyValue, key string) bool {
	for k := range attrs {
		if idxStr(strings, k) == key {
			return true
		}
	}
	return false
}
