// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

func (c ConnType) String() string {
	if c == TCP {
		return "TCP"
	}
	return "UDP"
}

func (c ConnFamily) String() string {
	if c == IPv4 {
		return "v4"
	}
	return "v6"
}
