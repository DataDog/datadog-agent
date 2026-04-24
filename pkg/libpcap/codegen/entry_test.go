// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"testing"
)

// mockResolver implements NameResolver for testing.
type mockResolver struct{}

func (m *mockResolver) LookupHost(name string) ([]uint32, error) {
	switch name {
	case "192.168.1.1":
		return []uint32{0xC0A80101}, nil
	case "localhost":
		return []uint32{0x7F000001}, nil
	}
	return nil, &lookupError{name}
}

func (m *mockResolver) LookupHost6(name string) ([][16]byte, error) {
	if name == "::1" {
		addr := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		return [][16]byte{addr}, nil
	}
	return nil, &lookupError{name}
}

func (m *mockResolver) LookupPort(name string, _ int) (int, error) {
	switch name {
	case "http", "80":
		return 80, nil
	case "dns", "53":
		return 53, nil
	}
	return 0, &lookupError{name}
}

func (m *mockResolver) LookupProto(name string) (int, error) {
	switch name {
	case "tcp":
		return 6, nil
	case "udp":
		return 17, nil
	}
	return 0, &lookupError{name}
}

func (m *mockResolver) LookupEProto(name string) (int, error) { return 0, &lookupError{name} }
func (m *mockResolver) LookupLLC(name string) (int, error)    { return 0, &lookupError{name} }
func (m *mockResolver) LookupNet(name string) (uint32, uint32, error) {
	return 0, 0, &lookupError{name}
}
func (m *mockResolver) LookupEther(name string) ([]byte, error) {
	return nil, &lookupError{name}
}
func (m *mockResolver) LookupPortRange(name string, _ int) (int, int, error) {
	return 0, 0, &lookupError{name}
}

type lookupError struct{ name string }

func (e *lookupError) Error() string { return "unknown: " + e.name }

func newEthernetCSWithResolver() *CompilerState {
	cs := NewCompilerState(DLTEN10MB, 262144, 0, &mockResolver{})
	if err := InitLinktype(cs); err != nil {
		panic(err)
	}
	return cs
}

func TestGenNcodePort(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QPort, Proto: QDefault, Dir: QDefault}
	b := GenNcode(cs, "", 80, q)
	if b == nil {
		t.Fatal("GenNcode(port 80) returned nil")
	}
	if cs.Err != nil {
		t.Fatalf("error: %v", cs.Err)
	}
}

func TestGenNcodeHost(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QHost, Proto: QIP, Dir: QDefault}
	b := GenNcode(cs, "192.168.1.1", 0, q)
	if b == nil {
		t.Fatal("GenNcode(host 192.168.1.1) returned nil")
	}
}

func TestGenNcodeNet(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QNet, Proto: QIP, Dir: QDefault}
	b := GenNcode(cs, "192.168.0.0", 0, q)
	if b == nil {
		t.Fatal("GenNcode(net) returned nil")
	}
}

func TestGenNcodeProto(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QProto, Proto: QDefault, Dir: QDefault}
	b := GenNcode(cs, "", 6, q) // proto 6 = TCP
	if b == nil {
		t.Fatal("GenNcode(proto 6) returned nil")
	}
}

func TestGenScodeHost(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QHost, Proto: QDefault, Dir: QDefault}
	b := GenScode(cs, "localhost", q)
	if b == nil {
		t.Fatal("GenScode(localhost) returned nil")
	}
}

func TestGenScodePort(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QPort, Proto: QDefault, Dir: QDefault}
	b := GenScode(cs, "http", q)
	if b == nil {
		t.Fatal("GenScode(http) returned nil")
	}
}

func TestGenScodeProto(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QProto, Proto: QDefault, Dir: QDefault}
	b := GenScode(cs, "tcp", q)
	if b == nil {
		t.Fatal("GenScode(proto tcp) returned nil")
	}
}

func TestGenMcode(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QNet, Proto: QIP, Dir: QDefault}
	b := GenMcode(cs, "192.168.0.0", "", 16, q)
	if b == nil {
		t.Fatal("GenMcode(192.168.0.0/16) returned nil")
	}
}

func TestGenMcode6(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QHost, Proto: QDefault, Dir: QDefault}
	b := GenMcode6(cs, "::1", 128, q)
	if b == nil {
		t.Fatal("GenMcode6(::1/128) returned nil")
	}
}

func TestGenEcode(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QHost, Proto: QLink, Dir: QSrc}
	b := GenEcode(cs, "00:11:22:33:44:55", q)
	if b == nil {
		t.Fatal("GenEcode returned nil")
	}
}

func TestGenEcodeInvalidMAC(t *testing.T) {
	cs := newEthernetCSWithResolver()
	q := Qual{Addr: QHost, Proto: QLink, Dir: QSrc}
	b := GenEcode(cs, "invalid", q)
	if b != nil {
		t.Error("expected nil for invalid MAC")
	}
}

func TestParseIPv4Addr(t *testing.T) {
	tests := []struct {
		s    string
		bits int
		val  uint32
	}{
		{"10", 8, 10},
		{"10.1", 16, 0x0A01},
		{"10.1.2", 24, 0x0A0102},
		{"10.1.2.3", 32, 0x0A010203},
	}
	for _, tt := range tests {
		bits, val, err := parseIPv4Addr(tt.s)
		if err != nil {
			t.Errorf("parseIPv4Addr(%q) error: %v", tt.s, err)
			continue
		}
		if bits != tt.bits || val != tt.val {
			t.Errorf("parseIPv4Addr(%q) = (%d, %#x), want (%d, %#x)", tt.s, bits, val, tt.bits, tt.val)
		}
	}
}

func TestGenNcodeEndToEnd(t *testing.T) {
	cs := newEthernetCSWithResolver()
	// "tcp dst port 80" equivalent: qualifier = {port, tcp, dst}
	q := Qual{Addr: QPort, Proto: QTCP, Dir: QDst}
	b := GenNcode(cs, "", 80, q)
	if b == nil {
		t.Fatal("GenNcode returned nil")
	}
	err := FinishParse(cs, b)
	if err != nil {
		t.Fatalf("FinishParse = %v", err)
	}
	if cs.IC.Root == nil {
		t.Fatal("IC.Root is nil")
	}
}
