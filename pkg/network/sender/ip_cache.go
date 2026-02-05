// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import "net/netip"

type ipCache map[netip.Addr]string

func (ipc ipCache) get(addr netip.Addr) string {
	if v, ok := ipc[addr]; ok {
		return v
	}

	v := addr.String()
	ipc[addr] = v
	return v
}
