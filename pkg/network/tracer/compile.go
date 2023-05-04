// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
)

//go:generate go run ../../../pkg/ebpf/include_headers.go ../../../pkg/network/ebpf/c/runtime/conntrack.c ../../../pkg/ebpf/bytecode/build/runtime/conntrack.c ../../../pkg/ebpf/c ../../../pkg/ebpf/c/protocols ../../../pkg/network/ebpf/c/runtime ../../../pkg/network/ebpf/c
//go:generate go run ../../../pkg/ebpf/bytecode/runtime/integrity.go ../../../pkg/ebpf/bytecode/build/runtime/conntrack.c ../../../pkg/ebpf/bytecode/runtime/conntrack.go runtime

func getRuntimeCompiledConntracker(config *config.Config) (runtime.CompiledOutput, error) {
	return runtime.Conntrack.Compile(&config.Config, getCFlags(config), statsd.Client)
}

func getCFlags(config *config.Config) []string {
	cflags := []string{"-g"}

	if config.CollectTCPv6Conns {
		cflags = append(cflags, "-DFEATURE_TCPV6_ENABLED")
	}
	if config.CollectUDPv6Conns {
		cflags = append(cflags, "-DFEATURE_UDPV6_ENABLED")
	}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}
