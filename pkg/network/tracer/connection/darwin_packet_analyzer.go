// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"sort"
	"sync"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	tlstags "github.com/DataDog/datadog-agent/pkg/network/protocols/tls"
)

const (
	darwinPrefixLimit   = 8192
	darwinSegmentLimit  = 32
	tlsHandshakeRecord  = 22
	tlsClientHello      = 1
	tlsServerHello      = 2
	tlsSupportedVersion = 43
)

type darwinPacketAnalysis struct {
	direction      network.ConnectionDirection
	retransmits    uint32
	failureErrno   uint16
	failure        bool
	protocolStack  protocols.Stack
	tlsTags        tlstags.Tags
	prefixTruncate bool
}

type darwinPacketFlowState struct {
	direction network.ConnectionDirection

	hasMaxSeq bool
	maxSeq    uint32

	sawSyn           bool
	sawSynAck        bool
	established      bool
	failureLatched   bool
	retransmits      uint32
	captureTruncated bool

	outgoingPrefix darwinPrefixAssembler
	incomingPrefix darwinPrefixAssembler
	protocolStack  protocols.Stack
	tlsTags        tlstags.Tags
}

// darwinPacketAnalyzer derives only packet-owned fields. It intentionally has
// no access to authoritative NStat counters or lifecycle maps.
type darwinPacketAnalyzer struct {
	mu       sync.Mutex
	maxFlows int
	flows    map[uint64]*darwinPacketFlowState
}

func newDarwinPacketAnalyzer(maxFlows int) *darwinPacketAnalyzer {
	return &darwinPacketAnalyzer{
		maxFlows: maxFlows,
		flows:    make(map[uint64]*darwinPacketFlowState, maxFlows),
	}
}

func (a *darwinPacketAnalyzer) process(cookie uint64, outgoing bool, captureTruncated bool, tcp *layers.TCP) darwinPacketAnalysis {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.flows[cookie]
	if state == nil {
		if len(a.flows) >= a.maxFlows {
			return darwinPacketAnalysis{}
		}
		state = &darwinPacketFlowState{}
		a.flows[cookie] = state
	}
	state.captureTruncated = state.captureTruncated || captureTruncated

	if tcp.SYN && !tcp.ACK {
		state.sawSyn = true
		if outgoing {
			state.direction = network.OUTGOING
		} else {
			state.direction = network.INCOMING
		}
	}
	if tcp.SYN && tcp.ACK {
		state.sawSynAck = true
	}
	if tcp.ACK && (state.sawSynAck || len(tcp.Payload) > 0) {
		state.established = true
	}

	if outgoing {
		state.observeOutgoingSequence(tcp)
		state.outgoingPrefix.add(tcp.Seq, tcp.Payload)
	} else {
		state.incomingPrefix.add(tcp.Seq, tcp.Payload)
	}

	var failureErrno uint16
	var failure bool
	if tcp.RST && !state.failureLatched {
		state.failureLatched = true
		failure = true
		if state.sawSyn && !state.established {
			failureErrno = network.TCPFailureErrnoConnRefused
		} else {
			failureErrno = network.TCPFailureErrnoConnReset
		}
	}

	state.classify()
	return darwinPacketAnalysis{
		direction:      state.direction,
		retransmits:    state.retransmits,
		failureErrno:   failureErrno,
		failure:        failure,
		protocolStack:  state.protocolStack,
		tlsTags:        state.tlsTags,
		prefixTruncate: state.captureTruncated || state.outgoingPrefix.truncated || state.incomingPrefix.truncated,
	}
}

func (a *darwinPacketAnalyzer) remove(cookie uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.flows, cookie)
}

func (s *darwinPacketFlowState) observeOutgoingSequence(tcp *layers.TCP) {
	next := tcp.Seq + uint32(len(tcp.Payload))
	if tcp.SYN || tcp.FIN {
		next++
	}
	if next == tcp.Seq {
		return
	}
	if !s.hasMaxSeq || darwinSeqBefore(s.maxSeq, next) {
		s.hasMaxSeq = true
		s.maxSeq = next
		return
	}
	s.retransmits++
}

func (s *darwinPacketFlowState) classify() {
	for _, prefix := range [][]byte{s.outgoingPrefix.bytes(), s.incomingPrefix.bytes()} {
		stack, tags := classifyDarwinPrefix(prefix)
		s.protocolStack.MergeWith(stack)
		s.tlsTags.MergeWith(tags)
	}
}

func darwinSeqBefore(left, right uint32) bool {
	return int32(left-right) < 0
}

type darwinPrefixSegment struct {
	seq  uint32
	data []byte
}

type darwinPrefixAssembler struct {
	segments  []darwinPrefixSegment
	truncated bool
}

func (a *darwinPrefixAssembler) add(seq uint32, payload []byte) {
	if len(payload) == 0 || a.truncated {
		return
	}
	for index := range a.segments {
		if a.segments[index].seq == seq {
			if len(payload) > len(a.segments[index].data) {
				a.segments[index].data = append(a.segments[index].data[:0], payload...)
			}
			return
		}
	}
	if len(a.segments) >= darwinSegmentLimit {
		a.truncated = true
		return
	}
	total := len(payload)
	for _, segment := range a.segments {
		total += len(segment.data)
	}
	if total > darwinPrefixLimit {
		remaining := darwinPrefixLimit - (total - len(payload))
		if remaining <= 0 {
			a.truncated = true
			return
		}
		payload = payload[:remaining]
		a.truncated = true
	}
	a.segments = append(a.segments, darwinPrefixSegment{
		seq:  seq,
		data: append([]byte(nil), payload...),
	})
}

func (a *darwinPrefixAssembler) bytes() []byte {
	if len(a.segments) == 0 {
		return nil
	}
	segments := append([]darwinPrefixSegment(nil), a.segments...)
	sort.Slice(segments, func(i, j int) bool {
		return darwinSeqBefore(segments[i].seq, segments[j].seq)
	})
	result := append([]byte(nil), segments[0].data...)
	next := segments[0].seq + uint32(len(segments[0].data))
	for _, segment := range segments[1:] {
		if darwinSeqBefore(next, segment.seq) {
			break
		}
		overlap := uint32(0)
		if darwinSeqBefore(segment.seq, next) {
			overlap = next - segment.seq
		}
		if overlap < uint32(len(segment.data)) {
			result = append(result, segment.data[overlap:]...)
			next = segment.seq + uint32(len(segment.data))
		}
	}
	return result
}

func classifyDarwinPrefix(prefix []byte) (protocols.Stack, tlstags.Tags) {
	var stack protocols.Stack
	var tags tlstags.Tags
	if len(prefix) == 0 {
		return stack, tags
	}

	if parsed, ok := parseDarwinTLSHandshake(prefix); ok {
		stack.Encryption = protocols.TLS
		tags.MergeWith(parsed)
	}

	upper := bytes.ToUpper(prefix)
	if bytes.HasPrefix(prefix, []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")) ||
		(bytes.HasPrefix(upper, []byte("HTTP/1.1 101")) &&
			bytes.Contains(upper, []byte("UPGRADE: H2C"))) {
		stack.Application = protocols.HTTP2
	} else if isDarwinHTTPPrefix(upper) {
		stack.Application = protocols.HTTP
	}
	return stack, tags
}

func isDarwinHTTPPrefix(prefix []byte) bool {
	if bytes.HasPrefix(prefix, []byte("HTTP/1.")) {
		return true
	}
	for _, method := range []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH ", "CONNECT ", "TRACE "} {
		if bytes.HasPrefix(prefix, []byte(method)) {
			return true
		}
	}
	return false
}

func parseDarwinTLSHandshake(data []byte) (tlstags.Tags, bool) {
	if len(data) < 9 || data[0] != tlsHandshakeRecord {
		return tlstags.Tags{}, false
	}
	recordLength := int(binary.BigEndian.Uint16(data[3:5]))
	if recordLength < 4 || recordLength+5 > len(data) {
		return tlstags.Tags{}, false
	}
	handshakeType := data[5]
	handshakeLength := int(data[6])<<16 | int(data[7])<<8 | int(data[8])
	if handshakeLength+4 > recordLength || handshakeLength+9 > len(data) {
		return tlstags.Tags{}, false
	}
	body := data[9 : 9+handshakeLength]
	switch handshakeType {
	case tlsClientHello:
		tags, ok := parseDarwinClientHello(body)
		return tags, ok
	case tlsServerHello:
		tags, ok := parseDarwinServerHello(body)
		return tags, ok
	default:
		return tlstags.Tags{}, false
	}
}

func parseDarwinClientHello(body []byte) (tlstags.Tags, bool) {
	if len(body) < 35 {
		return tlstags.Tags{}, false
	}
	legacyVersion := binary.BigEndian.Uint16(body[:2])
	offset := 34
	sessionLength := int(body[offset])
	offset++
	if offset+sessionLength+2 > len(body) {
		return tlstags.Tags{}, false
	}
	offset += sessionLength
	cipherLength := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2
	if cipherLength%2 != 0 || offset+cipherLength+1 > len(body) {
		return tlstags.Tags{}, false
	}
	offset += cipherLength
	compressionLength := int(body[offset])
	offset++
	if offset+compressionLength > len(body) {
		return tlstags.Tags{}, false
	}
	offset += compressionLength

	tags := tlstags.Tags{OfferedVersions: offeredDarwinTLSVersion(legacyVersion)}
	extensions, ok := darwinTLSExtensions(body, offset)
	if !ok {
		return tags, true
	}
	if supported := extensions[tlsSupportedVersion]; len(supported) >= 1 {
		length := int(supported[0])
		if length <= len(supported)-1 && length%2 == 0 {
			for pos := 1; pos < 1+length; pos += 2 {
				tags.OfferedVersions |= offeredDarwinTLSVersion(binary.BigEndian.Uint16(supported[pos : pos+2]))
			}
		}
	}
	return tags, true
}

func parseDarwinServerHello(body []byte) (tlstags.Tags, bool) {
	if len(body) < 38 {
		return tlstags.Tags{}, false
	}
	chosenVersion := binary.BigEndian.Uint16(body[:2])
	offset := 34
	sessionLength := int(body[offset])
	offset++
	if offset+sessionLength+3 > len(body) {
		return tlstags.Tags{}, false
	}
	offset += sessionLength
	cipherSuite := binary.BigEndian.Uint16(body[offset : offset+2])
	offset += 3 // cipher suite plus compression method
	if extensions, ok := darwinTLSExtensions(body, offset); ok {
		if supported := extensions[tlsSupportedVersion]; len(supported) == 2 {
			chosenVersion = binary.BigEndian.Uint16(supported)
		}
	}
	return tlstags.Tags{
		ChosenVersion: chosenVersion,
		CipherSuite:   cipherSuite,
	}, true
}

func darwinTLSExtensions(body []byte, offset int) (map[uint16][]byte, bool) {
	if offset == len(body) {
		return nil, false
	}
	if offset+2 > len(body) {
		return nil, false
	}
	length := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2
	if offset+length > len(body) {
		return nil, false
	}
	extensions := make(map[uint16][]byte)
	for end := offset + length; offset+4 <= end; {
		typ := binary.BigEndian.Uint16(body[offset : offset+2])
		extensionLength := int(binary.BigEndian.Uint16(body[offset+2 : offset+4]))
		offset += 4
		if offset+extensionLength > end {
			return nil, false
		}
		extensions[typ] = body[offset : offset+extensionLength]
		offset += extensionLength
	}
	return extensions, true
}

func offeredDarwinTLSVersion(version uint16) uint8 {
	switch version {
	case tls.VersionTLS10:
		return tlstags.OfferedTLSVersion10
	case tls.VersionTLS11:
		return tlstags.OfferedTLSVersion11
	case tls.VersionTLS12:
		return tlstags.OfferedTLSVersion12
	case tls.VersionTLS13:
		return tlstags.OfferedTLSVersion13
	default:
		return 0
	}
}
