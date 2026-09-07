// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package otelattrs holds the resolver for the extra attributes of OTel thread local context records
package otelattrs

import (
	"fmt"

	lib "github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
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

// Attribute is one raw entry of an attrs_data section: the key index the producer
// assigned, and the value bytes as-is. Mapping the index to a name needs the
// process-side key list, so it is left to the caller.
type Attribute struct {
	KeyIndex uint8
	Value    string
}

// Resolve parses the attributes snapshot the kernel published at attrsID's ring
// slot. The kernel stamps each snapshot with its id, so a mismatch means the slot
// was recycled for a later snapshot before this event got drained, and the
// attributes are gone.
func (r *Resolver) Resolve(attrsID uint64) ([]Attribute, error) {
	key := uint32(attrsID % maxEntries)

	var entry kernelAttrsEntry
	if err := r.attrsMap.Lookup(key, &entry); err != nil {
		return nil, fmt.Errorf("unable to resolve the otel span attributes for `%d`: %w", attrsID, err)
	}

	if uint64(entry.ID) != attrsID {
		return nil, fmt.Errorf("incorrect id `%d` vs `%d`", attrsID, entry.ID)
	}

	size := int(entry.Size)
	if size == 0 || size > len(entry.Data) {
		return nil, fmt.Errorf("invalid otel span attributes size for `%d`: %d", attrsID, size)
	}

	return parseAttributes(entry.Data[:size]), nil
}

// parseAttributes walks the attrs_data encoding: repeated [key index (u8), value
// length (u8), value bytes]. A trailing entry the kernel truncated is dropped.
func parseAttributes(attrsData []byte) []Attribute {
	var attrs []Attribute

	for off := 0; off+2 <= len(attrsData); {
		keyIndex := attrsData[off]
		valLen := int(attrsData[off+1])
		off += 2

		if off+valLen > len(attrsData) {
			break
		}
		attrs = append(attrs, Attribute{KeyIndex: keyIndex, Value: string(attrsData[off : off+valLen])})
		off += valLen
	}

	return attrs
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
