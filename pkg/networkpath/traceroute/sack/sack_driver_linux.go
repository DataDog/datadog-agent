// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sack

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"os"
	"time"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type sackDriver struct {
	sink packets.Sink

	source packets.Source
	buffer []byte
	parser *packets.FrameParser

	sendTimes []time.Time
	localAddr netip.Addr
	localPort uint16
	params    Params
	state     *sackTCPState
}

func newSackDriver(params Params, localAddr netip.Addr) (*sackDriver, error) {
	sink, err := packets.NewSinkUnix(params.Target.Addr())
	if err != nil {
		return nil, fmt.Errorf("newSackDriver failed to make SinkUnix: %w", err)
	}

	source, err := packets.NewAFPacketSource()
	if err != nil {
		sink.Close()
		return nil, fmt.Errorf("newSackDriver failed to make AFPacketSource: %w", err)
	}

	retval := &sackDriver{
		sink:      sink,
		source:    source,
		buffer:    make([]byte, 1024),
		parser:    packets.NewFrameParser(),
		sendTimes: make([]time.Time, params.ParallelParams.MaxTTL+1),
		localAddr: localAddr,
		localPort: 0, // to be set by ReadHandshake()
		params:    params,
	}
	return retval, nil
}

func (s *sackDriver) Close() {
	s.source.Close()
	s.sink.Close()
}

func (s *sackDriver) GetDriverInfo() common.TracerouteDriverInfo {
	return common.TracerouteDriverInfo{
		SupportsParallel: true,
	}
}

func (s *sackDriver) SendProbe(ttl uint8) error {
	if !s.IsHandshakeFinished() {
		return fmt.Errorf("sackDriver hasn't finished ReadHandshake()")
	}
	if ttl < s.params.ParallelParams.MinTTL || ttl > s.params.ParallelParams.MaxTTL {
		return fmt.Errorf("sackDriver asked to send invalid TTL %d", ttl)
	}
	// store the send time for the RTT later when we receive the response
	if !s.sendTimes[ttl].IsZero() {
		return fmt.Errorf("sackDriver asked to send probe for TTL %d but it was already sent", ttl)
	}
	s.sendTimes[ttl] = time.Now()

	gen := sackPacketGen{
		ipPair: s.ExpectedIPPair().Flipped(),
		sPort:  s.localPort,
		dPort:  s.params.Target.Port(),
		state:  *s.state,
	}
	// TODO ipv6
	packet, err := gen.generateV4(ttl)
	if err != nil {
		return fmt.Errorf("sackDriver failed to generate packet: %w", err)
	}

	log.TraceFunc(func() string {
		return fmt.Sprintf("sending packet: %s\n", hex.EncodeToString(packet))
	})
	err = s.sink.WriteTo(packet, s.params.Target)
	if err != nil {
		return fmt.Errorf("sackDriver failed to WriteToIP: %w", err)
	}
	return nil
}
func (s *sackDriver) ReceiveProbe(timeout time.Duration) (*common.ProbeResponse, error) {
	if !s.IsHandshakeFinished() {
		return nil, fmt.Errorf("sackDriver hasn't finished ReadHandshake()")
	}

	err := s.source.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return nil, fmt.Errorf("sackDriver failed to SetReadDeadline: %w", err)
	}
	err = packets.ReadAndParse(s.source, s.buffer, s.parser)
	if err != nil {
		return nil, err
	}

	return s.handleProbeLayers(s.parser)
}

func (s *sackDriver) ExpectedIPPair() packets.IPPair {
	// from the target to us
	return packets.IPPair{
		SrcAddr: s.params.Target.Addr(),
		DstAddr: s.localAddr,
	}
}

// IsHandshakeFinished returns whether the sackDriver is ready to perform a traceroute.
// After ReadHandshake() succeeds, this returns true.
func (s *sackDriver) IsHandshakeFinished() bool {
	return s.state != nil
}

// getMinSack returns the minimum SACK value from the SACK options.
// we use this to find the earliest TTL that actually arrived
func getMinSack(localInitSeq uint32, opts []layers.TCPOption) (uint32, error) {
	minSack := uint32(math.MaxUint32)
	foundSack := false
	for _, opt := range opts {
		if opt.OptionType != layers.TCPOptionKindSACK {
			continue
		}

		for data := opt.OptionData; len(data) >= 8; data = data[8:] {
			foundSack = true
			leftEdge := binary.BigEndian.Uint32(data[:4])
			relativeLeftEdge := leftEdge - localInitSeq
			if relativeLeftEdge < minSack {
				minSack = relativeLeftEdge
			}
		}
	}
	if !foundSack {
		return 0, fmt.Errorf("sackDriver found no SACK options")
	}
	return minSack, nil
}

func (s *sackDriver) getRTTFromRelSeq(relSeq uint32) (time.Duration, error) {
	if relSeq < uint32(s.params.ParallelParams.MinTTL) || relSeq > uint32(s.params.ParallelParams.MaxTTL) {
		return 0, fmt.Errorf("getRTTFromRelSeq: invalid relative sequence number %d", relSeq)
	}
	if s.sendTimes[relSeq].IsZero() {
		return 0, fmt.Errorf("getRTTFromRelSeq: no probe sent for relative sequence number %d", relSeq)
	}
	return time.Since(s.sendTimes[relSeq]), nil
}

var errPacketDidNotMatchTraceroute = &common.ReceiveProbeNoPktError{Err: fmt.Errorf("packet did not match the traceroute")}

func (s *sackDriver) handleProbeLayers(parser *packets.FrameParser) (*common.ProbeResponse, error) {
	ipPair, err := parser.GetIPPair()
	if err != nil {
		return nil, fmt.Errorf("sackDriver failed to get IP pair: %w", err)
	}

	switch parser.GetTransportLayer() {
	case layers.LayerTypeTCP:
		if ipPair != s.ExpectedIPPair() {
			return nil, errPacketDidNotMatchTraceroute
		}
		// make sure the ports match
		if s.params.Target.Port() != uint16(parser.TCP.SrcPort) ||
			s.localPort != uint16(parser.TCP.DstPort) {
			return nil, errPacketDidNotMatchTraceroute
		}
		// we only care about selective ACKs
		if parser.TCP.SYN || parser.TCP.FIN || parser.TCP.RST {
			return nil, errPacketDidNotMatchTraceroute
		}
		// get the first sequence number that was dupe ACKed
		relSeq, err := getMinSack(s.state.localInitSeq, parser.TCP.Options)
		if err != nil {
			// note: this is a NotSupportedError, not a BadPacketError because we want to fail the whole
			// traceroute if for some reason the endpoint returned SACK-permitted but isn't giving us SACK.
			// I've seen akamai CDN do this when serving static files for example.com.
			return nil, &NotSupportedError{
				Err: fmt.Errorf("endpoint returned SACK-permitted but found no SACK options: %w", err),
			}
		}
		rtt, err := s.getRTTFromRelSeq(relSeq)
		if err != nil {
			return nil, &common.BadPacketError{Err: fmt.Errorf("sackDriver failed to get RTT: %w", err)}
		}

		return &common.ProbeResponse{
			TTL:    uint8(relSeq),
			IP:     ipPair.SrcAddr,
			RTT:    rtt,
			IsDest: true,
		}, nil
	case layers.LayerTypeICMPv4:
		if !parser.IsTTLExceeded() {
			return nil, errPacketDidNotMatchTraceroute
		}

		icmpInfo, err := parser.GetICMPInfo()
		if err != nil {
			return nil, &common.BadPacketError{Err: fmt.Errorf("sackDriver failed to get ICMP info: %w", err)}
		}

		tcpInfo, err := packets.ParseTCPFirstBytes(icmpInfo.Payload)
		if err != nil {
			return nil, &common.BadPacketError{Err: fmt.Errorf("sackDriver failed to parse TCP info: %w", err)}
		}
		icmpDst := netip.AddrPortFrom(icmpInfo.ICMPPair.DstAddr, tcpInfo.DstPort)
		if icmpDst != s.params.Target {
			log.Tracef("icmp dst mismatch. expected: %s actual: %s", s.params.Target, icmpDst)
			return nil, errPacketDidNotMatchTraceroute
		}
		if !s.params.LoosenICMPSrc {
			icmpSrc := netip.AddrPortFrom(icmpInfo.IPPair.SrcAddr, tcpInfo.SrcPort)
			expectedSrc := netip.AddrPortFrom(s.localAddr, s.localPort)
			if icmpSrc != expectedSrc {
				log.Tracef("icmp src mismatch. expected: %s actual: %s", expectedSrc, icmpSrc)
				return nil, errPacketDidNotMatchTraceroute
			}
		}

		relSeq := tcpInfo.Seq - s.state.localInitSeq
		rtt, err := s.getRTTFromRelSeq(relSeq)
		if err != nil {
			return nil, &common.BadPacketError{Err: fmt.Errorf("sackDriver failed to get RTT: %w", err)}
		}
		return &common.ProbeResponse{
			TTL:    uint8(relSeq),
			IP:     ipPair.SrcAddr,
			RTT:    rtt,
			IsDest: false,
		}, nil
	default:
		return nil, errPacketDidNotMatchTraceroute
	}
}

var _ common.TracerouteDriver = &sackDriver{}

// FakeHandshake is sometimes used when debugging locally, to force the sackDriver to send packets
// even if SACK negotiation would fail
func (s *sackDriver) FakeHandshake() {
	s.localPort = 1234
	s.state = &sackTCPState{
		localInitSeq: 5678,
		localInitAck: 3333,
	}
}

// ReadHandshake polls for a synack from the target and populates the localInitSeq and localInitAck fields.
// it also checks that the target supports SACK.
func (s *sackDriver) ReadHandshake(localPort uint16) error {
	s.localPort = localPort
	err := s.source.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if err != nil {
		return fmt.Errorf("sackDriver failed to SetReadDeadline: %w", err)
	}
	for !s.IsHandshakeFinished() {
		// we should have already connected by now so it should be over quickly
		err = packets.ReadAndParse(s.source, s.buffer, s.parser)

		if errors.Is(err, os.ErrDeadlineExceeded) {
			return fmt.Errorf("sackDriver readHandshake timed out")
			// deadline exceeded is normally retryable, so this comes second in order
		} else if common.CheckProbeRetryable("ReadHandshake", err) {
			continue
		} else if err != nil {
			return fmt.Errorf("sackDriver failed to readAndParse: %w", err)
		}

		err = s.handleHandshake()
		if err != nil {
			return fmt.Errorf("sackDriver failed to handleHandshakeLayers: %w", err)
		}
	}
	return nil
}

func (s *sackDriver) handleHandshake() error {
	parser := s.parser
	ipPair, err := parser.GetIPPair()
	if err != nil {
		return fmt.Errorf("sackDriver failed to get IP pair: %w", err)
	}

	if parser.GetTransportLayer() != layers.LayerTypeTCP {
		return nil
	}

	if ipPair != s.ExpectedIPPair() {
		return nil
	}
	if s.params.Target.Port() != uint16(parser.TCP.SrcPort) ||
		s.localPort != uint16(parser.TCP.DstPort) {
		log.Debugf("bad ports, %d != %d, %d != %d", s.params.Target.Port(), uint16(parser.TCP.SrcPort), s.localPort, uint16(parser.TCP.DstPort))
		return nil
	}

	// must be the SYNACK response
	if !parser.TCP.SYN || !parser.TCP.ACK {
		return nil
	}
	// check if they support SACK otherwise we can't traceroute this way
	foundSackPermitted := false
	state := sackTCPState{}
	for _, opt := range parser.TCP.Options {
		log.Tracef("handleHandshake saw option %s", opt.OptionType)
		switch opt.OptionType {
		case layers.TCPOptionKindSACKPermitted:
			foundSackPermitted = true
		case layers.TCPOptionKindTimestamps:
			if len(opt.OptionData) < 8 {
				return fmt.Errorf("sackDriver found truncated timestamps option")
			}
			remoteTSValue := binary.BigEndian.Uint32(opt.OptionData[:4])
			remoteTSEcr := binary.BigEndian.Uint32(opt.OptionData[4:8])

			state.hasTS = true
			// simulate some time passing
			state.tsValue = remoteTSEcr + 50
			// send back their ts value otherwise the connection will be dropped
			state.tsEcr = remoteTSValue
		}
	}
	if !foundSackPermitted {
		return &NotSupportedError{
			Err: fmt.Errorf("SACK not supported by the target %s (missing SACK-permitted option)", s.params.Target),
		}
	}

	// set the localInitSeq and localInitAck based off the response
	state.localInitSeq = parser.TCP.Ack // this is NOT Ack - 1 because we need to leave a gap in the data
	state.localInitAck = parser.TCP.Seq + 1
	s.state = &state
	return nil
}
