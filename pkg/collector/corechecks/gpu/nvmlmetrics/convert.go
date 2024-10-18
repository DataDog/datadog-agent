// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024_present Datadog, Inc.

//go:build linux

package nvmlmetrics

import (
	"encoding/binary"
	"io"

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
