// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// Layout of skb_parse_result_t in skb-parse-test.c: ret, skb_info_t, conn_tuple_t.
const (
	skbParseInfoOff = 8
	skbParseTupOff  = 24
	skbParseSport   = skbParseTupOff + 32
)

type skbParseFrame struct {
	name string
	data []byte
	// bulk path must handle the untruncated frame
	expectBulk bool
}

// TestReadConnTupleSKBEquivalence runs read_conn_tuple_skb_bulk, read_conn_tuple_skb_fields, and
// read_conn_tuple_skb over a frame matrix at every truncation length and requires identical outputs.
func TestReadConnTupleSKBEquivalence(t *testing.T) {
	var prog *ebpf.Program
	var seedMap, outMap *ebpf.Map
	err := ddebpf.LoadCOREAsset("skb-parse-test.o", func(bc bytecode.AssetReader, opts manager.Options) error {
		spec, err := ebpf.LoadCollectionSpecFromReader(bc)
		if err != nil {
			return err
		}
		coll, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{KernelTypes: opts.VerifierOptions.Programs.KernelTypes},
		})
		if err != nil {
			return err
		}
		t.Cleanup(coll.Close)
		prog = coll.Programs["socket__skb_parse_test"]
		seedMap = coll.Maps["skb_parse_test_seed"]
		outMap = coll.Maps["skb_parse_test_out"]
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, prog)

	// Older kernels can't test-run socket filters; any other error still fails below.
	if _, err := prog.Run(&ebpf.RunOptions{Data: make([]byte, 14)}); errors.Is(err, ebpf.ErrNotSupported) {
		t.Skipf("kernel can't test-run socket filters: %s", err)
	}

	resSize := int(seedMap.ValueSize())
	require.Equal(t, 8+3*resSize, int(outMap.ValueSize()))

	seeds := [][]byte{make([]byte, resSize), bytes.Repeat([]byte{0xa5}, resSize)}
	frames := skbParseFrames()

	var runs, bulkRuns int
	key := uint32(0)
	for _, f := range frames {
		for n := 0; n <= len(f.data); n++ {
			for si, seed := range seeds {
				require.NoError(t, seedMap.Put(&key, seed))
				// BPF_PROG_TEST_RUN consumes a 14 byte Ethernet header for socket filters, so the
				// program sees the bytes after it at offset 0.
				in := append(make([]byte, 14), f.data[:n]...)
				in[12], in[13] = 0x08, 0x00
				_, err := prog.Run(&ebpf.RunOptions{Data: in})
				require.NoError(t, err)
				out := make([]byte, outMap.ValueSize())
				require.NoError(t, outMap.Lookup(&key, &out))

				bulkRet := int64(binary.NativeEndian.Uint64(out[0:8]))
				fields := out[8 : 8+resSize]
				bulk := out[8+resSize : 8+2*resSize]
				combined := out[8+2*resSize:]
				desc := fmt.Sprintf("%s len=%d seed=%d", f.name, n, si)

				require.Equal(t, fields, combined, "combined differs from per-field: %s", desc)
				switch {
				case bulkRet >= 0:
					bulkRuns++
					require.Equal(t, fields, bulk, "bulk differs from per-field: %s", desc)
				case bulkRet == -1:
					require.Equal(t, seed[skbParseInfoOff:], bulk[skbParseInfoOff:], "bulk fallback wrote outputs: %s", desc)
				default:
					t.Fatalf("unexpected bulk return %d: %s", bulkRet, desc)
				}
				if n == len(f.data) && f.expectBulk {
					require.GreaterOrEqual(t, bulkRet, int64(0), "bulk path not taken: %s", desc)
				}
				runs++
			}
		}
	}
	t.Logf("%d frames, %d runs, %d on the bulk path", len(frames), runs, bulkRuns)

	// Sanity check the harness offsets on one frame: an IPv4 TCP frame parses to its source port.
	require.NoError(t, seedMap.Put(&key, seeds[0]))
	in := append(make([]byte, 14), frames[0].data...)
	_, err = prog.Run(&ebpf.RunOptions{Data: in})
	require.NoError(t, err)
	out := make([]byte, outMap.ValueSize())
	require.NoError(t, outMap.Lookup(&key, &out))
	require.Equal(t, uint64(1), binary.NativeEndian.Uint64(out[8:16]))
	require.Equal(t, uint16(40000), binary.NativeEndian.Uint16(out[8+skbParseSport:]))
}

func ethHdr(proto uint16) []byte {
	b := make([]byte, 14)
	for i := 0; i < 12; i++ {
		b[i] = byte(i + 1)
	}
	binary.BigEndian.PutUint16(b[12:], proto)
	return b
}

func ipv4Hdr(ihlWords int, proto byte, l4Len int) []byte {
	hl := ihlWords * 4
	if hl < 20 {
		hl = 20
	}
	b := make([]byte, hl)
	b[0] = 0x40 | byte(ihlWords&0x0f)
	binary.BigEndian.PutUint16(b[2:], uint16(hl+l4Len))
	b[8] = 64
	b[9] = proto
	copy(b[12:], []byte{10, 1, 2, 3})
	copy(b[16:], []byte{192, 168, 200, 7})
	for i := 20; i < hl; i++ {
		b[i] = 0x01 // NOP options
	}
	return b
}

func ipv6Hdr(nexthdr byte, payloadLen int) []byte {
	b := make([]byte, 40)
	b[0] = 0x60
	binary.BigEndian.PutUint16(b[4:], uint16(payloadLen))
	b[6] = nexthdr
	b[7] = 64
	for i := 0; i < 16; i++ {
		b[8+i] = byte(0x20 + i)
		b[24+i] = byte(0xf0 - i)
	}
	return b
}

func tcpHdr(sport, dport uint16, doffWords int, flags byte, payload int) []byte {
	b := make([]byte, doffWords*4+payload)
	binary.BigEndian.PutUint16(b[0:], sport)
	binary.BigEndian.PutUint16(b[2:], dport)
	binary.BigEndian.PutUint32(b[4:], 0x11223344)
	binary.BigEndian.PutUint32(b[8:], 0x55667788)
	b[12] = byte(doffWords << 4)
	b[13] = flags
	binary.BigEndian.PutUint16(b[14:], 0xffff)
	for i := doffWords * 4; i < len(b); i++ {
		b[i] = byte('A' + i%26)
	}
	return b
}

func udpHdr(sport, dport uint16, payload int) []byte {
	b := make([]byte, 8+payload)
	binary.BigEndian.PutUint16(b[0:], sport)
	binary.BigEndian.PutUint16(b[2:], dport)
	binary.BigEndian.PutUint16(b[4:], uint16(8+payload))
	for i := 8; i < len(b); i++ {
		b[i] = byte(i)
	}
	return b
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func skbParseFrames() []skbParseFrame {
	const tcp, udp, icmp, icmp6 = 6, 17, 1, 58
	v4 := func(ihl int, proto byte, l4 []byte) []byte {
		return concat(ethHdr(0x0800), ipv4Hdr(ihl, proto, len(l4)), l4)
	}
	v6 := func(next byte, l4 []byte) []byte {
		return concat(ethHdr(0x86dd), ipv6Hdr(next, len(l4)), l4)
	}
	tcpData := tcpHdr(40000, 6379, 5, 0x18, 40)
	frames := []skbParseFrame{
		{"v4 tcp data", v4(5, tcp, tcpData), true},
		{"v4 tcp ack", v4(5, tcp, tcpHdr(40000, 80, 5, 0x10, 0)), true},
		{"v4 tcp ack timestamps", v4(5, tcp, tcpHdr(40000, 80, 8, 0x10, 0)), true},
		{"v4 tcp fin", v4(5, tcp, tcpHdr(40000, 80, 5, 0x11, 0)), true},
		{"v4 tcp rst", v4(5, tcp, tcpHdr(40000, 80, 5, 0x04, 0)), true},
		{"v4 tcp doff 15", v4(5, tcp, tcpHdr(40000, 443, 15, 0x18, 10)), true},
		{"v4 tcp doff 0", v4(5, tcp, tcpHdr(40000, 443, 0, 0x18, 30)), true},
		{"v4 opts ihl 6 tcp", v4(6, tcp, tcpData), false},
		{"v4 opts ihl 15 tcp", v4(15, tcp, tcpData), false},
		{"v4 opts ihl 6 udp", v4(6, udp, udpHdr(5353, 53, 30)), false},
		{"v4 ihl 0", v4(0, tcp, tcpData), true},
		{"v4 ihl 4", v4(4, tcp, tcpData), true},
		{"v4 udp dns", v4(5, udp, udpHdr(40001, 53, 33)), true},
		{"v4 udp empty", v4(5, udp, udpHdr(40001, 53, 0)), false},
		{"v4 icmp", v4(5, icmp, make([]byte, 56)), true},
		{"v4 gre", v4(5, 47, make([]byte, 40)), true},
		{"v6 tcp data", v6(tcp, tcpData), true},
		{"v6 tcp ack", v6(tcp, tcpHdr(40000, 80, 5, 0x10, 0)), true},
		{"v6 udp dns", v6(udp, udpHdr(40001, 53, 33)), true},
		{"v6 udp empty", v6(udp, udpHdr(40001, 53, 0)), false},
		{"v6 icmpv6", v6(icmp6, make([]byte, 32)), true},
		{"v6 hop-by-hop then tcp", v6(0, concat([]byte{tcp, 0, 1, 4, 0, 0, 0, 0}, tcpData)), true},
		{"arp", concat(ethHdr(0x0806), make([]byte, 28)), false},
		{"arp padded", concat(ethHdr(0x0806), make([]byte, 46)), true},
		{"vlan v4 tcp", concat(ethHdr(0x8100), []byte{0, 1, 0x08, 0x00}, ipv4Hdr(5, tcp, len(tcpData)), tcpData), true},
		{"unknown ethertype", concat(ethHdr(0x88cc), make([]byte, 60)), true},
	}

	// tot_len/payload_len that disagree with the frame, including lengths shorter than the headers.
	short := v4(5, tcp, tcpData)
	binary.BigEndian.PutUint16(short[16:], 10)
	frames = append(frames, skbParseFrame{"v4 tot_len short", short, true})
	zero6 := v6(tcp, tcpData)
	binary.BigEndian.PutUint16(zero6[18:], 0)
	frames = append(frames, skbParseFrame{"v6 payload_len 0", zero6, true})

	// Random headers with the ethertype, version/IHL, and protocol fields biased toward real values.
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 300; i++ {
		b := make([]byte, 14+r.Intn(110))
		r.Read(b)
		switch r.Intn(4) {
		case 0, 1:
			binary.BigEndian.PutUint16(b[12:], 0x0800)
			if len(b) > 23 && r.Intn(2) == 0 {
				b[14] = 0x45
				b[23] = []byte{tcp, udp}[r.Intn(2)]
			}
		case 2:
			binary.BigEndian.PutUint16(b[12:], 0x86dd)
			if len(b) > 20 && r.Intn(2) == 0 {
				b[20] = []byte{tcp, udp}[r.Intn(2)]
			}
		}
		frames = append(frames, skbParseFrame{fmt.Sprintf("random %d", i), b, false})
	}
	return frames
}
