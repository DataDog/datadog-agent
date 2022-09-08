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
	processNodes int
	fileNodes    int
	dnsNodes     int
	socketNodes  int
}

func (stats *ActivityDumpNodeStats) approximateSize() int {
	var total int
	total += stats.processNodes * int(unsafe.Sizeof(ProcessActivityNode{}))
	total += stats.fileNodes * int(unsafe.Sizeof(FileActivityNode{}))
	total += stats.dnsNodes * int(unsafe.Sizeof(DNSNode{}))
	total += stats.socketNodes * int(unsafe.Sizeof(SocketNode{}))
	return total
}
