// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import "unsafe"

// ActivityDumpSizeStats represents the node counts in an activity dump
type ActivityDumpSizeStats struct {
	processNodes uint64
	fileNodes    uint64
	dnsNodes     uint64
	socketNodes  uint64
}

func (stats *ActivityDumpSizeStats) approximateSize() uint64 {
	var total uint64
	total += stats.processNodes * uint64(unsafe.Sizeof(ProcessActivityNode{}))
	total += stats.fileNodes * uint64(unsafe.Sizeof(FileActivityNode{}))
	total += stats.dnsNodes * uint64(unsafe.Sizeof(DNSNode{}))
	total += stats.socketNodes * uint64(unsafe.Sizeof(SocketNode{}))
	return total
}

// Caution: you must hold a lock on the AD before calling this
func (ad *ActivityDump) computeSizeStats() ActivityDumpSizeStats {
	stats := ActivityDumpSizeStats{}

	openList := make([]*ProcessActivityNode, len(ad.ProcessActivityTree))
	copy(openList, ad.ProcessActivityTree)

	for len(openList) != 0 {
		current := openList[len(openList)-1]
		openList = openList[:len(openList)-1]

		stats.processNodes++

		// files
		stats.fileNodes += countFileNodes(current.Files)

		// DNS
		for _, dnsNode := range current.DNSNames {
			stats.dnsNodes += uint64(len(dnsNode.Requests))
		}

		// sockets
		for _, socketNode := range current.Sockets {
			// each bind node + 1 for the global socket node
			stats.socketNodes += uint64(len(socketNode.Bind)) + 1
		}

		openList = append(openList, current.Children...)
	}

	return stats
}

func countFileNodes(files map[string]*FileActivityNode) uint64 {
	var counter uint64

	openList := []map[string]*FileActivityNode{files}

	for len(openList) != 0 {
		current := openList[len(openList)-1]
		openList = openList[:len(openList)-1]

		for _, f := range current {
			counter++
			openList = append(openList, f.Children)
		}
	}

	return counter
}
