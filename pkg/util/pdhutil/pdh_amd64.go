// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package pdhutil

// PDH_FMT_COUNTERVALUE_DOUBLE is a union specialization for double values
//
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/ns-pdh-pdh_fmt_countervalue
//
//revive:disable-next-line:var-naming Name is intended to match the Windows type name
type PDH_FMT_COUNTERVALUE_DOUBLE struct {
	CStatus     uint32
	DoubleValue float64
}

// PDH_FMT_COUNTERVALUE_LARGE is a union specialization for 64 bit integer values
//
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/ns-pdh-pdh_fmt_countervalue
//
//revive:disable-next-line:var-naming Name is intended to match the Windows type name
type PDH_FMT_COUNTERVALUE_LARGE struct {
	CStatus    uint32
	LargeValue int64
}

// PDH_FMT_COUNTERVALUE_LONG is a union specialization for long values
//
// https://learn.microsoft.com/en-us/windows/win32/api/pdh/ns-pdh-pdh_fmt_countervalue
//
//revive:disable-next-line:var-naming Name is intended to match the Windows type name
type PDH_FMT_COUNTERVALUE_LONG struct {
	CStatus   uint32
	_         uint32 // intpadding
	LongValue int32
}
