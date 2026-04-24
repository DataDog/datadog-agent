// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libpcap

import (
	"testing"
)

func TestCompileBPFFilterEmpty(t *testing.T) {
	insns, err := CompileBPFFilter(LinkTypeEthernet, 262144, "")
	if err != nil {
		t.Fatalf("CompileBPFFilter('') = %v", err)
	}
	if len(insns) != 1 {
		t.Fatalf("len = %d, want 1", len(insns))
	}
	// Empty filter should return snaplen
	if insns[0].K != 262144 {
		t.Errorf("k = %d, want 262144", insns[0].K)
	}
}

func TestCompileBPFFilterIP(t *testing.T) {
	insns, err := CompileBPFFilter(LinkTypeEthernet, 262144, "ip")
	if err != nil {
		t.Fatalf("CompileBPFFilter('ip') = %v", err)
	}
	if len(insns) < 3 {
		t.Fatalf("len = %d, want >= 3", len(insns))
	}
	t.Logf("'ip' compiled to %d instructions", len(insns))
}

func TestCompileBPFFilterTCP(t *testing.T) {
	insns, err := CompileBPFFilter(LinkTypeEthernet, 262144, "tcp")
	if err != nil {
		t.Fatalf("CompileBPFFilter('tcp') = %v", err)
	}
	if len(insns) < 5 {
		t.Fatalf("len = %d, want >= 5", len(insns))
	}
	t.Logf("'tcp' compiled to %d instructions", len(insns))
}

func TestCompileBPFFilterSyntaxError(t *testing.T) {
	_, err := CompileBPFFilter(LinkTypeEthernet, 262144, "((((")
	if err == nil {
		t.Error("expected error for syntax error")
	}
}

func TestDumpFilterIP(t *testing.T) {
	dump, err := DumpFilter(LinkTypeEthernet, 262144, "ip")
	if err != nil {
		t.Fatalf("DumpFilter('ip') = %v", err)
	}
	t.Logf("'ip' dump:\n%s", dump)
	if dump == "" {
		t.Error("empty dump")
	}
}

func TestDumpFilterTCPPort80(t *testing.T) {
	dump, err := DumpFilter(LinkTypeEthernet, 262144, "tcp")
	if err != nil {
		t.Fatalf("DumpFilter('tcp') = %v", err)
	}
	t.Logf("'tcp' dump:\n%s", dump)
}
