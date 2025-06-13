// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package udp

import (
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
)

func initTest(t *testing.T) (*UDPv4, *udpDriver, *packets.MockSink, *packets.MockSource) {
	packets.RandomizePacketIDBase()

	ctrl := gomock.NewController(t)
	mockSource := packets.NewMockSource(ctrl)
	mockSink := packets.NewMockSink(ctrl)

	config := NewUDPv4(
		net.ParseIP("1.2.3.4"),
		80,
		1,
		1,
		30,
		10*time.Millisecond,
		100*time.Second,
	)
	config.srcIP = net.ParseIP("5.6.7.8")
	config.srcPort = 12345

	driver := newUDPDriver(config, mockSink, mockSource)

	return config, driver, mockSink, mockSource
}

func expectIDs(t *testing.T, config *UDPv4, buf []byte) {
	var IP4 layers.IPv4
	var UDP layers.UDP
	var Payload gopacket.Payload

	parser := gopacket.NewDecodingLayerParser(
		layers.LayerTypeIPv4,
		&IP4, &UDP, &Payload, // include UDP here
	)
	decoded := []gopacket.LayerType{}
	err := parser.DecodeLayers(buf, &decoded)
	require.NoError(t, err)

	require.True(t, config.srcIP.Equal(IP4.SrcIP))
	require.True(t, config.Target.Equal(IP4.DstIP))
	require.Equal(t, config.srcPort, uint16(UDP.SrcPort))
	require.Equal(t, config.TargetPort, uint16(UDP.DstPort))
}

func mockICMPResp(t *testing.T, config *UDPv4, hopIP net.IP, id uint16, ttl uint8, udpInfo packets.UDPInfo, timeExceeded bool) []byte {
	ipLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      ttl,
		Id:       id,
		Protocol: layers.IPProtocolICMPv4,
		DstIP:    config.srcIP,
		SrcIP:    hopIP,
	}

	var icmpLayer *layers.ICMPv4
	if timeExceeded {
		icmpLayer = &layers.ICMPv4{
			TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded),
		}
	} else {
		icmpLayer = &layers.ICMPv4{
			TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeDestinationUnreachable, layers.ICMPv4CodePort),
		}
	}
	innerIPLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      ttl,
		Id:       id,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    config.srcIP,
		DstIP:    config.Target,
	}

	payload := packets.WriteUDPFirstBytes(udpInfo)

	// clear the gopacket.SerializeBuffer
	buf := gopacket.NewSerializeBuffer()

	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err := gopacket.SerializeLayers(buf, opts,
		ipLayer,
		icmpLayer,
		innerIPLayer,
		gopacket.Payload(payload),
	)
	require.NoError(t, err)
	return buf.Bytes()
}

func mockRead(mockSource *packets.MockSource, packet []byte) {
	mockSource.EXPECT().Read(gomock.Any()).DoAndReturn(func(buf []byte) (int, error) {
		n := copy(buf, packet)
		return n, nil
	})
}

func TestUDPDriverTwoHops(t *testing.T) {
	config, driver, mockSink, mockSource := initTest(t)

	// *** TTL=1 -- get back an ICMP TTL exceeded
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		expectIDs(t, config, buf)
		require.True(t, config.Target.Equal(addrPort.Addr().AsSlice()))
		require.Equal(t, config.TargetPort, addrPort.Port())
		return nil
	})

	// trigger the mock
	err := driver.SendProbe(1)
	require.NoError(t, err)
	var checksum uint16
	for k, _ := range driver.sentProbes {
		checksum = k.checksum
		break
	}
	// make the source return an ICMP TTL exceeded
	hopIP := net.ParseIP("42.42.42.42")
	icmpResp := mockICMPResp(t, config, hopIP, 41821, 1, packets.UDPInfo{
		SrcPort:  config.srcPort,
		DstPort:  config.TargetPort,
		ID:       uint16(1) + config.TargetPort,
		Checksum: checksum,
	}, true)

	mockSource.EXPECT().SetReadDeadline(gomock.Any()).DoAndReturn(func(deadline time.Time) error {
		//require.True(t, deadline.After(time.Now().Add(500*time.Millisecond)))
		return nil
	})
	mockRead(mockSource, icmpResp)

	// should get back the ICMP hop IP
	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.NoError(t, err)
	require.Equal(t, uint8(1), probeResp.TTL)
	require.True(t, hopIP.Equal(probeResp.IP.AsSlice()))
	require.False(t, probeResp.IsDest)

	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		//parseSynAndExpectIDs(t, config, driver.basePacketID+2, driver.seqNum, buf)
		require.True(t, config.Target.Equal(addrPort.Addr().AsSlice()))
		require.Equal(t, config.TargetPort, addrPort.Port())
		return nil
	})

	// send the second packet
	driver.SendProbe(2)

	for k, _ := range driver.sentProbes {
		if checksum != k.checksum {
			checksum = k.checksum
			break
		}
	}
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).Return(nil)
	icmpResp = mockICMPResp(t, config, config.Target, 41821, 2, packets.UDPInfo{
		SrcPort:  config.srcPort,
		DstPort:  config.TargetPort,
		ID:       uint16(2) + config.TargetPort,
		Checksum: checksum,
	}, false)
	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.NoError(t, err)
	require.Equal(t, uint8(2), probeResp.TTL)
	require.True(t, config.Target.Equal(probeResp.IP.AsSlice()))
	require.True(t, probeResp.IsDest)
}

func TestUDPDriverICMPMismatchedIP(t *testing.T) {
	config, driver, mockSink, mockSource := initTest(t)
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).AnyTimes().Return(nil)
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).Return(nil)

	// trigger the mock
	err := driver.SendProbe(1)
	require.NoError(t, err)

	// *** CASE 1: mismatched src IP
	hopIP := net.ParseIP("42.42.42.42")
	badConfig := *config
	badConfig.srcIP = net.ParseIP("8.8.8.8")

	var checksum uint16
	for k, _ := range driver.sentProbes {
		checksum = k.checksum
		break
	}
	icmpResp := mockICMPResp(t, &badConfig, hopIP, 41821, 1, packets.UDPInfo{
		SrcPort:  config.srcPort,
		DstPort:  config.TargetPort,
		ID:       uint16(1) + config.TargetPort,
		Checksum: checksum,
	}, true)
	mockRead(mockSource, icmpResp)

	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** CASE 2: mismatched dest IP
	hopIP = net.ParseIP("42.42.42.42")
	badConfig = *config
	badConfig.Target = net.ParseIP("8.8.8.8")
	for k, _ := range driver.sentProbes {
		checksum = k.checksum
		break
	}
	icmpResp = mockICMPResp(t, &badConfig, hopIP, 41821, 1, packets.UDPInfo{
		SrcPort:  config.srcPort,
		DstPort:  config.TargetPort,
		ID:       uint16(1) + config.TargetPort,
		Checksum: checksum,
	}, true)

	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)
}

func TestUDPDriverICMPMismatchedUDPInfo(t *testing.T) {
	config, driver, mockSink, mockSource := initTest(t)
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).AnyTimes().Return(nil)
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).Return(nil)
	// trigger the mock
	err := driver.SendProbe(1)
	require.NoError(t, err)

	// *** get back an ICMP TTL exceeded, but for the wrong seq num
	hopIP := net.ParseIP("42.42.42.42")
	var checksum uint16
	for k, _ := range driver.sentProbes {
		checksum = k.checksum
		break
	}
	icmpResp := mockICMPResp(t, config, hopIP, 41821, 2, packets.UDPInfo{
		SrcPort:  config.srcPort,
		DstPort:  config.TargetPort,
		ID:       uint16(2) + config.TargetPort,
		Checksum: checksum + 1,
	}, true)

	mockRead(mockSource, icmpResp)

	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** get back an ICMP TTL exceeded, but for the wrong srcPort
	icmpResp = mockICMPResp(t, config, hopIP, 41821, 1, packets.UDPInfo{
		SrcPort:  config.srcPort + 1,
		DstPort:  config.TargetPort,
		ID:       uint16(1) + config.TargetPort,
		Checksum: checksum,
	}, true)

	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** get back an ICMP TTL exceeded, but for the wrong dstPort
	icmpResp = mockICMPResp(t, config, hopIP, 41821, 1, packets.UDPInfo{
		SrcPort:  config.srcPort,
		DstPort:  config.TargetPort + 1,
		ID:       uint16(1) + config.TargetPort,
		Checksum: checksum,
	}, true)
	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)
}
