// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import "unsafe"

// ActivityDumpNodeStats represents the node counts in an activity dump
type ActivityDumpNodeStats struct {
	processNodes int64
	fileNodes    int64
	dnsNodes     int64
	socketNodes  int64
}

func (stats *ActivityDumpNodeStats) approximateSize() int64 {
	var total int64
	total += stats.processNodes * int64(unsafe.Sizeof(ProcessActivityNode{})) // 1024
	total += stats.fileNodes * int64(unsafe.Sizeof(FileActivityNode{}))       // 80
	total += stats.dnsNodes * int64(unsafe.Sizeof(DNSNode{}))                 // 24
	total += stats.socketNodes * int64(unsafe.Sizeof(SocketNode{}))           // 40
	return total
}
