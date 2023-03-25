// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows
// +build windows

package winutil

// From GetUnixTimestamp() datadog-windows-filter\ddfilter\http\http_callbacks.c
// difference between windows and unix epochs in 100ns intervals
// 11644473600s * 1000ms/s * 1000us/ms * 10 intervals/us
const EPOCH_DIFFERENCE_SECS uint64 = 116444736000000000

// FileTimeToUnixTime translates FileTime to a golang time. Same as in standard packages.
// // From GetUnixTimestamp() datadog-windows-filter\ddfilter\http\http_callbacks.c
// returns timestamp in ns since unix epoch
func FileTimeToUnixTimeNs(ft uint64) uint64 {
	return (ft - EPOCH_DIFFERENCE_SECS) * 100
}

// returns time in seconds since unix epoch
func FileTimeToUnixTimeS(ft uint64) uint64 {
	return (ft - EPOCH_DIFFERENCE_SECS) / 10000000
}
