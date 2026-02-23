// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// formatType recursively formats a value based on its type, writing to buf.
// Uses encodingContext for cycle detection and data item lookups.
func formatType(
	c *encodingContext,
	buf *bytes.Buffer,
	t ir.Type,
	data []byte,
	limits *formatLimits,
) error {
	// Get decoderType from encodingContext.
	decoderType, ok := c.getType(t.GetID())
	if !ok {
		// Fallback for types not in decoder.
		writeBoundedFallback(buf, limits, "type not found")
		return nil
	}
	return decoderType.formatValueFields(c, buf, data, limits)
}

// Helper functions for reading numeric types using NativeEndian

func readInt(data []byte, size uint32) int64 {
	if uint32(len(data)) < size {
		return 0
	}
	switch size {
	case 1:
		return int64(int8(data[0]))
	case 2:
		return int64(int16(binary.NativeEndian.Uint16(data)))
	case 4:
		return int64(int32(binary.NativeEndian.Uint32(data)))
	case 8:
		return int64(binary.NativeEndian.Uint64(data))
	default:
		return 0
	}
}

func readUint(data []byte, size uint32) uint64 {
	if uint32(len(data)) < size {
		return 0
	}
	switch size {
	case 1:
		return uint64(data[0])
	case 2:
		return uint64(binary.NativeEndian.Uint16(data))
	case 4:
		return uint64(binary.NativeEndian.Uint32(data))
	case 8:
		return binary.NativeEndian.Uint64(data)
	default:
		return 0
	}
}

func readFloat(data []byte, size uint32) float64 {
	if uint32(len(data)) < size {
		return 0
	}
	switch size {
	case 4:
		return float64(math.Float32frombits(binary.NativeEndian.Uint32(data)))
	case 8:
		return math.Float64frombits(binary.NativeEndian.Uint64(data))
	default:
		return 0
	}
}
