// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package golabelsctx holds the resolver for Go pprof-label syscall context
package golabelsctx

import (
	"fmt"
	"strconv"
	"strings"

	lib "github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
)

// see kernel definitions (constants/custom.h)
const (
	maxEntries = 4096
	keySize    = 32
	valSize    = 64
	maxPairs   = 10

	// keys published by dd-trace-go as goroutine pprof labels
	spanIDKey  = "span id"
	traceIDKey = "local root span id"
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
	if r == nil || r.ctxMap == nil {
		return 0, utils.TraceID{}, fmt.Errorf("go labels context resolver is not initialized")
	}

	key := ctxID % maxEntries

	var entry kernelLabelsEntry
	if err = r.ctxMap.Lookup(key, &entry); err != nil {
		return 0, utils.TraceID{}, fmt.Errorf("unable to resolve the go labels context for `%d`: %w", ctxID, err)
	}

	if ctxID != entry.ID {
		return 0, utils.TraceID{}, fmt.Errorf("incorrect id `%d` vs `%d`", ctxID, entry.ID)
	}

	for i := 0; i < maxPairs; i++ {
		pair := &entry.Pairs[i]
		if pair.KeyLen == 0 {
			continue
		}

		switch labelString(pair.Key[:], pair.KeyLen) {
		case spanIDKey:
			spanID = parseDecimal(labelString(pair.Val[:], pair.ValLen))
		case traceIDKey:
			traceID.Lo = parseDecimal(labelString(pair.Val[:], pair.ValLen))
		}
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
