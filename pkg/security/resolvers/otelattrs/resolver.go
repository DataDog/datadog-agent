// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package otelattrs holds the resolver for the extra attributes of OTel thread local context records
package otelattrs

import (
	"encoding/binary"
	"fmt"

	lib "github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
)

// Resolver resolves the extra attributes an OTel thread local context record
// carried, from the snapshot the kernel staged in the otel_span_attrs map.
type Resolver struct {
	attrsMap *lib.Map
}

// Attribute is one raw entry of an attrs_data section: the key index the
// producer assigned, and the value bytes as-is. Resolving the index to a name
// needs the process-side key map, so it is left to the caller.
type Attribute struct {
	KeyIndex uint8
	Value    string
}

// Resolve looks the attributes snapshot up by id and parses its entries. The
// snapshot is per event, so it is dropped from the map once read.
func (r *Resolver) Resolve(attrsID uint64) ([]Attribute, error) {
	key := make([]byte, 8)
	binary.NativeEndian.PutUint64(key, attrsID)

	// value layout: u16 size + data[OTEL_ATTRS_MAX_SIZE], see struct otel_span_attrs_t
	data, err := r.attrsMap.LookupBytes(key)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve the otel span attributes for `%d`: %w", attrsID, err)
	}
	if len(data) < 2 {
		return nil, fmt.Errorf("otel span attributes value for `%d` is too short: %d bytes", attrsID, len(data))
	}
	_ = r.attrsMap.Delete(key)

	size := int(binary.NativeEndian.Uint16(data[0:2]))
	if size == 0 || size+2 > len(data) {
		return nil, fmt.Errorf("invalid otel span attributes size for `%d`: %d", attrsID, size)
	}

	return parseAttributes(data[2 : 2+size]), nil
}

// parseAttributes walks the attrs_data encoding: repeated
// [key index (u8) + value length (u8) + value bytes]. A trailing entry the
// kernel had to truncate is dropped.
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
