// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import "unsafe"

// ActivityDumpNodeStats represents the node counts in an activity dump
type ActivityDumpNodeStats struct {
	processNodes uint64
	fileNodes    uint64
	dnsNodes     uint64
	socketNodes  uint64
}

func (stats *ActivityDumpNodeStats) approximateSize() uint64 {
	var total uint64
	total += stats.processNodes * uint64(unsafe.Sizeof(ProcessActivityNode{}))
	total += stats.fileNodes * uint64(unsafe.Sizeof(FileActivityNode{}))
	total += stats.dnsNodes * uint64(unsafe.Sizeof(DNSNode{}))
	total += stats.socketNodes * uint64(unsafe.Sizeof(SocketNode{}))
	return total
}
