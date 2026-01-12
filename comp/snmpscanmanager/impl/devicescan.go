// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"time"
)

type deviceScansByIP map[string]deviceScan

type deviceScan struct {
	DeviceIP   string     `json:"device_ip"`
	ScanStatus scanStatus `json:"scan_status"`
	ScanEndTs  time.Time  `json:"scan_end_ts"`
	Failures   int        `json:"failures"`
}

type scanStatus string

const (
	successScan scanStatus = "success"
	failedScan  scanStatus = "failed"
)

func (ds *deviceScan) isSuccess() bool {
	return ds.ScanStatus == successScan
}

func (ds *deviceScan) isFailed() bool {
	return ds.ScanStatus == failedScan
}

type ipSet map[string]struct{}

func (s ipSet) add(ip string) {
	s[ip] = struct{}{}
}

func (s ipSet) contains(ip string) bool {
	_, ok := s[ip]
	return ok
}
