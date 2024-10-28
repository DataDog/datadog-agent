// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"testing"

	"github.com/mdlayher/netlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestAttributeScanner(t *testing.T) {
	attrs := []netlink.Attribute{
		{
			Type: 1,
			Data: []byte{0x1, 0x2, 0x3},
		},
		{
			Type: 2,
			Data: []byte{0x4, 0x5, 0x6},
		},
		{
			Type: 3,
			Data: []byte{0x7, 0x8, 0x9},
		},
	}

	data, err := netlink.MarshalAttributes(attrs)
	require.NoError(t, err)
	payload := createPayload(data)

	captured := make(map[uint16][]byte)
	scanner := NewAttributeScanner()
	scanner.ResetTo(payload)

	for scanner.Next() {
		captured[scanner.Type()] = scanner.Bytes()
	}

	expected := map[uint16][]byte{
		1: attrs[0].Data,
		2: attrs[1].Data,
		3: attrs[2].Data,
	}
	assert.Nil(t, scanner.Err())
	assert.Equal(t, expected, captured)
}

func TestNestedFields(t *testing.T) {
	nestedAttrs := []netlink.Attribute{
		{
			Type: 2,
			Data: []byte{0x1, 0x2, 0x3},
		},
		{
			Type: 3,
			Data: []byte{0x4, 0x5, 0x6},
		},
	}

	marshaledNested, err := netlink.MarshalAttributes(nestedAttrs)
	require.NoError(t, err)

	topLevelAttrs := []netlink.Attribute{
		{
			Type: 1,
			Data: marshaledNested,
		},
		{
			Type: 4,
			Data: []byte{0x7, 0x8, 0x9},
		},
	}

	data, err := netlink.MarshalAttributes(topLevelAttrs)
	require.NoError(t, err)
	payload := createPayload(data)

	scanner := NewAttributeScanner()
	scanner.ResetTo(payload)

	// First attribute is type 1 which encloses a nested attribute
	scanner.Next()
	assert.Equal(t, uint16(1), scanner.Type())
	assert.Equal(t, topLevelAttrs[0].Data, scanner.Bytes())

	// Inside this function we "see" only the nested attributes
	scanner.Nested(func() error {
		scanner.Next()
		assert.Equal(t, uint16(2), scanner.Type())
		assert.Equal(t, nestedAttrs[0].Data, scanner.Bytes())

		scanner.Next()
		assert.Equal(t, uint16(3), scanner.Type())
		assert.Equal(t, nestedAttrs[1].Data, scanner.Bytes())

		// Once we have traversed all fields, Next() should return false
		assert.False(t, scanner.Next())

		return nil
	})

	// And we're back to top-level again
	scanner.Next()
	assert.Equal(t, uint16(4), scanner.Type())
	assert.Equal(t, topLevelAttrs[1].Data, scanner.Bytes())

	// Once we have traversed all fields, Next() should return false
	assert.False(t, scanner.Next())
}

// The messages we're interested in have this preamble
var preamble = []byte{unix.AF_INET, unix.NFNETLINK_V0, 0x0, 0x0}

func createPayload(data []byte) []byte {
	var payload []byte
	payload = append(payload, preamble...)
	payload = append(payload, data...)
	return payload
}
