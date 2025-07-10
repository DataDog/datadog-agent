// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package icmp

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

//nolint:unused // This is used, but not on all platforms yet
var curEchoID atomic.Uint32

//nolint:unused // This is used, but not on all platforms yet
func nextEchoID() uint16 {
	next := curEchoID.Add(1)
	return uint16(next)
}

type icmpDriver struct {
	sink packets.Sink

	source packets.Source
	buffer []byte
	parser *packets.FrameParser
	// mu guards against concurrent access to sentProbes
	mu         sync.Mutex
	sentProbes map[uint8]time.Time
	localAddr  netip.Addr
	params     Params
	echoID     uint16
	isIPV6     bool
}

//nolint:unused // This is used, but not on all platforms yet
func newICMPDriver(params Params, localAddr netip.Addr, sink packets.Sink, source packets.Source) (*icmpDriver, error) {
	retval := &icmpDriver{
		sink:       sink,
		source:     source,
		buffer:     make([]byte, 1024),
		parser:     packets.NewFrameParser(),
		sentProbes: make(map[uint8]time.Time),
		localAddr:  localAddr,
		params:     params,
		echoID:     nextEchoID(),
		isIPV6:     localAddr.Is6(),
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
	err := s.storeProbe(ttl)
	if err != nil {
		return err
	}

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
	err = s.sink.WriteTo(packet, netip.AddrPortFrom(s.params.Target, 80))
	if err != nil {
		return fmt.Errorf("icmpDriver failed to WriteToIP: %w", err)
	}
	return nil
}

func (s *icmpDriver) storeProbe(ttl uint8) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// store the send time for the RTT later when we receive the response
	if !s.sentProbes[ttl].IsZero() {
		return fmt.Errorf("icmpDriver asked to send probe for TTL %d but it was already sent", ttl)
	}
	// refuse to store it if we somehow would overwrite
	if _, ok := s.sentProbes[ttl]; ok {
		return fmt.Errorf("icmpDriver Sendprobe tried to sent the same probe twice for ttl=%d", ttl)
	}
	s.sentProbes[ttl] = time.Now()
	return nil
}

func (s *icmpDriver) findMatchingProbe(ttl uint8) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, ok := s.sentProbes[ttl]
	return data, ok
}

func (s *icmpDriver) ReceiveProbe(timeout time.Duration) (*common.ProbeResponse, error) {
	err := s.source.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return nil, fmt.Errorf("icmpDriver failed to SetReadDeadline: %w", err)
	}
	if err := packets.ReadAndParse(s.source, s.buffer, s.parser); err != nil {
		return nil, err
	}
	return s.handleProbeLayers(s.parser)
}

func (s *icmpDriver) getRTTFromRelSeq(relSeq uint8) (time.Duration, error) {
	if relSeq < s.params.ParallelParams.MinTTL || relSeq > s.params.ParallelParams.MaxTTL {
		return 0, fmt.Errorf("getRTTFromRelSeq: invalid relative sequence number %d", relSeq)
	}
	t, ok := s.findMatchingProbe(relSeq)
	if !ok || t.IsZero() {
		return 0, fmt.Errorf("getRTTFromRelSeq: no probe sent for relative sequence number %d", relSeq)
	}
	return time.Since(t), nil
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
			local := s.localAddr
			target := s.params.Target
			if icmpInfo.ICMPPair.DstAddr.Compare(target) != 0 {
				log.Tracef("icmpDriver ignored ICMP packet which had another destination: expected=%s, actual=%s", target, icmpInfo.ICMPPair.DstAddr)
				return nil, common.ErrPacketDidNotMatchTraceroute
			}
			if icmpInfo.ICMPPair.SrcAddr.Compare(local) != 0 {
				log.Tracef("icmpDriver ignored packet which had another source: expected=%s, actual=%s", local, icmpInfo.ICMPPair.SrcAddr)
				return nil, common.ErrPacketDidNotMatchTraceroute
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
			icmpInfo, err := parser.GetICMPInfo()
			if err != nil {
				return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get ICMP info: %w", err)}
			}
			local := s.localAddr
			target := s.params.Target
			if icmpInfo.ICMPPair.DstAddr.Compare(target) != 0 {
				log.Tracef("icmpDriver ignored ICMP packet which had another destination: expected=%s, actual=%s", target, icmpInfo.ICMPPair.DstAddr)
				return nil, common.ErrPacketDidNotMatchTraceroute
			}

			if icmpInfo.ICMPPair.SrcAddr.Compare(local) != 0 {
				log.Tracef("icmpDriver ignored packet which had another source: expected=%s, actual=%s", local, icmpInfo.ICMPPair.SrcAddr)
				return nil, common.ErrPacketDidNotMatchTraceroute
			}

			echo, err := extractEchoRequest(icmpInfo)
			if err != nil {
				return nil, &common.BadPacketError{Err: fmt.Errorf("icmpDriver failed to get echo request: %w", err)}
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
			if len(payload) < 4 {
				return nil, errPacketDidNotMatchTraceroute
			}
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
				IsDest: true,
			}, nil
		default:
			return nil, errPacketDidNotMatchTraceroute
		}
	default:
		return nil, errPacketDidNotMatchTraceroute
	}
}

func extractEchoRequest(icmpInfo packets.ICMPInfo) (*layers.ICMPv6Echo, error) {
	var icmp6 layers.ICMPv6
	var echo layers.ICMPv6Echo
	dparser := gopacket.NewDecodingLayerParser(
		layers.LayerTypeICMPv6,
		&icmp6, &echo,
	)
	decoded := []gopacket.LayerType{}
	err := dparser.DecodeLayers(icmpInfo.Payload, &decoded)
	if err != nil {
		return nil, fmt.Errorf("icmpDriver failed to decode ICMPv6 info payload: %w", err)
	}
	var unsupportedErr gopacket.UnsupportedLayerType
	if errors.As(err, &unsupportedErr) {
		// there are extra layers beyond TLS, ignore those too
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("icmpDriver failed to get echo request: %w", err)
	}
	return &echo, nil
}

var _ common.TracerouteDriver = &icmpDriver{}
