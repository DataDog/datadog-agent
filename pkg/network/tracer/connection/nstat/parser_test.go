// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package nstat

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAggregateLifecycleAndError(t *testing.T) {
	added := message(messageSourceAdd, 32)
	putUint64(added, addedSourceRef, 99)
	putUint32(added, addedProvider, ProviderTCPKernel)

	failed := message(messageError, 24)
	putUint32(failed, 16, 55)

	removed := message(messageSourceGone, 24)
	putUint64(removed, addedSourceRef, 99)

	events, err := ParseDatagram(append(append(added, failed...), removed...))

	require.NoError(t, err)
	require.Equal(t, []Event{
		{Kind: EventAdded, SourceRef: 99, Provider: ProviderTCPKernel},
		{Kind: EventError, Error: 55},
		{Kind: EventRemoved, SourceRef: 99},
	}, events)
}

func TestParseTCPUpdate(t *testing.T) {
	update := message(messageUpdate, updateData+tcpMinimumSize)
	putUint64(update, updateSourceRef, 7)
	putUint32(update, updateProvider, ProviderTCPKernel)

	putUint64(update, updateCounts+countsRXPackets, 11)
	putUint64(update, updateCounts+countsRXBytes, 1200)
	putUint64(update, updateCounts+countsTXPackets, 13)
	putUint64(update, updateCounts+countsTXBytes, 1400)
	putUint32(update, updateCounts+countsRXDuplicate, 15)
	putUint32(update, updateCounts+countsRXOutOfOrder, 16)
	putUint32(update, updateCounts+countsTXRetransmitted, 17)
	putUint32(update, updateCounts+countsConnectAttempts, 2)
	putUint32(update, updateCounts+countsConnectSuccess, 1)
	putUint32(update, updateCounts+countsMinRTT, 20)
	putUint32(update, updateCounts+countsAvgRTT, 25)
	putUint32(update, updateCounts+countsVarRTT, 3)

	description := update[updateData:]
	putUint64(description, descriptionUniquePID, 0x123400001234)
	putUint64(description, descriptionEffectiveUniquePID, 0x432100004321)
	putUint32(description, tcpInterfaceIndex, 14)
	putUint32(description, tcpState, 4)
	putUint32(description, tcpPID, 1234)
	putUint32(description, tcpEffectivePID, 4321)
	putIPv4(description, tcpLocal, [4]byte{127, 0, 0, 1}, 50000)
	putIPv4(description, tcpRemote, [4]byte{1, 1, 1, 1}, 443)

	events, err := ParseDatagram(update)

	require.NoError(t, err)
	require.Len(t, events, 1)
	event := events[0]
	require.Equal(t, EventUpdate, event.Kind)
	require.Equal(t, uint64(7), event.SourceRef)
	require.Equal(t, ProviderTCPKernel, event.Provider)
	require.Equal(t, &Counts{
		RXPackets:            11,
		RXBytes:              1200,
		TXPackets:            13,
		TXBytes:              1400,
		RXDuplicateBytes:     15,
		RXOutOfOrderBytes:    16,
		TXRetransmittedBytes: 17,
		ConnectAttempts:      2,
		ConnectSuccesses:     1,
		MinimumRTT:           20,
		AverageRTT:           25,
		RTTVariance:          3,
	}, event.Counts)
	require.Equal(t, &Flow{
		UniquePID:          0x123400001234,
		EffectiveUniquePID: 0x432100004321,
		PID:                1234,
		EffectivePID:       4321,
		InterfaceIndex:     14,
		TCPState:           4,
		Local: Endpoint{
			Address: netip.MustParseAddr("127.0.0.1"),
			Port:    50000,
			Present: true,
		},
		Remote: Endpoint{
			Address: netip.MustParseAddr("1.1.1.1"),
			Port:    443,
			Present: true,
		},
	}, event.Flow)
}

func TestParseUDPDescriptionAndCounts(t *testing.T) {
	description := message(messageSourceDesc, descriptionData+udpMinimumSize)
	putUint64(description, descriptionSourceRef, 8)
	putUint32(description, descriptionProvider, ProviderUDPUserland)
	flow := description[descriptionData:]
	putUint64(flow, descriptionUniquePID, 8080)
	putUint32(flow, udpInterfaceIndex, 12)
	putUint32(flow, udpPID, 80)
	putUint32(flow, udpEffectivePID, 81)
	putIPv4(flow, udpLocal, [4]byte{}, 5353)
	putIPv4(flow, udpRemote, [4]byte{224, 0, 0, 251}, 5353)

	counts := message(messageCounts, countsData+countsSize)
	putUint64(counts, countsSourceRef, 8)
	putUint64(counts, countsData+countsRXBytes, 900)
	putUint64(counts, countsData+countsTXBytes, 901)

	events, err := ParseDatagram(description)
	require.NoError(t, err)
	more, err := ParseDatagram(counts)
	require.NoError(t, err)
	events = append(events, more...)

	require.Len(t, events, 2)
	require.Equal(t, EventDescription, events[0].Kind)
	require.Equal(t, uint32(12), events[0].Flow.InterfaceIndex)
	require.Equal(t, uint32(0), events[0].Flow.TCPState)
	require.Equal(t, uint16(5353), events[0].Flow.Local.Port)
	require.Equal(t, EventCounts, events[1].Kind)
	require.Equal(t, uint64(900), events[1].Counts.RXBytes)
	require.Equal(t, uint64(901), events[1].Counts.TXBytes)
}

func TestParseIPv6Description(t *testing.T) {
	description := message(messageSourceDesc, descriptionData+tcpMinimumSize)
	putUint32(description, descriptionProvider, ProviderTCPUserland)
	flow := description[descriptionData:]
	putUint32(flow, tcpState, 4)
	putUint32(flow, tcpPID, 80)
	putIPv6(flow, tcpLocal, netip.MustParseAddr("2001:db8::1").As16(), 50000)
	putIPv6(flow, tcpRemote, netip.MustParseAddr("2001:db8::2").As16(), 443)

	events, err := ParseDatagram(description)

	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, netip.MustParseAddr("2001:db8::1"), events[0].Flow.Local.Address)
	require.Equal(t, netip.MustParseAddr("2001:db8::2"), events[0].Flow.Remote.Address)
}

func TestParseDatagramRejectsMalformedFrames(t *testing.T) {
	shortLength := message(messageSuccess, headerSize)
	putUint16(shortLength, 12, 8)
	tooLong := message(messageSuccess, headerSize)
	putUint16(tooLong, 12, 32)
	trailing := append(message(messageSuccess, headerSize), 0)

	for name, tc := range map[string]struct {
		data        []byte
		priorEvents int
	}{
		"empty header":  {data: []byte{0}},
		"short length":  {data: shortLength},
		"long length":   {data: tooLong},
		"trailing byte": {data: trailing, priorEvents: 1},
	} {
		t.Run(name, func(t *testing.T) {
			events, err := ParseDatagram(tc.data)
			require.ErrorIs(t, err, ErrMalformed)
			require.Len(t, events, tc.priorEvents)
		})
	}
}

func TestParseDatagramAcceptsPartialDescriptionWithUnspecifiedEndpoints(t *testing.T) {
	for name, tc := range map[string]struct {
		provider uint32
		minimum  int
		pid      int
	}{
		"tcp": {provider: ProviderTCPKernel, minimum: tcpMinimumSize, pid: tcpPID},
		"udp": {provider: ProviderUDPKernel, minimum: udpMinimumSize, pid: udpPID},
	} {
		t.Run(name, func(t *testing.T) {
			description := message(messageSourceDesc, descriptionData+tc.minimum)
			putUint32(description, descriptionProvider, tc.provider)
			flow := description[descriptionData:]
			putUint32(flow, tc.pid, 80)

			events, err := ParseDatagram(description)

			require.NoError(t, err)
			require.Len(t, events, 1)
			require.Equal(t, uint32(80), events[0].Flow.PID)
			require.False(t, events[0].Flow.Local.Present)
			require.False(t, events[0].Flow.Remote.Present)
		})
	}
}

func TestParseDatagramRejectsImplausibleInterfaceIndex(t *testing.T) {
	description := message(messageSourceDesc, descriptionData+tcpMinimumSize)
	putUint32(description, descriptionProvider, ProviderTCPKernel)
	flow := description[descriptionData:]
	putUint32(flow, tcpState, 4)
	putUint32(flow, tcpPID, 80)
	putUint32(flow, tcpInterfaceIndex, 1<<24)
	putIPv4(flow, tcpLocal, [4]byte{127, 0, 0, 1}, 50000)

	events, err := ParseDatagram(description)

	require.ErrorIs(t, err, ErrABIMismatch)
	require.Empty(t, events)
}

func TestParseDatagramReturnsPriorEventsOnABIMismatch(t *testing.T) {
	added := message(messageSourceAdd, 32)
	putUint64(added, addedSourceRef, 99)
	putUint32(added, addedProvider, ProviderTCPKernel)
	badDescription := message(messageSourceDesc, descriptionData+tcpMinimumSize)
	putUint32(badDescription, descriptionProvider, ProviderTCPKernel)
	putUint32(badDescription[descriptionData:], tcpPID, 1<<30)

	events, err := ParseDatagram(append(added, badDescription...))

	require.ErrorIs(t, err, ErrABIMismatch)
	require.Equal(t, []Event{{Kind: EventAdded, SourceRef: 99, Provider: ProviderTCPKernel}}, events)
}

func TestParseDatagramSkipsUnknownMessages(t *testing.T) {
	events, err := ParseDatagram(message(99999, headerSize))

	require.NoError(t, err)
	require.Empty(t, events)
}

func TestProviderClassification(t *testing.T) {
	require.True(t, IsTCPProvider(ProviderTCPKernel))
	require.True(t, IsTCPProvider(ProviderTCPUserland))
	require.False(t, IsTCPProvider(ProviderUDPKernel))
	require.True(t, IsUDPProvider(ProviderUDPKernel))
	require.True(t, IsUDPProvider(ProviderUDPUserland))
	require.False(t, IsUDPProvider(ProviderTCPKernel))
}

func TestErrorKindsRemainDistinct(t *testing.T) {
	require.False(t, errors.Is(ErrMalformed, ErrABIMismatch))
}

func message(messageType uint32, length int) []byte {
	result := make([]byte, length)
	putUint32(result, 8, messageType)
	putUint16(result, 12, uint16(length))
	return result
}

func putUint16(buffer []byte, offset int, value uint16) {
	binary.LittleEndian.PutUint16(buffer[offset:offset+2], value)
}

func putUint32(buffer []byte, offset int, value uint32) {
	binary.LittleEndian.PutUint32(buffer[offset:offset+4], value)
}

func putUint64(buffer []byte, offset int, value uint64) {
	binary.LittleEndian.PutUint64(buffer[offset:offset+8], value)
}

func putIPv4(buffer []byte, offset int, address [4]byte, port uint16) {
	buffer[offset] = 16
	buffer[offset+1] = addressFamilyIPv4
	binary.BigEndian.PutUint16(buffer[offset+2:offset+4], port)
	copy(buffer[offset+4:offset+8], address[:])
}

func putIPv6(buffer []byte, offset int, address [16]byte, port uint16) {
	buffer[offset] = sockaddrStorageSize
	buffer[offset+1] = addressFamilyIPv6
	binary.BigEndian.PutUint16(buffer[offset+2:offset+4], port)
	copy(buffer[offset+8:offset+24], address[:])
}
