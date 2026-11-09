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
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// Layout of skb_parse_result_t in skb-parse-test.c: ret, skb_info_t, conn_tuple_t.
const (
	skbParseTupOff = 24
	skbParseSport  = skbParseTupOff + 32
)

type skbParseFrame struct {
	name string
	data []byte
}

// TestReadConnTupleSKBEquivalence runs read_conn_tuple_skb_bulk, read_conn_tuple_skb_fields, and
// read_conn_tuple_skb over a frame matrix at every truncation length and requires identical outputs.
func TestReadConnTupleSKBEquivalence(t *testing.T) {
	// Socket filters can't call bpf_skb_load_bytes before 4.11, the classification minimum.
	if kv := kernel.MustHostVersion(); kv < kernel.VersionCode(4, 11, 0) {
		t.Skipf("socket filters can't use bpf_skb_load_bytes on kernel %s", kv)
	}

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

	var runs, rejected int
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
				if testRunRejectsShortIPv4(in, err) {
					rejected++
					continue
				}
				require.NoError(t, err)
				out := make([]byte, outMap.ValueSize())
				require.NoError(t, outMap.Lookup(&key, &out))

				bulkRet := int64(binary.NativeEndian.Uint64(out[0:8]))
				fields := out[8 : 8+resSize]
				bulk := out[8+resSize : 8+2*resSize]
				combined := out[8+2*resSize:]
				desc := fmt.Sprintf("%s len=%d seed=%d", f.name, n, si)

				require.Equal(t, fields, combined, "combined differs from per-field: %s", desc)
				// bpf_skb_load_bytes is always available here, so the bulk parse handles every frame.
				require.GreaterOrEqual(t, bulkRet, int64(0), "bulk path not taken: %s", desc)
				require.Equal(t, fields, bulk, "bulk differs from per-field: %s", desc)
				runs++
			}
		}
	}
	t.Logf("%d frames, %d runs, %d short IPv4 inputs rejected by test-run", len(frames), runs, rejected)

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

// testRunRejectsShortIPv4 reports whether err is the EINVAL newer kernels return from
// BPF_PROG_TEST_RUN for an ETH_P_IP frame without a full IPv4 header: a harness limit, not parser behavior.
func testRunRejectsShortIPv4(in []byte, err error) bool {
	return errors.Is(err, unix.EINVAL) && binary.BigEndian.Uint16(in[12:14]) == unix.ETH_P_IP && len(in)-14 < 20
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
		{"v4 tcp data", v4(5, tcp, tcpData)},
		{"v4 tcp ack", v4(5, tcp, tcpHdr(40000, 80, 5, 0x10, 0))},
		{"v4 tcp ack timestamps", v4(5, tcp, tcpHdr(40000, 80, 8, 0x10, 0))},
		{"v4 tcp fin", v4(5, tcp, tcpHdr(40000, 80, 5, 0x11, 0))},
		{"v4 tcp rst", v4(5, tcp, tcpHdr(40000, 80, 5, 0x04, 0))},
		{"v4 tcp doff 15", v4(5, tcp, tcpHdr(40000, 443, 15, 0x18, 10))},
		{"v4 tcp doff 0", v4(5, tcp, tcpHdr(40000, 443, 0, 0x18, 30))},
		{"v4 opts ihl 6 tcp", v4(6, tcp, tcpData)},
		{"v4 opts ihl 15 tcp", v4(15, tcp, tcpData)},
		{"v4 opts ihl 6 udp", v4(6, udp, udpHdr(5353, 53, 30))},
		{"v4 ihl 0", v4(0, tcp, tcpData)},
		{"v4 ihl 4", v4(4, tcp, tcpData)},
		{"v4 udp dns", v4(5, udp, udpHdr(40001, 53, 33))},
		{"v4 udp empty", v4(5, udp, udpHdr(40001, 53, 0))},
		{"v4 icmp", v4(5, icmp, make([]byte, 56))},
		{"v4 gre", v4(5, 47, make([]byte, 40))},
		{"v6 tcp data", v6(tcp, tcpData)},
		{"v6 tcp ack", v6(tcp, tcpHdr(40000, 80, 5, 0x10, 0))},
		{"v6 udp dns", v6(udp, udpHdr(40001, 53, 33))},
		{"v6 udp empty", v6(udp, udpHdr(40001, 53, 0))},
		{"v6 icmpv6", v6(icmp6, make([]byte, 32))},
		{"v6 hop-by-hop then tcp", v6(0, concat([]byte{tcp, 0, 1, 4, 0, 0, 0, 0}, tcpData))},
		{"arp", concat(ethHdr(0x0806), make([]byte, 28))},
		{"arp padded", concat(ethHdr(0x0806), make([]byte, 46))},
		{"vlan v4 tcp", concat(ethHdr(0x8100), []byte{0, 1, 0x08, 0x00}, ipv4Hdr(5, tcp, len(tcpData)), tcpData)},
		{"unknown ethertype", concat(ethHdr(0x88cc), make([]byte, 60))},
	}

	// tot_len/payload_len that disagree with the frame, including lengths shorter than the headers.
	short := v4(5, tcp, tcpData)
	binary.BigEndian.PutUint16(short[16:], 10)
	frames = append(frames, skbParseFrame{"v4 tot_len short", short})
	zero6 := v6(tcp, tcpData)
	binary.BigEndian.PutUint16(zero6[18:], 0)
	frames = append(frames, skbParseFrame{"v6 payload_len 0", zero6})

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
		frames = append(frames, skbParseFrame{fmt.Sprintf("random %d", i), b})
	}
	return frames
}
