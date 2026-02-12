// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector provides telemetry data collection and aggregation.
package collector

import (
	"fmt"
	"strings"
)

type TelemetrySignal struct {
	Type   string
	Values map[string]float64
}

func NewTelemetrySignal[T any](typeName string, values []T, getKey func(T) string, getValue func(T) float64) TelemetrySignal {
	sum := float64(0)
	for _, signal := range values {
		sum += getValue(signal)
	}

	valuesMap := make(map[string]float64)
	for _, signal := range values {
		valuesMap[getKey(signal)] = getValue(signal) / sum
	}
	return TelemetrySignal{
		Type:   typeName,
		Values: valuesMap,
	}
}

func (t TelemetrySignal) String() string {
	var sb strings.Builder
	for k, v := range t.Values {
		fmt.Fprintf(&sb, "%v:%v\n", k, v)
	}
	return fmt.Sprintf("%v: %v\n", t.Type, sb.String())
}

func (t TelemetrySignal) IsSimilarTo(t2 TelemetrySignal) float64 {
	score := float64(0)
	for k, v := range t.Values {

		if v2, found := t2.Values[k]; found {
			score += min(v, v2)
		}
	}
	return score
}
