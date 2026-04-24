// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grammar

import "testing"

func scanAll(input string) []int {
	s := NewScanner(input)
	var tokens []int
	var lval yySymType
	for {
		tok := s.Lex(&lval)
		if tok == 0 {
			break
		}
		tokens = append(tokens, tok)
	}
	return tokens
}

func TestScannerEmpty(t *testing.T) {
	tokens := scanAll("")
	if len(tokens) != 0 {
		t.Errorf("expected no tokens, got %v", tokens)
	}
}

func TestScannerWhitespace(t *testing.T) {
	tokens := scanAll("   \t\n\r  ")
	if len(tokens) != 0 {
		t.Errorf("expected no tokens, got %v", tokens)
	}
}

func TestScannerProtocols(t *testing.T) {
	tests := map[string]int{
		"tcp":   TCP,
		"udp":   UDP,
		"icmp":  ICMP,
		"arp":   ARP,
		"rarp":  RARP,
		"ip":    IP,
		"ip6":   IPV6,
		"sctp":  SCTP,
		"igmp":  IGMP,
		"pim":   PIM,
		"icmp6": ICMPV6,
		"ah":    AH,
		"esp":   ESP,
		"vrrp":  VRRP,
		"carp":  CARP,
	}
	for word, want := range tests {
		var lval yySymType
		s := NewScanner(word)
		got := s.Lex(&lval)
		if got != want {
			t.Errorf("Lex(%q) = %d, want %d", word, got, want)
		}
	}
}

func TestScannerDirections(t *testing.T) {
	var lval yySymType

	s := NewScanner("src")
	if tok := s.Lex(&lval); tok != SRC {
		t.Errorf("expected SRC, got %d", tok)
	}

	s = NewScanner("dst")
	if tok := s.Lex(&lval); tok != DST {
		t.Errorf("expected DST, got %d", tok)
	}
}

func TestScannerQualifiers(t *testing.T) {
	tests := map[string]int{
		"host":      HOST,
		"net":       NET,
		"port":      PORT,
		"portrange": PORTRANGE,
		"proto":     PROTO,
		"gateway":   GATEWAY,
	}
	for word, want := range tests {
		var lval yySymType
		s := NewScanner(word)
		got := s.Lex(&lval)
		if got != want {
			t.Errorf("Lex(%q) = %d, want %d", word, got, want)
		}
	}
}

func TestScannerNumber(t *testing.T) {
	var lval yySymType

	s := NewScanner("80")
	if tok := s.Lex(&lval); tok != NUM {
		t.Fatalf("expected NUM, got %d", tok)
	}
	if lval.h != 80 {
		t.Errorf("value = %d, want 80", lval.h)
	}
}

func TestScannerHexNumber(t *testing.T) {
	var lval yySymType

	s := NewScanner("0x0800")
	if tok := s.Lex(&lval); tok != NUM {
		t.Fatalf("expected NUM, got %d", tok)
	}
	if lval.h != 0x0800 {
		t.Errorf("value = %#x, want 0x0800", lval.h)
	}
}

func TestScannerDottedAddr(t *testing.T) {
	var lval yySymType

	s := NewScanner("192.168.1.1")
	if tok := s.Lex(&lval); tok != HID {
		t.Fatalf("expected HID, got %d", tok)
	}
	if lval.s != "192.168.1.1" {
		t.Errorf("value = %q, want 192.168.1.1", lval.s)
	}
}

func TestScannerIPv6(t *testing.T) {
	var lval yySymType

	s := NewScanner("::1")
	if tok := s.Lex(&lval); tok != HID6 {
		t.Fatalf("expected HID6, got %d", tok)
	}
	if lval.s != "::1" {
		t.Errorf("value = %q, want ::1", lval.s)
	}
}

func TestScannerIPv6Full(t *testing.T) {
	var lval yySymType

	s := NewScanner("fe80::1")
	if tok := s.Lex(&lval); tok != HID6 {
		t.Fatalf("expected HID6 for fe80::1, got %d", tok)
	}
	if lval.s != "fe80::1" {
		t.Errorf("value = %q, want fe80::1", lval.s)
	}
}

func TestScannerMAC(t *testing.T) {
	var lval yySymType

	s := NewScanner("00:11:22:33:44:55")
	if tok := s.Lex(&lval); tok != EID {
		t.Fatalf("expected EID, got %d", tok)
	}
	if lval.s != "00:11:22:33:44:55" {
		t.Errorf("value = %q, want 00:11:22:33:44:55", lval.s)
	}
}

func TestScannerOperators(t *testing.T) {
	tokens := scanAll("( ) [ ] + - * / % & | ^ ! < > =")
	want := []int{'(', ')', '[', ']', '+', '-', '*', '/', '%', '&', '|', '^', '!', '<', '>', '='}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(want))
	}
	for i, tok := range tokens {
		if tok != want[i] {
			t.Errorf("token[%d] = %d, want %d", i, tok, want[i])
		}
	}
}

func TestScannerTwoCharOps(t *testing.T) {
	tests := map[string]int{
		">=": GEQ,
		"<=": LEQ,
		"!=": NEQ,
		"==": int('='),
		"<<": LSH,
		">>": RSH,
		"&&": AND,
		"||": OR,
	}
	for input, want := range tests {
		var lval yySymType
		s := NewScanner(input)
		got := s.Lex(&lval)
		if got != want {
			t.Errorf("Lex(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestScannerBoolOps(t *testing.T) {
	var lval yySymType

	s := NewScanner("and")
	if tok := s.Lex(&lval); tok != AND {
		t.Errorf("expected AND, got %d", tok)
	}

	s = NewScanner("or")
	if tok := s.Lex(&lval); tok != OR {
		t.Errorf("expected OR, got %d", tok)
	}

	s = NewScanner("not")
	if tok := s.Lex(&lval); tok != int('!') {
		t.Errorf("expected '!', got %d", tok)
	}
}

func TestScannerTCPFlags(t *testing.T) {
	tests := map[string]uint32{
		"tcp-syn":  0x02,
		"tcp-ack":  0x10,
		"tcp-fin":  0x01,
		"tcp-rst":  0x04,
		"tcp-push": 0x08,
		"tcp-urg":  0x20,
		"tcpflags": 13,
	}
	for word, wantVal := range tests {
		var lval yySymType
		s := NewScanner(word)
		tok := s.Lex(&lval)
		if tok != NUM {
			t.Errorf("Lex(%q) token = %d, want NUM(%d)", word, tok, NUM)
			continue
		}
		if lval.h != wantVal {
			t.Errorf("Lex(%q) value = %d, want %d", word, lval.h, wantVal)
		}
	}
}

func TestScannerICMPTypes(t *testing.T) {
	tests := map[string]uint32{
		"icmp-echo":      8,
		"icmp-echoreply": 0,
		"icmp-unreach":   3,
	}
	for word, wantVal := range tests {
		var lval yySymType
		s := NewScanner(word)
		tok := s.Lex(&lval)
		if tok != NUM {
			t.Errorf("Lex(%q) token = %d, want NUM", word, tok)
			continue
		}
		if lval.h != wantVal {
			t.Errorf("Lex(%q) value = %d, want %d", word, lval.h, wantVal)
		}
	}
}

func TestScannerComplexFilter(t *testing.T) {
	tokens := scanAll("tcp dst port 80")
	if len(tokens) != 4 {
		t.Fatalf("got %d tokens, want 4: %v", len(tokens), tokens)
	}
	if tokens[0] != TCP {
		t.Errorf("token[0] = %d, want TCP(%d)", tokens[0], TCP)
	}
	if tokens[1] != DST {
		t.Errorf("token[1] = %d, want DST(%d)", tokens[1], DST)
	}
	if tokens[2] != PORT {
		t.Errorf("token[2] = %d, want PORT(%d)", tokens[2], PORT)
	}
	if tokens[3] != NUM {
		t.Errorf("token[3] = %d, want NUM(%d)", tokens[3], NUM)
	}
}

func TestScannerBracketExpression(t *testing.T) {
	tokens := scanAll("tcp[13] & 0x02 != 0")
	// tcp [ 13 ] & 0x02 != 0
	want := []int{TCP, int('['), NUM, int(']'), int('&'), NUM, NEQ, NUM}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(tokens), len(want), tokens)
	}
	for i, tok := range tokens {
		if tok != want[i] {
			t.Errorf("token[%d] = %d, want %d", i, tok, want[i])
		}
	}
}

func TestScannerIdentifier(t *testing.T) {
	var lval yySymType
	s := NewScanner("myhost.example.com")
	tok := s.Lex(&lval)
	if tok != ID {
		t.Fatalf("expected ID, got %d", tok)
	}
	if lval.s != "myhost.example.com" {
		t.Errorf("value = %q, want myhost.example.com", lval.s)
	}
}

func TestScannerVLAN(t *testing.T) {
	tests := map[string]int{
		"vlan":   VLAN,
		"mpls":   MPLS,
		"pppoed": PPPOED,
		"pppoes": PPPOES,
		"geneve": GENEVE,
	}
	for word, want := range tests {
		var lval yySymType
		s := NewScanner(word)
		got := s.Lex(&lval)
		if got != want {
			t.Errorf("Lex(%q) = %d, want %d", word, got, want)
		}
	}
}

func TestScannerLen(t *testing.T) {
	var lval yySymType

	for _, word := range []string{"len", "length"} {
		s := NewScanner(word)
		tok := s.Lex(&lval)
		if tok != LEN {
			t.Errorf("Lex(%q) = %d, want LEN(%d)", word, tok, LEN)
		}
	}
}
