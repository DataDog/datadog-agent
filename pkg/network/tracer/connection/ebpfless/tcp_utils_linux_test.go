// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import "strconv"

var connStatusLabels = []string{
	"Closed",
	"Attempted",
	"Established",
}

func labelForState(tcpState connStatus) string {
	idx := int(tcpState)
	if idx < len(connStatusLabels) {
		return connStatusLabels[idx]
	}
	return "BadState-" + strconv.Itoa(idx)
}
