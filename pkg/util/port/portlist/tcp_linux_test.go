// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build linux

package portlist

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"testing"
)

func TestTCPListenersReplaceProcNet(t *testing.T) {
	// A real listener that the source omits must not show up: /proc/net is skipped.
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	realPort := uint16(l.Addr().(*net.TCPAddr).Port)

	any4 := netip.IPv4Unspecified()
	lo4 := netip.MustParseAddr("127.0.0.1")
	src := []TCPListener{
		{IP: any4, Port: 1, Inode: 1 << 30},
		{IP: lo4, Port: 2, Inode: 1<<30 + 1, Loopback: true},
	}
	for _, includeLocalhost := range []bool{true, false} {
		t.Run(fmt.Sprintf("includeLocalhost=%v", includeLocalhost), func(t *testing.T) {
			ls := src
			p := NewTCPPoller(includeLocalhost, func() ([]TCPListener, error) { return ls, nil })
			got, _, err := p.Poll()
			if err != nil {
				t.Fatal(err)
			}
			want := []Port{{Proto: "tcp", IP: any4, Port: 1}}
			if includeLocalhost {
				want = append(want, Port{Proto: "tcp", IP: lo4, Port: 2})
			}
			if !slices.Equal(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
			if slices.ContainsFunc(got, func(p Port) bool { return p.Port == realPort }) {
				t.Fatalf("/proc/net listener on port %d leaked into %v", realPort, got)
			}

			// A listener the source stops reporting is dropped on the next poll.
			ls = src[1:]
			got, _, err = p.Poll()
			if err != nil {
				t.Fatal(err)
			}
			if slices.ContainsFunc(got, func(p Port) bool { return p.Port == 1 }) {
				t.Fatalf("stale listener kept: %v", got)
			}
		})
	}
}

func TestTCPPollerFallsBackToProcNet(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	lport := uint16(l.Addr().(*net.TCPAddr).Port)
	u, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()

	calls := 0
	failing := func() ([]TCPListener, error) {
		calls++
		return nil, errors.New("forced TCPListeners failure")
	}

	var diff string
	for attempt := 0; attempt < 5; attempt++ {
		got, _, err := NewTCPPoller(true, failing).Poll()
		if err != nil {
			t.Fatal(err)
		}
		all, _, err := (&Poller{IncludeLocalhost: true}).Poll()
		if err != nil {
			t.Fatal(err)
		}
		var want []Port
		for _, p := range all {
			if p.Proto == "tcp" {
				want = append(want, p)
			}
		}
		diff = ""
		if !slices.Equal(want, got) {
			diff = fmt.Sprintf("want %v\ngot  %v", want, got)
		}
		if diff == "" {
			if !slices.ContainsFunc(got, func(p Port) bool { return p.Port == lport && p.Pid == os.Getpid() }) {
				t.Fatalf("fallback result is missing our listener on port %d: %v", lport, got)
			}
			break
		}
	}
	if diff != "" {
		t.Fatalf("fallback differs from /proc/net TCP entries:\n%s", diff)
	}
	if calls == 0 {
		t.Fatal("TCPListeners was never called")
	}
}
