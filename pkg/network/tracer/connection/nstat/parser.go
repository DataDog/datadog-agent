// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

// Package nstat decodes the private revision-9 Darwin Network Statistics ABI.
package nstat

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

const (
	// ABIRevision is the private NStat ABI revision supported by this package.
	ABIRevision = 9

	ProviderTCPKernel   uint32 = 2
	ProviderTCPUserland uint32 = 3
	ProviderUDPKernel   uint32 = 4
	ProviderUDPUserland uint32 = 5
)

const (
	messageSuccess    uint32 = 0
	messageError      uint32 = 1
	messageSourceAdd  uint32 = 10001
	messageSourceGone uint32 = 10002
	messageSourceDesc uint32 = 10003
	messageCounts     uint32 = 10004
	messageUpdate     uint32 = 10006
)

const (
	headerSize = 16

	addedSourceRef = 16
	addedProvider  = 24

	descriptionSourceRef = 16
	descriptionProvider  = 32
	descriptionData      = 40

	countsSourceRef = 16
	countsData      = 32

	updateSourceRef = 16
	updateCounts    = 32
	updateProvider  = 144
	updateData      = 152

	countsSize            = 112
	countsRXPackets       = 0
	countsRXBytes         = 8
	countsTXPackets       = 16
	countsTXBytes         = 24
	countsRXDuplicate     = 80
	countsRXOutOfOrder    = 84
	countsTXRetransmitted = 88
	countsConnectAttempts = 92
	countsConnectSuccess  = 96
	countsMinRTT          = 100
	countsAvgRTT          = 104
	countsVarRTT          = 108

	descriptionUniquePID          = 0
	descriptionEffectiveUniquePID = 8

	tcpInterfaceIndex = 72
	tcpState          = 76
	tcpPID            = 116
	tcpEffectivePID   = 120
	tcpLocal          = 124
	tcpRemote         = 152
	tcpMinimumSize    = 260

	udpLocal          = 56
	udpRemote         = 84
	udpInterfaceIndex = 112
	udpPID            = 128
	udpEffectivePID   = 196
	udpMinimumSize    = 200

	sockaddrStorageSize = 28
	addressFamilyIPv4   = 2
	addressFamilyIPv6   = 30
)

var (
	// ErrMalformed indicates that a datagram's length framing or known message
	// body is truncated or invalid.
	ErrMalformed = errors.New("malformed nstat datagram")
	// ErrABIMismatch indicates that a descriptor does not plausibly match the
	// private revision-9 layout. Callers must fail closed on this error.
	ErrABIMismatch = errors.New("nstat descriptor does not match revision-9 ABI")
)

// EventKind identifies a decoded NStat message.
type EventKind uint8

const (
	EventAdded EventKind = iota
	EventDescription
	EventCounts
	EventUpdate
	EventRemoved
	EventError
	EventSuccess
)

// Counts contains cumulative transport counters from an NStat source.
type Counts struct {
	RXPackets            uint64
	RXBytes              uint64
	TXPackets            uint64
	TXBytes              uint64
	RXDuplicateBytes     uint32
	RXOutOfOrderBytes    uint32
	TXRetransmittedBytes uint32
	ConnectAttempts      uint32
	ConnectSuccesses     uint32
	MinimumRTT           uint32
	AverageRTT           uint32
	RTTVariance          uint32
}

// Endpoint is one local or remote socket endpoint. Present is false for an
// intentionally unspecified sockaddr, which is common for UDP.
type Endpoint struct {
	Address netip.Addr
	Port    uint16
	Present bool
}

// Flow contains process, socket, and interface identity from a source
// descriptor.
type Flow struct {
	UniquePID          uint64
	EffectiveUniquePID uint64
	PID                uint32
	EffectivePID       uint32
	InterfaceIndex     uint32
	TCPState           uint32
	Local              Endpoint
	Remote             Endpoint
}

// Event is one decoded message from an NStat datagram.
type Event struct {
	Kind      EventKind
	Context   uint64
	SourceRef uint64
	Provider  uint32
	Error     uint32
	Counts    *Counts
	Flow      *Flow
}

// IsTCPProvider reports whether provider describes TCP flows.
func IsTCPProvider(provider uint32) bool {
	return provider == ProviderTCPKernel || provider == ProviderTCPUserland
}

// IsUDPProvider reports whether provider describes UDP flows.
func IsUDPProvider(provider uint32) bool {
	return provider == ProviderUDPKernel || provider == ProviderUDPUserland
}

// ParseDatagram decodes all length-framed messages in one kernel datagram.
// Unknown message types are skipped. Events decoded before an error are
// returned so callers can account for them, but must fail closed on
// ErrABIMismatch.
func ParseDatagram(data []byte) ([]Event, error) {
	var events []Event
	for offset := 0; offset < len(data); {
		if !rangeOK(data, offset, headerSize) {
			return events, ErrMalformed
		}
		length := int(binary.LittleEndian.Uint16(data[offset+12 : offset+14]))
		if length < headerSize || !rangeOK(data, offset, length) {
			return events, ErrMalformed
		}

		event, known, err := parseMessage(data[offset : offset+length])
		if err != nil {
			return events, err
		}
		if known {
			events = append(events, event)
		}
		offset += length
	}
	return events, nil
}

func parseMessage(message []byte) (Event, bool, error) {
	event := Event{
		Context: binary.LittleEndian.Uint64(message[0:8]),
	}
	messageType := binary.LittleEndian.Uint32(message[8:12])

	switch messageType {
	case messageSuccess:
		event.Kind = EventSuccess
	case messageError:
		event.Kind = EventError
		if !rangeOK(message, 16, 4) {
			return Event{}, false, ErrMalformed
		}
		event.Error = binary.LittleEndian.Uint32(message[16:20])
	case messageSourceAdd:
		event.Kind = EventAdded
		if !rangeOK(message, addedSourceRef, 12) {
			return Event{}, false, ErrMalformed
		}
		event.SourceRef = binary.LittleEndian.Uint64(message[addedSourceRef : addedSourceRef+8])
		event.Provider = binary.LittleEndian.Uint32(message[addedProvider : addedProvider+4])
	case messageSourceGone:
		event.Kind = EventRemoved
		if !rangeOK(message, addedSourceRef, 8) {
			return Event{}, false, ErrMalformed
		}
		event.SourceRef = binary.LittleEndian.Uint64(message[addedSourceRef : addedSourceRef+8])
	case messageCounts:
		event.Kind = EventCounts
		if !rangeOK(message, countsSourceRef, 8) {
			return Event{}, false, ErrMalformed
		}
		event.SourceRef = binary.LittleEndian.Uint64(message[countsSourceRef : countsSourceRef+8])
		counts, err := parseCounts(message, countsData)
		if err != nil {
			return Event{}, false, err
		}
		event.Counts = &counts
	case messageSourceDesc:
		event.Kind = EventDescription
		if !rangeOK(message, descriptionSourceRef, 8) ||
			!rangeOK(message, descriptionProvider, 4) {
			return Event{}, false, ErrMalformed
		}
		event.SourceRef = binary.LittleEndian.Uint64(message[descriptionSourceRef : descriptionSourceRef+8])
		event.Provider = binary.LittleEndian.Uint32(message[descriptionProvider : descriptionProvider+4])
		flow, err := parseFlow(message, descriptionData, event.Provider)
		if err != nil {
			return Event{}, false, err
		}
		event.Flow = &flow
	case messageUpdate:
		event.Kind = EventUpdate
		if !rangeOK(message, updateSourceRef, 8) ||
			!rangeOK(message, updateProvider, 4) {
			return Event{}, false, ErrMalformed
		}
		event.SourceRef = binary.LittleEndian.Uint64(message[updateSourceRef : updateSourceRef+8])
		event.Provider = binary.LittleEndian.Uint32(message[updateProvider : updateProvider+4])
		counts, err := parseCounts(message, updateCounts)
		if err != nil {
			return Event{}, false, err
		}
		flow, err := parseFlow(message, updateData, event.Provider)
		if err != nil {
			return Event{}, false, err
		}
		event.Counts = &counts
		event.Flow = &flow
	default:
		return Event{}, false, nil
	}
	return event, true, nil
}

func parseCounts(data []byte, offset int) (Counts, error) {
	if !rangeOK(data, offset, countsSize) {
		return Counts{}, ErrMalformed
	}
	u32 := func(field int) uint32 {
		return binary.LittleEndian.Uint32(data[offset+field : offset+field+4])
	}
	u64 := func(field int) uint64 {
		return binary.LittleEndian.Uint64(data[offset+field : offset+field+8])
	}
	return Counts{
		RXPackets:            u64(countsRXPackets),
		RXBytes:              u64(countsRXBytes),
		TXPackets:            u64(countsTXPackets),
		TXBytes:              u64(countsTXBytes),
		RXDuplicateBytes:     u32(countsRXDuplicate),
		RXOutOfOrderBytes:    u32(countsRXOutOfOrder),
		TXRetransmittedBytes: u32(countsTXRetransmitted),
		ConnectAttempts:      u32(countsConnectAttempts),
		ConnectSuccesses:     u32(countsConnectSuccess),
		MinimumRTT:           u32(countsMinRTT),
		AverageRTT:           u32(countsAvgRTT),
		RTTVariance:          u32(countsVarRTT),
	}, nil
}

func parseFlow(data []byte, offset int, provider uint32) (Flow, error) {
	var minimum, pidOffset, effectivePIDOffset, interfaceOffset, localOffset, remoteOffset int
	switch {
	case IsTCPProvider(provider):
		minimum = tcpMinimumSize
		pidOffset = tcpPID
		effectivePIDOffset = tcpEffectivePID
		interfaceOffset = tcpInterfaceIndex
		localOffset = tcpLocal
		remoteOffset = tcpRemote
	case IsUDPProvider(provider):
		minimum = udpMinimumSize
		pidOffset = udpPID
		effectivePIDOffset = udpEffectivePID
		interfaceOffset = udpInterfaceIndex
		localOffset = udpLocal
		remoteOffset = udpRemote
	default:
		return Flow{}, fmt.Errorf("%w: unknown provider %d", ErrABIMismatch, provider)
	}
	if !rangeOK(data, offset, minimum) {
		return Flow{}, ErrABIMismatch
	}
	description := data[offset:]
	local, err := parseEndpoint(description, localOffset)
	if err != nil {
		return Flow{}, ErrABIMismatch
	}
	remote, err := parseEndpoint(description, remoteOffset)
	if err != nil {
		return Flow{}, ErrABIMismatch
	}

	flow := Flow{
		UniquePID:          binary.LittleEndian.Uint64(description[descriptionUniquePID : descriptionUniquePID+8]),
		EffectiveUniquePID: binary.LittleEndian.Uint64(description[descriptionEffectiveUniquePID : descriptionEffectiveUniquePID+8]),
		PID:                binary.LittleEndian.Uint32(description[pidOffset : pidOffset+4]),
		EffectivePID:       binary.LittleEndian.Uint32(description[effectivePIDOffset : effectivePIDOffset+4]),
		InterfaceIndex:     binary.LittleEndian.Uint32(description[interfaceOffset : interfaceOffset+4]),
		Local:              local,
		Remote:             remote,
	}
	if IsTCPProvider(provider) {
		flow.TCPState = binary.LittleEndian.Uint32(description[tcpState : tcpState+4])
	}
	if flow.PID >= 1<<30 ||
		flow.InterfaceIndex >= 1<<24 ||
		(IsTCPProvider(provider) && flow.TCPState > 11) {
		return Flow{}, ErrABIMismatch
	}
	return flow, nil
}

func parseEndpoint(data []byte, offset int) (Endpoint, error) {
	if !rangeOK(data, offset, sockaddrStorageSize) {
		return Endpoint{}, ErrMalformed
	}
	sockaddr := data[offset : offset+sockaddrStorageSize]
	port := binary.BigEndian.Uint16(sockaddr[2:4])
	switch sockaddr[1] {
	case 0:
		if sockaddr[0] != 0 {
			return Endpoint{}, ErrABIMismatch
		}
		return Endpoint{}, nil
	case addressFamilyIPv4:
		if sockaddr[0] != 16 {
			return Endpoint{}, ErrABIMismatch
		}
		address := netip.AddrFrom4([4]byte(sockaddr[4:8]))
		return Endpoint{
			Address: address,
			Port:    port,
			Present: port != 0 || !address.IsUnspecified(),
		}, nil
	case addressFamilyIPv6:
		if sockaddr[0] != sockaddrStorageSize {
			return Endpoint{}, ErrABIMismatch
		}
		var raw [16]byte
		copy(raw[:], sockaddr[8:24])
		address := netip.AddrFrom16(raw)
		return Endpoint{
			Address: address,
			Port:    port,
			Present: port != 0 || !address.IsUnspecified(),
		}, nil
	default:
		return Endpoint{}, ErrABIMismatch
	}
}

func rangeOK(data []byte, offset, width int) bool {
	return offset >= 0 && width >= 0 && offset <= len(data) && width <= len(data)-offset
}
