// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nameresolver

import "testing"

func TestLookupHostNumeric(t *testing.T) {
	r := New()
	addrs, err := r.LookupHost("192.168.1.1")
	if err != nil {
		t.Fatalf("LookupHost(192.168.1.1) error: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != 0xC0A80101 {
		t.Errorf("LookupHost(192.168.1.1) = %#x, want [0xC0A80101]", addrs)
	}
}

func TestLookupHostIPv4Zero(t *testing.T) {
	r := New()
	addrs, err := r.LookupHost("0.0.0.0")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != 0 {
		t.Errorf("got %#x, want [0x0]", addrs)
	}
}

func TestLookupHost6Numeric(t *testing.T) {
	r := New()
	addrs, err := r.LookupHost6("::1")
	if err != nil {
		t.Fatalf("LookupHost6(::1) error: %v", err)
	}
	if len(addrs) != 1 {
		t.Fatalf("LookupHost6(::1) returned %d addrs, want 1", len(addrs))
	}
	expected := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	if addrs[0] != expected {
		t.Errorf("LookupHost6(::1) = %v, want %v", addrs[0], expected)
	}
}

func TestLookupPortNumeric(t *testing.T) {
	r := New()
	port, err := r.LookupPort("80", 6)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if port != 80 {
		t.Errorf("LookupPort(80) = %d, want 80", port)
	}
}

func TestLookupPortNamed(t *testing.T) {
	r := New()
	port, err := r.LookupPort("http", 6)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if port != 80 {
		t.Errorf("LookupPort(http) = %d, want 80", port)
	}
}

func TestLookupProto(t *testing.T) {
	r := New()
	tests := map[string]int{
		"tcp":  6,
		"udp":  17,
		"icmp": 1,
		"sctp": 132,
		"gre":  47,
		"esp":  50,
		"ah":   51,
	}
	for name, want := range tests {
		got, err := r.LookupProto(name)
		if err != nil {
			t.Errorf("LookupProto(%s) error: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("LookupProto(%s) = %d, want %d", name, got, want)
		}
	}
}

func TestLookupEProto(t *testing.T) {
	r := New()
	tests := map[string]int{
		"ip":   0x0800,
		"arp":  0x0806,
		"ipv6": 0x86dd,
	}
	for name, want := range tests {
		got, err := r.LookupEProto(name)
		if err != nil {
			t.Errorf("LookupEProto(%s) error: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("LookupEProto(%s) = %#x, want %#x", name, got, want)
		}
	}
}

func TestLookupNet(t *testing.T) {
	r := New()
	addr, mask, err := r.LookupNet("192.168.0.0/24")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if addr != 0xC0A80000 {
		t.Errorf("addr = %#x, want 0xC0A80000", addr)
	}
	if mask != 0xFFFFFF00 {
		t.Errorf("mask = %#x, want 0xFFFFFF00", mask)
	}
}

func TestLookupPortRange(t *testing.T) {
	r := New()
	p1, p2, err := r.LookupPortRange("8000-9000", -1)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p1 != 8000 || p2 != 9000 {
		t.Errorf("LookupPortRange = (%d, %d), want (8000, 9000)", p1, p2)
	}
}

func TestLookupProtoUnknown(t *testing.T) {
	r := New()
	_, err := r.LookupProto("nonexistent")
	if err == nil {
		t.Error("expected error for unknown protocol")
	}
}
