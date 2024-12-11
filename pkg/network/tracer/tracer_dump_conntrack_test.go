// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/netlink"
)

func getTestTableTwoEntries() DebugConntrackTable {
	return DebugConntrackTable{
		Kind:   "test-kind",
		RootNS: 1234,
		Entries: map[uint32][]netlink.DebugConntrackEntry{
			1234: {
				{
					Proto:  "TCP",
					Family: "v4",
					Origin: netlink.DebugConntrackTuple{
						Src: netip.MustParseAddrPort("127.0.0.1:10000"),
						Dst: netip.MustParseAddrPort("127.0.0.2:10001"),
					},
					Reply: netlink.DebugConntrackTuple{
						Src: netip.MustParseAddrPort("127.0.0.3:10002"),
						Dst: netip.MustParseAddrPort("127.0.0.4:10003"),
					},
				},
				{
					Proto:  "TCP",
					Family: "v4",
					Origin: netlink.DebugConntrackTuple{
						Src: netip.MustParseAddrPort("10.0.0.1:11000"),
						Dst: netip.MustParseAddrPort("10.0.0.2:11001"),
					},
					Reply: netlink.DebugConntrackTuple{
						Src: netip.MustParseAddrPort("10.0.0.3:11002"),
						Dst: netip.MustParseAddrPort("10.0.0.4:11003"),
					},
				},
			},
		},
	}
}
func getIPv6EntryExample() netlink.DebugConntrackEntry {
	return netlink.DebugConntrackEntry{
		Proto:  "UDP",
		Family: "v6",
		Origin: netlink.DebugConntrackTuple{
			Src: netip.MustParseAddrPort("[::9999:0001]:12000"),
			Dst: netip.MustParseAddrPort("[::9999:0002]:12001"),
		},
		Reply: netlink.DebugConntrackTuple{
			Src: netip.MustParseAddrPort("[::9999:0003]:12002"),
			Dst: netip.MustParseAddrPort("[::9999:0004]:12003"),
		},
	}
}

func TestDumpingConntrack(t *testing.T) {
	sb := strings.Builder{}
	table := getTestTableTwoEntries()
	err := table.WriteTo(&sb, 99)
	require.NoError(t, err)
	expected := `conntrack dump, kind=test-kind rootNS=1234
totalEntries=2
namespace 1234, size=2:
TCP v4 src=10.0.0.1 dst=10.0.0.2 sport=11000 dport=11001 src=10.0.0.3 dst=10.0.0.4 sport=11002 dport=11003
TCP v4 src=127.0.0.1 dst=127.0.0.2 sport=10000 dport=10001 src=127.0.0.3 dst=127.0.0.4 sport=10002 dport=10003
`
	require.Equal(t, expected, sb.String())
}

func TestDumpingConntrackSizeLimit(t *testing.T) {
	sb := strings.Builder{}
	table := getTestTableTwoEntries()
	err := table.WriteTo(&sb, 1)
	require.NoError(t, err)

	expected := `conntrack dump, kind=test-kind rootNS=1234
totalEntries=2, capped to 1 to reduce output size
namespace 1234, size=2:
TCP v4 src=10.0.0.1 dst=10.0.0.2 sport=11000 dport=11001 src=10.0.0.3 dst=10.0.0.4 sport=11002 dport=11003
<reached max entries, skipping remaining 1 entries...>
`
	require.Equal(t, expected, sb.String())
}

func TestDumpingConntrackSizeLimitWithTruncation(t *testing.T) {
	sb := strings.Builder{}
	table := getTestTableTwoEntries()
	table.IsTruncated = true
	err := table.WriteTo(&sb, 1)
	require.NoError(t, err)

	expected := `conntrack dump, kind=test-kind rootNS=1234
totalEntries=2, capped to 1 to reduce output size
netlink table truncated due to response timeout, some entries may be missing
namespace 1234, size=2:
TCP v4 src=10.0.0.1 dst=10.0.0.2 sport=11000 dport=11001 src=10.0.0.3 dst=10.0.0.4 sport=11002 dport=11003
<reached max entries, skipping remaining 1 entries...>
`
	require.Equal(t, expected, sb.String())
}

func TestDumpingConntrackIPv6(t *testing.T) {
	sb := strings.Builder{}
	table := getTestTableTwoEntries()
	table.Entries[1234] = append(table.Entries[1234], getIPv6EntryExample())

	err := table.WriteTo(&sb, 99)
	require.NoError(t, err)

	expected :=
		`conntrack dump, kind=test-kind rootNS=1234
totalEntries=3
namespace 1234, size=3:
TCP v4 src=10.0.0.1 dst=10.0.0.2 sport=11000 dport=11001 src=10.0.0.3 dst=10.0.0.4 sport=11002 dport=11003
TCP v4 src=127.0.0.1 dst=127.0.0.2 sport=10000 dport=10001 src=127.0.0.3 dst=127.0.0.4 sport=10002 dport=10003
UDP v6 src=::9999:1 dst=::9999:2 sport=12000 dport=12001 src=::9999:3 dst=::9999:4 sport=12002 dport=12003
`
	require.Equal(t, expected, sb.String())
}

func TestDumpingConntrackInAnotherNamespace(t *testing.T) {
	sb := strings.Builder{}
	table := getTestTableTwoEntries()
	table.Entries[9999] = append(table.Entries[9999], getIPv6EntryExample())

	err := table.WriteTo(&sb, 99)
	require.NoError(t, err)

	expected :=
		`conntrack dump, kind=test-kind rootNS=1234
totalEntries=3
namespace 1234, size=2:
TCP v4 src=10.0.0.1 dst=10.0.0.2 sport=11000 dport=11001 src=10.0.0.3 dst=10.0.0.4 sport=11002 dport=11003
TCP v4 src=127.0.0.1 dst=127.0.0.2 sport=10000 dport=10001 src=127.0.0.3 dst=127.0.0.4 sport=10002 dport=10003
namespace 9999, size=1:
UDP v6 src=::9999:1 dst=::9999:2 sport=12000 dport=12001 src=::9999:3 dst=::9999:4 sport=12002 dport=12003
`
	require.Equal(t, expected, sb.String())
}

type testBadWriter struct{}

func (testBadWriter) Write(_ []byte) (n int, err error) {
	return 0, errors.New("bad writer")
}

func TestDumpingConntrackWriteError(t *testing.T) {
	table := getTestTableTwoEntries()
	err := table.WriteTo(&testBadWriter{}, 99)
	require.ErrorContains(t, err, "bad writer")
}
