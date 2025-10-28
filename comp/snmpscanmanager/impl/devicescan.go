package snmpscanmanagerimpl

import (
	"time"
)

type deviceScansByIP map[string]deviceScan

type deviceScan struct {
	DeviceIP   string     `json:"device_ip"`
	ScanStatus scanStatus `json:"scan_status"`
	ScanEndTs  time.Time  `json:"scan_end_ts"`
}

type scanStatus string

const (
	pendingStatus scanStatus = "pending"
	failedStatus  scanStatus = "failed"
	successStatus scanStatus = "success"
)

func (ds *deviceScan) isCacheable() bool {
	return ds.ScanStatus == successStatus || ds.ScanStatus == failedStatus
}
