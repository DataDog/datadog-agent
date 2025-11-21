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
	ScanEndTs  *time.Time `json:"scan_end_ts,omitempty"`
}

type scanStatus string

const (
	pendingStatus scanStatus = "pending"
	successStatus scanStatus = "success"
	failedStatus  scanStatus = "failed"
)

func (ds *deviceScan) isCacheable() bool {
	return ds.ScanStatus == successStatus || ds.ScanStatus == failedStatus
}
