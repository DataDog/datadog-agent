// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"context"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// DebugConntrackEntry is a entry in a conntrack table (host or cached).
type DebugConntrackEntry struct {
	Proto  string
	Family string
	Origin DebugConntrackTuple
	Reply  DebugConntrackTuple
}

// DebugConntrackTuple is one side of a conntrack entry
type DebugConntrackTuple struct {
	Src DebugConntrackAddress
	Dst DebugConntrackAddress
}

// DebugConntrackAddress is an endpoint for one part of a conntrack tuple
type DebugConntrackAddress struct {
	IP   string
	Port uint16
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

	for _, k := range keys {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		ck, ok := k.(connKey)
		if !ok {
			continue
		}
		v, ok := ctr.cache.cache.Peek(ck)
		if !ok {
			continue
		}
		te, ok := v.(*translationEntry)
		if !ok {
			continue
		}

		table[ns] = append(table[ns], DebugConntrackEntry{
			Family: ck.transport.String(),
			Origin: DebugConntrackTuple{
				Src: DebugConntrackAddress{
					IP:   ck.src.Addr().String(),
					Port: ck.src.Port(),
				},
				Dst: DebugConntrackAddress{
					IP:   ck.dst.Addr().String(),
					Port: ck.dst.Port(),
				},
			},
			Reply: DebugConntrackTuple{
				Src: DebugConntrackAddress{
					IP:   te.ReplSrcIP.String(),
					Port: te.ReplSrcPort,
				},
				Dst: DebugConntrackAddress{
					IP:   te.ReplDstIP.String(),
					Port: te.ReplDstPort,
				},
			},
		})
	}
	return table, nil
}

// DumpHostTable dumps the host conntrack NAT entries grouped by network namespace
func DumpHostTable(ctx context.Context, cfg *config.Config) (map[uint32][]DebugConntrackEntry, error) {
	consumer, err := NewConsumer(cfg)
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
							Src: DebugConntrackAddress{
								IP:   src.src.Addr().String(),
								Port: src.src.Port(),
							},
							Dst: DebugConntrackAddress{
								IP:   src.dst.Addr().String(),
								Port: src.dst.Port(),
							},
						},
						Reply: DebugConntrackTuple{
							Src: DebugConntrackAddress{
								IP:   dst.src.Addr().String(),
								Port: dst.src.Port(),
							},
							Dst: DebugConntrackAddress{
								IP:   dst.dst.Addr().String(),
								Port: dst.dst.Port(),
							},
						},
					})
				}
			}
		}
	}
	return table, nil
}
