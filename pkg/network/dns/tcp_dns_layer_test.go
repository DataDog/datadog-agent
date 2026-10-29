// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || (linux && bpf) || darwin

package dns

import (
	"encoding/binary"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var (
	// tcpDNSFirstAnswerIP is the first A record in responses built by tcpDNSMessage
	tcpDNSFirstAnswerIP = net.IP{10, 1, 0, 0}
	tcpDNSClientIP      = net.IP{10, 0, 0, 1}
	tcpDNSServerIP      = net.IP{10, 0, 0, 2}
)

const (
	// replaySnapLenForTests matches the packet source's default snaplen
	replaySnapLenForTests = 4096
	tcpDNSClientPort      = 40000
	tcpDNSDomain          = "big.example.com"
)

// noopPacketSource lets tests build a snooper and feed it frames directly.
type noopPacketSource struct{}

func (noopPacketSource) VisitPackets(func([]byte, filter.PacketInfo, time.Time) error) error {
	return nil
}
func (noopPacketSource) LayerType() gopacket.LayerType { return layers.LayerTypeEthernet }
func (noopPacketSource) Close()                        {}

// tcpDNSHarness feeds raw frames to a snooper as if they were captured off the wire.
type tcpDNSHarness struct {
	t         *testing.T
	snooper   *socketFilterSnooper
	clientSeq uint32
	serverSeq uint32
	now       time.Time
}

func newTCPDNSHarness(t *testing.T) *tcpDNSHarness {
	cfg := config.New()
	cfg.CollectDNSStats = true
	cfg.CollectDNSDomains = true
	cfg.DNSTimeout = 15 * time.Second
	snooper, err := newSocketFilterSnooper(cfg, noopPacketSource{}, nil)
	require.NoError(t, err)
	t.Cleanup(snooper.Close)
	return &tcpDNSHarness{
		t:         t,
		snooper:   snooper,
		clientSeq: 1000,
		serverSeq: 5000,
		now:       time.Now(),
	}
}

// sendFromClient emits one TCP segment from client to server carrying payload.
func (h *tcpDNSHarness) sendFromClient(payload []byte) {
	h.feed(tcpDNSFrame(h.t, tcpDNSClientIP, tcpDNSServerIP, tcpDNSClientPort, 53, h.clientSeq, h.serverSeq, payload))
	h.clientSeq += uint32(len(payload))
}

// sendFromServer emits one TCP segment from server to client carrying payload.
func (h *tcpDNSHarness) sendFromServer(payload []byte) {
	h.feed(tcpDNSFrame(h.t, tcpDNSServerIP, tcpDNSClientIP, 53, tcpDNSClientPort, h.serverSeq, h.clientSeq, payload))
	h.serverSeq += uint32(len(payload))
}

func (h *tcpDNSHarness) feed(frame []byte) {
	h.now = h.now.Add(time.Millisecond)
	if len(frame) > replaySnapLenForTests {
		frame = frame[:replaySnapLenForTests] // as the packet source would
	}
	require.NoError(h.t, h.snooper.processPacket(frame, nil, h.now))
}

// finish sweeps every pending query (as the expiry ticker eventually would) and
// returns the stats of the test domain over the given transport protocol.
func (h *tcpDNSHarness) finish(protocol uint8) Stats {
	h.snooper.statKeeper.removeExpiredStates(h.now.Add(time.Hour))
	key := Key{
		ServerIP:   util.AddressFromNetIP(tcpDNSServerIP),
		ClientIP:   util.AddressFromNetIP(tcpDNSClientIP),
		ClientPort: tcpDNSClientPort,
		Protocol:   protocol,
	}
	all := h.snooper.statKeeper.GetAndResetAllStats()
	return all[key][ToHostname(tcpDNSDomain)][TypeA]
}

func TestTCPDNSReassembly(t *testing.T) {
	small := func(id uint16) []byte { return tcpDNSMessage(t, id, true, 1) }
	large := func(id uint16) []byte { return tcpDNSMessage(t, id, true, 200) } // ~6KB, larger than one 1448-byte MSS segment

	tests := []struct {
		name      string
		queries   int
		responses func(h *tcpDNSHarness)
		// cached is whether the first answer reaches the reverse DNS cache,
		// which needs the first response to be fully decoded
		cached bool
	}{
		{
			name:    "self-contained answer",
			queries: 1,
			responses: func(h *tcpDNSHarness) {
				h.sendFromServer(withTCPLength(small(1)))
			},
			cached: true,
		},
		{
			name:    "answer split across MSS-sized segments",
			queries: 1,
			responses: func(h *tcpDNSHarness) {
				for _, seg := range splitEvery(withTCPLength(large(1)), 1448) {
					h.sendFromServer(seg)
				}
			},
		},
		{
			name:    "two answers in one segment",
			queries: 2,
			responses: func(h *tcpDNSHarness) {
				h.sendFromServer(append(withTCPLength(small(1)), withTCPLength(small(2))...))
			},
			cached: true,
		},
		{
			name:    "length prefix in its own segment",
			queries: 1,
			responses: func(h *tcpDNSHarness) {
				msg := withTCPLength(small(1))
				h.sendFromServer(msg[:2])
				h.sendFromServer(msg[2:])
			},
		},
		{
			name:    "segment truncated by the snaplen after complete answers",
			queries: 3,
			responses: func(h *tcpDNSHarness) {
				payload := append(withTCPLength(small(1)), withTCPLength(small(2))...)
				h.sendFromServer(append(payload, withTCPLength(large(3))...))
			},
			cached: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decodingErrors := snooperTelemetry.decodingErrors.Load()
			h := newTCPDNSHarness(t)
			for i := 1; i <= tt.queries; i++ {
				h.sendFromClient(withTCPLength(tcpDNSMessage(t, uint16(i), false, 0)))
			}
			tt.responses(h)
			stats := h.finish(syscall.IPPROTO_TCP)

			assert.Equal(t, uint32(tt.queries), stats.CountByRcode[0], "every answer should be matched to its query")
			assert.Zero(t, stats.Timeouts, "answered queries must not be counted as timeouts")
			assert.Equal(t, decodingErrors, snooperTelemetry.decodingErrors.Load(), "no segment should count as a decoding error")
			resolved := h.snooper.Resolve(map[util.Address]struct{}{util.AddressFromNetIP(tcpDNSFirstAnswerIP): {}})
			assert.Equal(t, tt.cached, len(resolved) > 0, "first answer in the reverse DNS cache")
		})
	}
}

func TestTrailingResponseNeedsPlausibleLength(t *testing.T) {
	h := newTCPDNSHarness(t)
	h.sendFromClient(withTCPLength(tcpDNSMessage(t, 1, false, 0)))
	h.sendFromClient(withTCPLength(tcpDNSMessage(t, 2, false, 0)))
	// the second response follows a length field too small for a DNS header
	payload := append(withTCPLength(tcpDNSMessage(t, 1, true, 1)), 0, 0)
	h.sendFromServer(append(payload, tcpDNSMessage(t, 2, true, 1)...))
	stats := h.finish(syscall.IPPROTO_TCP)
	assert.Equal(t, uint32(1), stats.CountByRcode[0])
	assert.Equal(t, uint32(1), stats.Timeouts)
}

func TestTrailingResponses(t *testing.T) {
	t.Run("after a response for a query type that isn't recorded", func(t *testing.T) {
		h := newTCPDNSHarness(t)
		h.sendFromClient(withTCPLength(tcpDNSMessageOfType(t, 1, false, 0, layers.DNSTypeTXT)))
		h.sendFromClient(withTCPLength(tcpDNSMessage(t, 2, false, 0)))
		payload := withTCPLength(tcpDNSMessageOfType(t, 1, true, 0, layers.DNSTypeTXT))
		h.sendFromServer(append(payload, withTCPLength(tcpDNSMessage(t, 2, true, 1))...))
		stats := h.finish(syscall.IPPROTO_TCP)
		assert.Equal(t, uint32(1), stats.CountByRcode[0])
		assert.Zero(t, stats.Timeouts)
	})

	t.Run("after a message that isn't a standard response", func(t *testing.T) {
		h := newTCPDNSHarness(t)
		h.sendFromClient(withTCPLength(tcpDNSMessage(t, 1, false, 0)))
		h.sendFromClient(withTCPLength(tcpDNSMessage(t, 3, false, 0)))
		formErr := tcpDNSMessage(t, 9, true, 0)
		formErr[3] = formErr[3]&0xf0 | byte(layers.DNSResponseCodeFormErr)
		formErr[5] = 0 // no question
		payload := append(withTCPLength(tcpDNSMessage(t, 1, true, 1)), withTCPLength(formErr)...)
		h.sendFromServer(append(payload, withTCPLength(tcpDNSMessage(t, 3, true, 1))...))
		stats := h.finish(syscall.IPPROTO_TCP)
		assert.Equal(t, uint32(2), stats.CountByRcode[0])
		assert.Zero(t, stats.Timeouts)
	})

	t.Run("after a first response that can't be decoded", func(t *testing.T) {
		h := newTCPDNSHarness(t)
		h.sendFromClient(withTCPLength(tcpDNSMessage(t, 1, false, 0)))
		h.sendFromClient(withTCPLength(tcpDNSMessage(t, 2, false, 0)))
		broken := tcpDNSMessage(t, 1, true, 1)
		broken[len(broken)-6] = 0x40 // record data length past the end of the message
		h.sendFromServer(append(withTCPLength(broken), withTCPLength(tcpDNSMessage(t, 2, true, 1))...))
		stats := h.finish(syscall.IPPROTO_TCP)
		assert.Equal(t, uint32(2), stats.CountByRcode[0])
		assert.Zero(t, stats.Timeouts)
	})
}

func TestDNSResponseHeaderOnly(t *testing.T) {
	sendQuery := func(h *tcpDNSHarness) {
		h.feed(udpDNSFrameFrom(t, tcpDNSClientIP, tcpDNSServerIP, tcpDNSClientPort, 53, tcpDNSMessage(t, 7, false, 0)))
	}

	t.Run("udp response truncated by the snaplen", func(t *testing.T) {
		h := newTCPDNSHarness(t)
		sendQuery(h)
		h.feed(udpDNSFrame(t, tcpDNSMessage(t, 7, true, 400))[:4096])
		s := h.finish(syscall.IPPROTO_UDP)
		assert.Equal(t, uint32(1), s.CountByRcode[0])
		assert.Zero(t, s.Timeouts)
	})

	t.Run("failure response code is kept", func(t *testing.T) {
		h := newTCPDNSHarness(t)
		sendQuery(h)
		msg := tcpDNSMessage(t, 7, true, 400)
		msg[3] = msg[3]&0xf0 | byte(layers.DNSResponseCodeServFail)
		h.feed(udpDNSFrame(t, msg)[:4096])
		s := h.finish(syscall.IPPROTO_UDP)
		assert.Equal(t, uint32(1), s.CountByRcode[uint32(layers.DNSResponseCodeServFail)])
		assert.Zero(t, s.Timeouts)
	})

	t.Run("truncated response with another id doesn't match", func(t *testing.T) {
		h := newTCPDNSHarness(t)
		sendQuery(h)
		h.feed(udpDNSFrame(t, tcpDNSMessage(t, 8, true, 400))[:4096])
		s := h.finish(syscall.IPPROTO_UDP)
		assert.Zero(t, s.CountByRcode[0])
		assert.Equal(t, uint32(1), s.Timeouts)
	})
}

func TestSetResponseFromHeader(t *testing.T) {
	response := tcpDNSMessage(t, 42, true, 1)
	tests := []struct {
		name   string
		mutate func(msg []byte) []byte
		ok     bool
	}{
		{"response", func(m []byte) []byte { return m }, true},
		{"query", func(m []byte) []byte { m[2] &^= 0x80; return m }, false},
		{"non-standard opcode", func(m []byte) []byte { m[2] |= 0x10; return m }, false},
		{"reserved bit set", func(m []byte) []byte { m[3] |= 0x40; return m }, false},
		{"no question", func(m []byte) []byte { m[5] = 0; return m }, false},
		{"shorter than a header", func(m []byte) []byte { return m[:dnsHeaderLen-1] }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pktInfo := dnsPacketInfo{}
			msg := tt.mutate(append([]byte(nil), response...))
			require.Equal(t, tt.ok, setResponseFromHeader(msg, &pktInfo))
			if tt.ok {
				assert.Equal(t, uint16(42), pktInfo.transactionID)
				assert.Equal(t, successfulResponse, pktInfo.pktType)
			}
		})
	}
}

func BenchmarkParseDNSPacket(b *testing.B) {
	t := testing.TB(b)
	split := splitEvery(withTCPLength(tcpDNSMessage(t, 1, true, 200)), 1448)
	bigUDP := udpDNSFrame(t, tcpDNSMessage(t, 1, true, 400))
	frames := []struct {
		name  string
		frame []byte
	}{
		{"udp response", udpDNSFrame(t, tcpDNSMessage(t, 1, true, 5))},
		{"tcp self-contained response", tcpDNSFrame(t, tcpDNSServerIP, tcpDNSClientIP, 53, tcpDNSClientPort, 1, 1, withTCPLength(tcpDNSMessage(t, 1, true, 5)))},
		{"tcp split response first segment", tcpDNSFrame(t, tcpDNSServerIP, tcpDNSClientIP, 53, tcpDNSClientPort, 1, 1, split[0])},
		{"tcp split response continuation segment", tcpDNSFrame(t, tcpDNSServerIP, tcpDNSClientIP, 53, tcpDNSClientPort, 1, 1, split[1])},
		{"udp response truncated by snaplen", bigUDP[:4096]},
	}
	cfg := config.New()
	cfg.CollectDNSStats = true
	cfg.CollectDNSDomains = true
	for _, f := range frames {
		b.Run(f.name, func(b *testing.B) {
			parser := newDNSParser(layers.LayerTypeEthernet, cfg)
			tr := &translation{ips: make(map[util.Address]time.Time)}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pktInfo := dnsPacketInfo{}
				_, _ = parser.ParseInto(f.frame, tr, &pktInfo)
			}
		})
	}
}

// udpDNSFrame builds a server-to-client UDP frame carrying msg.
func udpDNSFrame(t testing.TB, msg []byte) []byte {
	return udpDNSFrameFrom(t, tcpDNSServerIP, tcpDNSClientIP, 53, tcpDNSClientPort, msg)
}

func udpDNSFrameFrom(t testing.TB, src, dst net.IP, sport, dport uint16, msg []byte) []byte {
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 1},
		DstMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 2},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: src, DstIP: dst}
	udp := &layers.UDP{SrcPort: layers.UDPPort(sport), DstPort: layers.UDPPort(dport)}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(msg)))
	return append([]byte(nil), buf.Bytes()...)
}

// tcpDNSMessage builds an A query (response=false) or a NOERROR response with numAnswers A records.
func tcpDNSMessage(t testing.TB, id uint16, response bool, numAnswers int) []byte {
	return tcpDNSMessageOfType(t, id, response, numAnswers, layers.DNSTypeA)
}

// tcpDNSMessageOfType is tcpDNSMessage with the given question type.
func tcpDNSMessageOfType(t testing.TB, id uint16, response bool, numAnswers int, qtype layers.DNSType) []byte {
	dns := &layers.DNS{
		ID:           id,
		QR:           response,
		RD:           true,
		RA:           response,
		ResponseCode: layers.DNSResponseCodeNoErr,
		Questions:    []layers.DNSQuestion{{Name: []byte(tcpDNSDomain), Type: qtype, Class: layers.DNSClassIN}},
	}
	for i := 0; i < numAnswers; i++ {
		dns.Answers = append(dns.Answers, layers.DNSResourceRecord{
			Name:  []byte(tcpDNSDomain),
			Type:  layers.DNSTypeA,
			Class: layers.DNSClassIN,
			TTL:   30,
			IP:    net.IP{10, 1, byte(i >> 8), byte(i)},
		})
	}
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, dns.SerializeTo(buf, gopacket.SerializeOptions{FixLengths: true}))
	return append([]byte(nil), buf.Bytes()...)
}

// withTCPLength prepends the 2-byte length field DNS uses over TCP.
func withTCPLength(msg []byte) []byte {
	out := make([]byte, 2, 2+len(msg))
	binary.BigEndian.PutUint16(out, uint16(len(msg)))
	return append(out, msg...)
}

func splitEvery(b []byte, n int) [][]byte {
	var out [][]byte
	for len(b) > n {
		out = append(out, b[:n])
		b = b[n:]
	}
	return append(out, b)
}

func tcpDNSFrame(t testing.TB, src, dst net.IP, sport, dport uint16, seq, ack uint32, payload []byte) []byte {
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 1},
		DstMAC:       net.HardwareAddr{0, 0, 0, 0, 0, 2},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(sport), DstPort: layers.TCPPort(dport), Seq: seq, Ack: ack, ACK: true, PSH: true, Window: 65535}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	require.NoError(t, gopacket.SerializeLayers(buf, opts, eth, ip, tcp, gopacket.Payload(payload)))
	return append([]byte(nil), buf.Bytes()...)
}
