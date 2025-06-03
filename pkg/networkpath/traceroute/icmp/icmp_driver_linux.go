// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package icmp

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/google/gopacket"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type icmpDriver struct {
	sink packets.Sink

	source packets.Source
	buffer []byte
	parser *packets.FrameParser

	sendTimes []time.Time
	localAddr netip.Addr
	params    Params
	echoID    uint16
	isIPV6    bool
	mu        *sync.Mutex
}

func newICMPDriver(params Params, localAddr netip.Addr) (*icmpDriver, error) {
	sink, err := packets.NewSinkUnix(params.Target)
	if err != nil {
		return nil, fmt.Errorf("newicmpDriver failed to make SinkUnix: %w", err)
	}

	source, err := packets.NewAFPacketSource()
	if err != nil {
		sink.Close()
		return nil, fmt.Errorf("newicmpDriver failed to make ICMP raw conn: %w", err)
	}

	retval := &icmpDriver{
		sink:      sink,
		source:    source,
		buffer:    make([]byte, 1024),
		parser:    packets.NewFrameParser(),
		sendTimes: make([]time.Time, params.ParallelParams.MaxTTL+1),
		localAddr: localAddr,
		params:    params,
		echoID:    uint16(os.Getpid() & 0xffff),
		isIPV6:    localAddr.Is6(),
		mu:        &sync.Mutex{},
	}
	return retval, nil
}

func (s *icmpDriver) Close() {
	s.source.Close()
	s.sink.Close()
}

func (s *icmpDriver) GetDriverInfo() common.TracerouteDriverInfo {
	return common.TracerouteDriverInfo{
		SupportsParallel: true,
	}
}

func (s *icmpDriver) SendProbe(ttl uint8) error {
	if ttl < s.params.ParallelParams.MinTTL || ttl > s.params.ParallelParams.MaxTTL {
		return fmt.Errorf("icmpDriver asked to send invalid TTL %d", ttl)
	}
	// store the send time for the RTT later when we receive the response
	if !s.sendTimes[ttl].IsZero() {
		return fmt.Errorf("icmpDriver asked to send probe for TTL %d but it was already sent", ttl)
	}
	s.sendTimes[ttl] = time.Now()

	gen := icmpPacketGen{
		ipPair: packets.IPPair{
			DstAddr: s.params.Target,
			SrcAddr: s.localAddr,
		},
	}
	packet, err := gen.generate(ttl, s.echoID, s.isIPV6)
	if err != nil {
		return fmt.Errorf("icmpDriver failed to generate packet: %w", err)
	}
	log.TraceFunc(func() string {
		return fmt.Sprintf("sending packet: %s\n", hex.EncodeToString(packet))
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	//if err := s.sink.Control(func(fd uintptr) error {
	//	if s.isIPV6 {
	//		// Set IPv6 Hop Limit
	//		return unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_UNICAST_HOPS, int(ttl))
	//	} else {
	//		// Set IPv4 TTL
	//		return unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_TTL, int(ttl))
	//	}
	//}); err != nil {
	//	return fmt.Errorf("icmpDriver failed to WriteToIP: %w", err)
	//}
	err = s.sink.WriteTo(packet, s.params.Target)
	if err != nil {
		return fmt.Errorf("icmpDriver failed to WriteToIP: %w", err)
	}
	return nil
}
func (s *icmpDriver) ReceiveProbe(timeout time.Duration) (*common.ProbeResponse, error) {
	err := s.source.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return nil, fmt.Errorf("icmpDriver failed to SetReadDeadline: %w", err)
	}

	err = packets.ReadAndParse(s.source, s.buffer, s.parser)
	if err != nil {
		return nil, err
	}

	return s.handleProbeLayers(s.parser)
}

func (s *icmpDriver) getRTTFromRelSeq(relSeq uint8) (time.Duration, error) {
	if relSeq < s.params.ParallelParams.MinTTL || relSeq > s.params.ParallelParams.MaxTTL {
		return 0, fmt.Errorf("getRTTFromRelSeq: invalid relative sequence number %d", relSeq)
	}
	if s.sendTimes[relSeq].IsZero() {
		return 0, fmt.Errorf("getRTTFromRelSeq: no probe sent for relative sequence number %d", relSeq)
	}
	return time.Since(s.sendTimes[relSeq]), nil
}

var errPacketDidNotMatchTraceroute = &common.ReceiveProbeNoPktError{Err: fmt.Errorf("packet did not match the traceroute")}

func (s *icmpDriver) handleProbeLayers(parser *packets.FrameParser) (*common.ProbeResponse, error) {
	ipPair, err := parser.GetIPPair()
	if err != nil {
		return nil, fmt.Errorf("icmpDriver failed to get IP pair: %w", err)
	}
	switch parser.GetTransportLayer() {
	case layers.LayerTypeICMPv4:
		t := parser.ICMP4.TypeCode.Type()
		switch t {
		case layers.ICMPv4TypeTimeExceeded:
			icmpInfo, err := parser.GetICMPInfo()
			if err != nil {
				return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get ICMP info: %w", err)}
			}
			msg, err := icmp.ParseMessage(ipv4.ICMPTypeEcho.Protocol(), icmpInfo.Payload)
			if err != nil {
				return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get echo request: %w", err)}
			}
			echo, ok := msg.Body.(*icmp.Echo)
			if !ok {
				return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get echo request: %w", err)}
			}
			if uint16(echo.ID) != s.echoID {
				return nil, &common.BadPacketError{Err: fmt.Errorf("mismatched echo ID")}
			}
			rtt, err := s.getRTTFromRelSeq(uint8(echo.Seq))
			if err != nil {
				return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get RTT: %w", err)}
			}
			return &common.ProbeResponse{
				TTL:    uint8(echo.Seq),
				IP:     ipPair.SrcAddr,
				RTT:    rtt,
				IsDest: false,
			}, nil
		case layers.ICMPv4TypeEchoReply:
			if parser.ICMP4.Id != s.echoID {
				return nil, &common.BadPacketError{Err: fmt.Errorf("mismatched echo ID")}
			}
			rtt, err := s.getRTTFromRelSeq(uint8(parser.ICMP4.Seq))
			if err != nil {
				return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get RTT: %w", err)}
			}
			return &common.ProbeResponse{
				TTL:    uint8(parser.ICMP4.Seq),
				IP:     ipPair.SrcAddr,
				RTT:    rtt,
				IsDest: true,
			}, nil

		default:
			return nil, errPacketDidNotMatchTraceroute
		}
	case layers.LayerTypeICMPv6:
		t := parser.ICMP6.TypeCode.Type()
		switch t {
		case layers.ICMPv6TypeTimeExceeded:
			//icmpInfo, err := parser.GetICMPInfo()
			//if err != nil {
			//	return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get ICMP info: %w", err)}
			//}
			//fmt.Println("icmpDriver got a TimeExceeded", icmpInfo)
			payload := parser.ICMP6.Payload
			var embedded []byte
			switch {
			case len(payload) > 4 && payload[4]>>4 == 6:
				// Strip 4-byte prefix and parse IPv6 from offset 4
				embedded = payload[4:]

			case len(payload) > 8 && payload[0] == 0x03 && payload[1] == 0x00 && payload[8]>>4 == 6:
				// ICMPv6 Time Exceeded + embedded IPv6
				embedded = payload[8:]

			case len(payload) > 0 && payload[0]>>4 == 6:
				// Raw IPv6 packet (no prefix, no ICMPv6 wrapper)
				embedded = payload

			default:
				return nil, fmt.Errorf("Embedded packet is not a valid IPv6 header")
			}
			var (
				ip6  layers.IPv6
				icmp layers.ICMPv6
				echo layers.ICMPv6Echo
			)

			parser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv6, &ip6, &icmp, &echo)
			decoded := []gopacket.LayerType{}

			err = parser.DecodeLayers(embedded, &decoded)
			if err != nil {
				return nil, fmt.Errorf("Embedded decode error:", err)
			}
			if echo.Identifier != s.echoID {
				return nil, &common.BadPacketError{Err: fmt.Errorf("mismatched echo ID")}
			}
			rtt, err := s.getRTTFromRelSeq(uint8(echo.SeqNumber))
			if err != nil {
				return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get RTT: %w", err)}
			}
			return &common.ProbeResponse{
				TTL:    uint8(echo.SeqNumber),
				IP:     ipPair.SrcAddr,
				RTT:    rtt,
				IsDest: false,
			}, nil
		case layers.ICMPv6TypeEchoReply:
			payload := parser.ICMP6.Payload
			if len(payload) >= 4 {
				id := binary.BigEndian.Uint16(payload[0:2])
				seq := binary.BigEndian.Uint16(payload[2:4])
				if id != s.echoID {
					return nil, &common.BadPacketError{Err: fmt.Errorf("mismatched echo ID")}
				}
				rtt, err := s.getRTTFromRelSeq(uint8(seq))
				if err != nil {
					return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get RTT: %w", err)}
				}
				return &common.ProbeResponse{
					TTL:    uint8(seq),
					IP:     ipPair.SrcAddr,
					RTT:    rtt,
					IsDest: false,
				}, nil
			} else {
				return nil, errPacketDidNotMatchTraceroute
			}
			return nil, errPacketDidNotMatchTraceroute
		default:
			return nil, errPacketDidNotMatchTraceroute
		}

	default:
		return nil, errPacketDidNotMatchTraceroute
	}
}

var _ common.TracerouteDriver = &icmpDriver{}
