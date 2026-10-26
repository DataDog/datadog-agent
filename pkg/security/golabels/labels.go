// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package golabels

import (
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

// see kernel definitions (constants/custom.h)
const (
	// KeySize is GO_LABELS_CTX_KEY_SIZE: at most KeySize-1 bytes of a key are read.
	KeySize = 32
	// ValSize is GO_LABELS_CTX_VAL_SIZE: at most ValSize-1 bytes of a value are read.
	ValSize = 64
	// MaxPairs is GO_LABELS_CTX_MAX_PAIRS, the most labels read per snapshot.
	MaxPairs = 10
)

const (
	// keys published by dd-trace-go as goroutine pprof labels
	spanIDKey = "span id"
	// legacyTraceIDKey is the low-64-bits-only decimal trace id label. Older
	// dd-trace-go versions publish it.
	legacyTraceIDKey = "local root span id"
	// fullTraceIDKey is the full 128-bit trace id, encoded as a 32-character
	// lowercase hex string.
	fullTraceIDKey = "trace id"
)

// Label is a pprof label.
type Label struct {
	Key   string
	Value string
}

// SpanContext extracts the span and trace ids dd-trace-go publishes from the
// labels. They are zero when the labels carry no span context.
func SpanContext(labels []Label) (spanID uint64, traceID utils.TraceID) {
	var legacyTraceIDVal, fullTraceIDVal string

	for _, label := range labels {
		switch label.Key {
		case spanIDKey:
			spanID = parseDecimal(label.Value)
		case legacyTraceIDKey:
			legacyTraceIDVal = label.Value
		case fullTraceIDKey:
			fullTraceIDVal = label.Value
		}
	}

	// The full trace id wins
	if hi, lo, ok := parseHexTraceID(fullTraceIDVal); ok {
		traceID.Hi, traceID.Lo = hi, lo
	} else {
		traceID.Lo = parseDecimal(legacyTraceIDVal)
	}

	return spanID, traceID
}

// parseDecimal parses a decimal id string as written by dd-trace-go. Trailing
// whitespace/newlines are trimmed; unparseable values resolve to 0.
func parseDecimal(s string) uint64 {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseHexTraceID parses the "trace id" label's 32-char lowercase hex value
// (hi, lo).
func parseHexTraceID(s string) (hi, lo uint64, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}

	split := max(len(s)-16, 0)

	lo, err := strconv.ParseUint(s[split:], 16, 64)
	if err != nil {
		return 0, 0, false
	}

	if split == 0 {
		// Less than 16 characters means we only have the low half.
		return 0, lo, true
	}

	hi, err = strconv.ParseUint(s[:split], 16, 64)
	if err != nil {
		return 0, 0, false
	}

	return hi, lo, true
}
