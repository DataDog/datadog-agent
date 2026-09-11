// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package golabelsctx holds the resolver for Go pprof-label syscall context
package golabelsctx

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	lib "github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

// Sentinels Resolve wraps its error with, for classifySpanCtxError
// (resolvers/process/span_ctx_stats.go) to classify per-event lookup
// failures without either package depending on the other's types.
var (
	// ErrMapLookup means the ring slot lookup itself failed.
	ErrMapLookup = errors.New("go labels context map lookup failed")
	// ErrStaleID means the ring slot's id no longer matches the one the event
	// carried: the slot was reused before this event's labels were resolved.
	ErrStaleID = errors.New("stale go labels context id")
)

// see kernel definitions (constants/custom.h)
const (
	maxEntries = 4096
	keySize    = 32
	valSize    = 64
	maxPairs   = 10

	// keys published by dd-trace-go as goroutine pprof labels
	spanIDKey = "span id"
	// legacyTraceIDKey is the low-64-bits-only decimal trace id label. Older
	// dd-trace-go versions publish it.
	legacyTraceIDKey = "local root span id"
	// fullTraceIDKey is the full 128-bit trace id, encoded as a 32-character
	// lowercase hex string.
	fullTraceIDKey = "trace id"
)

// kernelLabelPair mirrors struct go_label_pair_t
type kernelLabelPair struct {
	KeyLen uint16
	ValLen uint16
	Key    [keySize]byte
	Val    [valSize]byte
}

// kernelLabelsEntry mirrors struct go_labels_ctx_entry_t
type kernelLabelsEntry struct {
	ID    uint32
	Pairs [maxPairs]kernelLabelPair
}

// Resolver resolves Go pprof-label snapshots into span/trace ids
type Resolver struct {
	ctxMap *lib.Map
}

// Resolve looks the labels snapshot up by id and extracts the span/trace ids.
// It returns the zero values when the id is stale (ring reuse) or the labels
// carry no span context.
func (r *Resolver) Resolve(ctxID uint32) (spanID uint64, traceID utils.TraceID, err error) {
	key := ctxID % maxEntries

	var entry kernelLabelsEntry
	if err = r.ctxMap.Lookup(key, &entry); err != nil {
		return 0, utils.TraceID{}, fmt.Errorf("%w: unable to resolve the go labels context for `%d`: %w", ErrMapLookup, ctxID, err)
	}

	if ctxID != entry.ID {
		return 0, utils.TraceID{}, fmt.Errorf("%w: `%d` vs `%d`", ErrStaleID, ctxID, entry.ID)
	}

	var legacyTraceIDVal, fullTraceIDVal string

	for i := range entry.Pairs {
		pair := &entry.Pairs[i]
		if pair.KeyLen == 0 {
			continue
		}

		switch labelString(pair.Key[:], pair.KeyLen) {
		case spanIDKey:
			spanID = parseDecimal(labelString(pair.Val[:], pair.ValLen))
		case legacyTraceIDKey:
			legacyTraceIDVal = labelString(pair.Val[:], pair.ValLen)
		case fullTraceIDKey:
			fullTraceIDVal = labelString(pair.Val[:], pair.ValLen)
		}
	}

	// The full trace id wins
	if hi, lo, ok := parseHexTraceID(fullTraceIDVal); ok {
		traceID.Hi, traceID.Lo = hi, lo
	} else {
		traceID.Lo = parseDecimal(legacyTraceIDVal)
	}

	return spanID, traceID, nil
}

// labelString returns the label bytes as a string, clamped to the buffer size
// (the kernel stores the real, possibly-truncated length).
func labelString(buf []byte, length uint16) string {
	l := int(length)
	if l > len(buf) {
		l = len(buf)
	}
	return string(buf[:l])
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

// Start the go labels context resolver
func (r *Resolver) Start(manager *manager.Manager) error {
	ctxMap, err := managerhelper.Map(manager, "go_labels_ctx")
	if err != nil {
		return err
	}
	r.ctxMap = ctxMap

	return nil
}

// Close the resolver
func (r *Resolver) Close() error {
	return nil
}

// NewResolver returns a new go labels context resolver
func NewResolver() *Resolver {
	return &Resolver{}
}
