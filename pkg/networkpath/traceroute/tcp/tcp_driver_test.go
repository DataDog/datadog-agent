// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tcp

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
)

func initTest(t *testing.T) (*TCPv4, *tcpDriver, *packets.MockSink, *packets.MockSource) {
	packets.RandomizePacketIDBase()

	ctrl := gomock.NewController(t)
	mockSource := packets.NewMockSource(ctrl)
	mockSink := packets.NewMockSink(ctrl)

	config := NewTCPv4(
		net.ParseIP("1.2.3.4"),
		80,
		1,
		1,
		30,
		10*time.Millisecond,
		1*time.Second,
		false,
	)
	config.srcIP = net.ParseIP("5.6.7.8")
	config.srcPort = 12345

	driver := newTCPDriver(config, mockSink, mockSource)

	return config, driver, mockSink, mockSource
}

func parseSynAndExpectIDs(t *testing.T, config *TCPv4, packetID uint16, seqNum uint32, buf []byte) {
	parser := packets.NewFrameParser()
	err := parser.Parse(buf)
	require.NoError(t, err)

	require.Equal(t, layers.LayerTypeIPv4, parser.GetIPLayer())
	require.Equal(t, layers.LayerTypeTCP, parser.GetTransportLayer())

	require.True(t, parser.TCP.SYN)
	require.False(t, parser.TCP.ACK)
	require.False(t, parser.TCP.RST)

	require.True(t, config.srcIP.Equal(parser.IP4.SrcIP))
	require.True(t, config.Target.Equal(parser.IP4.DstIP))

	require.Equal(t, config.srcPort, uint16(parser.TCP.SrcPort))
	require.Equal(t, config.DestPort, uint16(parser.TCP.DstPort))

	require.Equal(t, packetID, parser.IP4.Id)
	require.Equal(t, seqNum, parser.TCP.Seq)
}

func mockICMPResp(t *testing.T, config *TCPv4, hopIP net.IP, basePacketID uint16, ttl uint8, tcpInfo packets.TCPInfo) []byte {
	ipLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      42,
		Id:       1234,
		Protocol: layers.IPProtocolICMPv4,
		DstIP:    config.srcIP,
		SrcIP:    hopIP,
	}

	icmpLayer := &layers.ICMPv4{
		TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded),
	}
	innerIPLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      ttl,
		Id:       basePacketID + uint16(ttl),
		Protocol: layers.IPProtocolTCP,
		SrcIP:    config.srcIP,
		DstIP:    config.Target,
	}

	payload := packets.SerializeTCPFirstBytes(tcpInfo)

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

func mockTCPResp(t *testing.T, config *TCPv4, syn, ack, rst bool, seqNum uint32) []byte {
	ipLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      42,
		Id:       1234,
		Protocol: layers.IPProtocolTCP,
		DstIP:    config.srcIP,
		SrcIP:    config.Target,
	}

	tcpLayer := &layers.TCP{
		SrcPort: layers.TCPPort(config.DestPort),
		DstPort: layers.TCPPort(config.srcPort),
		Seq:     123,
		Ack:     seqNum + 1,
		SYN:     syn,
		ACK:     ack,
		RST:     rst,
		Window:  1024,
	}

	err := tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	require.NoError(t, err)

	// clear the gopacket.SerializeBuffer
	buf := gopacket.NewSerializeBuffer()

	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err = gopacket.SerializeLayers(buf, opts,
		ipLayer,
		tcpLayer,
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

func TestTCPDriverNoParallel(t *testing.T) {
	_, driver, _, _ := initTest(t)

	info := driver.GetDriverInfo()
	require.Equal(t, common.TracerouteDriverInfo{
		SupportsParallel: false,
	}, info)
}
func TestTCPDriverTwoHops(t *testing.T) {
	config, driver, mockSink, mockSource := initTest(t)

	// *** TTL=1 -- get back an ICMP TTL exceeded
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		parseSynAndExpectIDs(t, config, driver.basePacketID+1, driver.seqNum, buf)
		require.True(t, config.Target.Equal(addrPort.Addr().AsSlice()))
		require.Equal(t, config.DestPort, addrPort.Port())
		return nil
	})

	// trigger the mock
	err := driver.SendProbe(1)
	require.NoError(t, err)

	// make the source return an ICMP TTL exceeded
	hopIP := net.ParseIP("42.42.42.42")
	icmpResp := mockICMPResp(t, config, hopIP, driver.basePacketID, 1, packets.TCPInfo{
		SrcPort: config.srcPort,
		DstPort: config.DestPort,
		Seq:     driver.seqNum,
	})

	mockSource.EXPECT().SetReadDeadline(gomock.Any()).DoAndReturn(func(deadline time.Time) error {
		require.True(t, deadline.After(time.Now().Add(500*time.Millisecond)))
		return nil
	})
	mockRead(mockSource, icmpResp)

	// should get back the ICMP hop IP
	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.NoError(t, err)
	require.Equal(t, uint8(1), probeResp.TTL)
	require.True(t, hopIP.Equal(probeResp.IP.AsSlice()))
	require.False(t, probeResp.IsDest)

	// *** TTL=2 -- get back a SYNACK
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		parseSynAndExpectIDs(t, config, driver.basePacketID+2, driver.seqNum, buf)
		require.True(t, config.Target.Equal(addrPort.Addr().AsSlice()))
		require.Equal(t, config.DestPort, addrPort.Port())
		return nil
	})

	// send the second packet
	driver.SendProbe(2)

	// return synack
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).Return(nil)
	ackPacket := mockTCPResp(t, config, true, true, false, driver.seqNum)
	mockRead(mockSource, ackPacket)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.NoError(t, err)
	require.Equal(t, uint8(2), probeResp.TTL)
	require.True(t, config.Target.Equal(probeResp.IP.AsSlice()))
	require.True(t, probeResp.IsDest)
}

func TestTCPDriverRST(t *testing.T) {
	config, driver, mockSink, mockSource := initTest(t)

	// *** TTL=1 -- get back a RST
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		parseSynAndExpectIDs(t, config, driver.basePacketID+1, driver.seqNum, buf)
		require.True(t, config.Target.Equal(addrPort.Addr().AsSlice()))
		require.Equal(t, config.DestPort, addrPort.Port())
		return nil
	})

	// send the second packet
	driver.SendProbe(1)

	// return rst
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).Return(nil)
	// seqNum of 0 is okay because the RST does not have the ACK flag
	ackPacket := mockTCPResp(t, config, false, false, true, 0)

	mockRead(mockSource, ackPacket)

	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.NoError(t, err)
	require.Equal(t, uint8(1), probeResp.TTL)
	require.True(t, config.Target.Equal(probeResp.IP.AsSlice()))
	require.True(t, probeResp.IsDest)
}

func TestTCPDriverICMPMismatchedIP(t *testing.T) {
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
	tcpInfo := packets.TCPInfo{
		SrcPort: config.srcPort,
		DstPort: config.DestPort,
		Seq:     driver.seqNum,
	}
	icmpResp := mockICMPResp(t, &badConfig, hopIP, driver.basePacketID, 1, tcpInfo)

	mockRead(mockSource, icmpResp)

	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** CASE 2: mismatched dest IP
	hopIP = net.ParseIP("42.42.42.42")
	badConfig = *config
	badConfig.Target = net.ParseIP("8.8.8.8")
	icmpResp = mockICMPResp(t, &badConfig, hopIP, driver.basePacketID, 1, tcpInfo)

	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** CASE 3: mismatched packet ID
	icmpResp = mockICMPResp(t, config, hopIP, driver.basePacketID+123, 1, tcpInfo)

	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)
}
func TestTCPDriverICMPMismatchedTCPInfo(t *testing.T) {
	config, driver, mockSink, mockSource := initTest(t)
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).AnyTimes().Return(nil)
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).Return(nil)
	// trigger the mock
	err := driver.SendProbe(1)
	require.NoError(t, err)

	tcpInfo := packets.TCPInfo{
		SrcPort: config.srcPort,
		DstPort: config.DestPort,
		Seq:     driver.seqNum,
	}

	// *** get back an ICMP TTL exceeded, but for the wrong seq num
	hopIP := net.ParseIP("42.42.42.42")
	badTCPInfo := tcpInfo
	badTCPInfo.Seq++
	icmpResp := mockICMPResp(t, config, hopIP, driver.basePacketID, 1, badTCPInfo)

	mockRead(mockSource, icmpResp)

	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** get back an ICMP TTL exceeded, but for the wrong srcPort
	badTCPInfo = tcpInfo
	badTCPInfo.SrcPort++
	icmpResp = mockICMPResp(t, config, hopIP, driver.basePacketID, 1, badTCPInfo)

	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** get back an ICMP TTL exceeded, but for the wrong dstPort
	badTCPInfo = tcpInfo
	badTCPInfo.DstPort++
	icmpResp = mockICMPResp(t, config, hopIP, driver.basePacketID, 1, badTCPInfo)

	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)
}

func TestTCPDriverMismatchedSynackInfo(t *testing.T) {
	config, driver, mockSink, mockSource := initTest(t)
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).AnyTimes().Return(nil)
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	// trigger the mock
	err := driver.SendProbe(2)
	require.NoError(t, err)

	// *** get back a SYNACK, but for the wrong seq num
	ackPacket := mockTCPResp(t, config, true, true, false, driver.seqNum+1)
	mockRead(mockSource, ackPacket)

	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** get back a SYNACK, but for the wrong port
	badConfig := *config
	badConfig.srcPort++
	ackPacket = mockTCPResp(t, &badConfig, true, true, false, driver.seqNum)
	mockRead(mockSource, ackPacket)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** get back a SYNACK, but for the wrong dest port
	badConfig = *config
	badConfig.DestPort++
	ackPacket = mockTCPResp(t, &badConfig, true, true, false, driver.seqNum)
	mockRead(mockSource, ackPacket)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)
}

func TestTCPDriverMismatchedSynackIP(t *testing.T) {
	config, driver, mockSink, mockSource := initTest(t)
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).AnyTimes().Return(nil)
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	// trigger the mock
	err := driver.SendProbe(2)
	require.NoError(t, err)

	// *** get back a SYNACK, but for the wrong srcIP
	badConfig := *config
	badConfig.srcIP = net.ParseIP("8.8.8.8")
	ackPacket := mockTCPResp(t, &badConfig, true, true, false, driver.seqNum)
	mockRead(mockSource, ackPacket)

	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	// *** get back a SYNACK, but for the wrong dest IP
	badConfig = *config
	badConfig.Target = net.ParseIP("8.8.8.8")
	ackPacket = mockTCPResp(t, &badConfig, true, true, false, driver.seqNum)
	mockRead(mockSource, ackPacket)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)
}
