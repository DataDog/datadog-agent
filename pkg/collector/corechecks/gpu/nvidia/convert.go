// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"golang.org/x/exp/constraints"
)

func boolToFloat(val bool) float64 {
	if val {
		return 1
	}
	return 0
}

type number interface {
	constraints.Integer | constraints.Float
}

func readNumberFromBuffer[T number, V number](reader io.Reader) (V, error) {
	var value T
	err := binary.Read(reader, binary.LittleEndian, &value)
	return V(value), err
}

func fieldValueToNumber[V number](valueType nvml.ValueType, value [8]byte) (V, error) {
	reader := bytes.NewReader(value[:])

	switch valueType {
	case nvml.VALUE_TYPE_DOUBLE:
		return readNumberFromBuffer[float64, V](reader)
	case nvml.VALUE_TYPE_UNSIGNED_INT:
		return readNumberFromBuffer[uint32, V](reader)
	case nvml.VALUE_TYPE_UNSIGNED_LONG, nvml.VALUE_TYPE_UNSIGNED_LONG_LONG:
		return readNumberFromBuffer[uint64, V](reader)
	case nvml.VALUE_TYPE_SIGNED_LONG_LONG: // No typo, there's no SIGNED_LONG in the NVML API
		return readNumberFromBuffer[int64, V](reader)
	case nvml.VALUE_TYPE_SIGNED_INT:
		return readNumberFromBuffer[int32, V](reader)

	default:
		return 0, fmt.Errorf("unsupported value type %d", valueType)
	}
}
