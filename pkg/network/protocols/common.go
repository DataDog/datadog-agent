// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package protocols provides the implementation of the network tracer protocols
package protocols

import (
	"fmt"
	"math"

	"github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

// below is copied from pkg/trace/stats/statsraw.go

// NSTimestampToFloat converts a nanosec timestamp into a float nanosecond timestamp truncated to a fixed precision
func NSTimestampToFloat(ns uint64) float64 {
	b := math.Float64bits(float64(ns))
	// IEEE-754
	// the mask include 1 bit sign 11 bits exponent (0xfff)
	// then we filter the mantissa to 10bits (0xff8) (9 bits as it has implicit value of 1)
	// 10 bits precision (any value will be +/- 1/1024)
	// https://en.wikipedia.org/wiki/Double-precision_floating-point_format
	b &= 0xfffff80000000000
	return math.Float64frombits(b)
}

// GetMap retrieves an eBPF map by name from the provided manager
func GetMap(mgr *manager.Manager, name string) (*ebpf.Map, error) {
	m, _, err := mgr.GetMap(name)
	if err != nil {
		return nil, fmt.Errorf("error getting %q map: %s", name, err)
	}
	if m == nil {
		return nil, fmt.Errorf("%q map is nil", name)
	}
	return m, nil
}
