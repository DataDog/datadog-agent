// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutil provides helpers for building fakeintake test payloads.
package testutil

import (
	"google.golang.org/protobuf/proto"

	apiv3 "github.com/DataDog/datadog-agent/test/fakeintake/aggregator/internal/apiv3"
)

// MinimalV3GaugePayload returns a marshalled /api/intake/metrics/v3/series payload
// containing a single gauge "test.gauge" with tag "env:test" at timestamp 1000.
// This payload can be POSTed to a fakeintake server with Content-Type "application/x-protobuf".
func MinimalV3GaugePayload() []byte {
	nameStr := append([]byte{10}, []byte("test.gauge")...)
	tagStr := append([]byte{8}, []byte("env:test")...)

	p := &apiv3.Payload{
		MetricData: &apiv3.MetricData{
			DictNameStr:        nameStr,
			DictTagStr:         tagStr,
			DictTagsets:        []int64{1, 1},
			Types:              []uint64{uint64(apiv3.MetricType_Gauge) | uint64(apiv3.ValueType_Float64)},
			NameRefs:           []int64{1},
			TagsetRefs:         []int64{1},
			ResourcesRefs:      []int64{0},
			SourceTypeNameRefs: []int64{0},
			OriginInfoRefs:     []int64{0},
			Intervals:          []uint64{0},
			NumPoints:          []uint64{1},
			Timestamps:         []int64{1000},
			ValsFloat64:        []float64{42.0},
		},
	}
	raw, err := proto.Marshal(p)
	if err != nil {
		panic("testutil.MinimalV3GaugePayload: proto.Marshal failed: " + err.Error())
	}
	return raw
}
