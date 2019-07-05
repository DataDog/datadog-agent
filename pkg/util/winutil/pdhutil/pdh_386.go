// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build windows

package pdhutil

// Union specialization for double values
type PDH_FMT_COUNTERVALUE_DOUBLE struct {
	CStatus      uint32
	floatpadding uint32
	DoubleValue  float64
	padding1     uint32
	padding2     uint32
}

// Union specialization for 64 bit integer values
type PDH_FMT_COUNTERVALUE_LARGE struct {
	CStatus      uint32
	floatpadding uint32
	LargeValue   int64
	padding1     uint32
	padding2     uint32
}

// Union specialization for long values
type PDH_FMT_COUNTERVALUE_LONG struct {
	CStatus      uint32
	floatpadding uint32
	LongValue    int32
	padding1     int32
	padding2     int32
}
