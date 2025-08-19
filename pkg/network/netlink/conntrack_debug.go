// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/netip"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// DebugConntrackEntry is a entry in a conntrack table (host or cached).
type DebugConntrackEntry struct {
	Proto  string
	Family string
	Origin DebugConntrackTuple
	Reply  DebugConntrackTuple
}

// String roughly matches conntrack -L format
func (e DebugConntrackEntry) String() string {
	return fmt.Sprintf("%s %s %s %s", e.Proto, e.Family, e.Origin, e.Reply)
}

// Compare orders entries to get deterministic output in the flare
func (e DebugConntrackEntry) Compare(o DebugConntrackEntry) int {
	return cmp.Or(
		cmp.Compare(e.Proto, o.Proto),
		cmp.Compare(e.Family, o.Family),
		e.Origin.Compare(o.Origin),
		e.Reply.Compare(o.Reply),
	)
}

// DebugConntrackTuple is one side of a conntrack entry
type DebugConntrackTuple struct {
	Src netip.AddrPort
	Dst netip.AddrPort
}

// String roughly matches conntrack -L format
func (t DebugConntrackTuple) String() string {
	return fmt.Sprintf("src=%s dst=%s sport=%d dport=%d", t.Src.Addr(), t.Dst.Addr(), t.Src.Port(), t.Dst.Port())
}

// Compare orders entries to get deterministic output in the flare
func (t DebugConntrackTuple) Compare(o DebugConntrackTuple) int {
	return cmp.Or(
		t.Src.Compare(o.Src),
		t.Dst.Compare(o.Dst),
	)
}

// DumpCachedTable dumps the cached conntrack NAT entries grouped by network namespace
func (ctr *realConntracker) DumpCachedTable(ctx context.Context) (map[uint32][]DebugConntrackEntry, error) {
	table := make(map[uint32][]DebugConntrackEntry)
	keys := ctr.cache.cache.Keys()
	if len(keys) == 0 {
		return table, nil
	}

	// netlink conntracker does not store netns values
	ns := uint32(0)
	table[ns] = []DebugConntrackEntry{}

	for _, ck := range keys {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		te, ok := ctr.cache.cache.Peek(ck)
		if !ok {
			continue
		}

		table[ns] = append(table[ns], DebugConntrackEntry{
			Family: ck.transport.String(),
			Origin: DebugConntrackTuple{
				Src: ck.src,
				Dst: ck.dst,
			},
			Reply: DebugConntrackTuple{
				Src: netip.AddrPortFrom(te.ReplSrcIP.Addr, te.ReplSrcPort),
				Dst: netip.AddrPortFrom(te.ReplDstIP.Addr, te.ReplDstPort),
			},
		})
	}
	return table, nil
}

// DumpHostTable dumps the host conntrack NAT entries grouped by network namespace
func DumpHostTable(ctx context.Context, cfg *config.Config, telemetryComp telemetry.Component) (map[uint32][]DebugConntrackEntry, error) {
	consumer, err := NewConsumer(cfg, telemetryComp)
	if err != nil {
		return nil, err
	}

	decoder := NewDecoder()
	defer consumer.Stop()

	table := make(map[uint32][]DebugConntrackEntry)

	for _, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		events, err := consumer.DumpTable(family)
		if err != nil {
			return nil, err
		}

		fstr := "v4"
		if family == unix.AF_INET6 {
			fstr = "v6"
		}

	dumploop:
		for {
			select {
			case <-ctx.Done():
				err = ctx.Err()
				// if we have exceeded the deadline, return partial data of what we have so far.
				if errors.Is(err, context.DeadlineExceeded) {
					break dumploop
				}
				return nil, ctx.Err()
			case ev, ok := <-events:
				if !ok {
					break dumploop
				}
				conns := decoder.DecodeAndReleaseEvent(ev)
				for _, c := range conns {
					if !IsNAT(c) {
						continue
					}

					ns := c.NetNS
					_, ok := table[ns]
					if !ok {
						table[ns] = []DebugConntrackEntry{}
					}

					src, sok := formatKey(&c.Origin)
					dst, dok := formatKey(&c.Reply)
					if !sok || !dok {
						continue
					}

					table[ns] = append(table[ns], DebugConntrackEntry{
						Family: fstr,
						Proto:  src.transport.String(),
						Origin: DebugConntrackTuple{
							Src: src.src,
							Dst: src.dst,
						},
						Reply: DebugConntrackTuple{
							Src: dst.src,
							Dst: dst.dst,
						},
					})
				}
			}
		}
	}
	return table, nil
}
