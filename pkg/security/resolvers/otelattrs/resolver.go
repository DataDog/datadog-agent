// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package otelattrs holds the resolver for the extra attributes of OTel thread local context records
package otelattrs

import (
	"errors"
	"fmt"

	lib "github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/otelthreadctx"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
)

// Sentinels Resolve wraps its error with, for classifySpanCtxError
// (resolvers/process/span_ctx_stats.go) to classify per-event lookup
// failures without either package depending on the other's types.
var (
	// ErrMapLookup means the ring slot lookup itself failed.
	ErrMapLookup = errors.New("otel span attrs map lookup failed")
	// ErrStaleID means the ring slot's id no longer matches the one the event
	// carried: the slot was reused before this event's attributes were resolved.
	ErrStaleID = errors.New("stale otel span attrs id")
	// ErrMalformed means the process published an implausible attributes size.
	ErrMalformed = errors.New("malformed otel span attrs")
)

// see kernel definitions (constants/custom.h, structs/process.h)
const (
	maxEntries = 4096
	maxSize    = 256
)

// kernelAttrsEntry mirrors struct otel_span_attrs_t. It must stay free of implicit
// padding: cilium/ebpf only decodes a map value straight into it while binary.Size
// and unsafe.Sizeof agree, hence the explicit Padding field.
type kernelAttrsEntry struct {
	ID      uint32
	Size    uint16
	Padding uint16
	Data    [maxSize]byte
}

// Resolver resolves the extra attributes of an OTel thread local context record
// from the otel_span_attrs ring the kernel staged them in.
type Resolver struct {
	attrsMap *lib.Map
}

// Resolve parses the attributes snapshot the kernel published at attrsID's ring
// slot. The kernel stamps each snapshot with its id, so a mismatch means the slot
// was recycled for a later snapshot before this event got drained, and the
// attributes are gone.
func (r *Resolver) Resolve(attrsID uint64) ([]otelthreadctx.Attribute, error) {
	key := uint32(attrsID % maxEntries)

	var entry kernelAttrsEntry
	if err := r.attrsMap.Lookup(key, &entry); err != nil {
		return nil, fmt.Errorf("%w: unable to resolve the otel span attributes for `%d`: %w", ErrMapLookup, attrsID, err)
	}

	if uint64(entry.ID) != attrsID {
		return nil, fmt.Errorf("%w: `%d` vs `%d`", ErrStaleID, attrsID, entry.ID)
	}

	size := int(entry.Size)
	if size == 0 || size > len(entry.Data) {
		return nil, fmt.Errorf("%w: invalid otel span attributes size for `%d`: %d", ErrMalformed, attrsID, size)
	}

	return otelthreadctx.ParseAttributes(entry.Data[:size]), nil
}

// Start the otel attributes resolver
func (r *Resolver) Start(manager *manager.Manager) error {
	attrsMap, err := managerhelper.Map(manager, "otel_span_attrs")
	if err != nil {
		return err
	}
	r.attrsMap = attrsMap

	return nil
}

// Close the resolver
func (r *Resolver) Close() error {
	return nil
}

// NewResolver returns a new otel attributes resolver
func NewResolver() *Resolver {
	return &Resolver{}
}
