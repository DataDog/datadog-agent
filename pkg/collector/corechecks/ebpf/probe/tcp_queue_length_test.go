// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux_bpf

package probe

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestTCPQueueLengthTracer(t *testing.T) {
	kv, err := kernel.HostVersion()
	if err != nil {
		t.Fatal(err)
	}
	if kv < kernel.VersionCode(4, 9, 0) {
		t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
	}

	cfg := ebpf.NewConfig()

	tcpTracer, err := NewTCPQueueLengthTracer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer tcpTracer.Close()
}
