// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

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

func readDoubleFromBuffer[T number](reader io.Reader) (float64, error) {
	var value T
	err := binary.Read(reader, binary.LittleEndian, &value)
	return float64(value), err
}

func metricValueToDouble(val nvml.FieldValue) (float64, error) {
	reader := bytes.NewReader(val.Value[:])

	switch nvml.ValueType(val.ValueType) {
	case nvml.VALUE_TYPE_DOUBLE:
		return readDoubleFromBuffer[float64](reader)
	case nvml.VALUE_TYPE_UNSIGNED_INT:
		return readDoubleFromBuffer[uint32](reader)
	case nvml.VALUE_TYPE_UNSIGNED_LONG:
	case nvml.VALUE_TYPE_UNSIGNED_LONG_LONG:
		return readDoubleFromBuffer[uint64](reader)
	case nvml.VALUE_TYPE_SIGNED_LONG_LONG: // No typo, there's no SIGNED_LONG in the NVML API
		return readDoubleFromBuffer[int64](reader)
	case nvml.VALUE_TYPE_SIGNED_INT:
		return readDoubleFromBuffer[int32](reader)
	}

	return 0, fmt.Errorf("unsupported value type %d", val.ValueType)
}
