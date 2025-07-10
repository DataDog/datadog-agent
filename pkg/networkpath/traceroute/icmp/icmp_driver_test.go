// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package icmp

import (
	"encoding/binary"
	"errors"
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

func initTest(t *testing.T, ipv6 bool) (*icmpDriver, *packets.MockSink, *packets.MockSource) {
	ctrl := gomock.NewController(t)
	mockSource := packets.NewMockSource(ctrl)
	mockSink := packets.NewMockSink(ctrl)

	ipAddress := netip.MustParseAddr("1.2.3.4")
	if ipv6 {
		ipAddress = netip.MustParseAddr("2001:0db8:abcd:0012::0a00:fffe")
	}
	params := Params{
		Target: ipAddress,
		ParallelParams: common.TracerouteParallelParams{TracerouteParams: common.TracerouteParams{
			MinTTL:            1,
			MaxTTL:            30,
			TracerouteTimeout: 100 * time.Second,
			PollFrequency:     time.Millisecond,
			SendDelay:         10 * time.Millisecond,
		}},
	}
	srcIP := netip.MustParseAddr("5.6.7.8")
	if ipv6 {
		srcIP = netip.MustParseAddr("2001:0db8:1234:5678:0000:0000:9abc:def0")
	}

	driver, err := newICMPDriver(params, srcIP, mockSink, mockSource)
	require.NoError(t, err)

	return driver, mockSink, mockSource
}

func expectIDs(t *testing.T, buf []byte, ipv6 bool, ttl uint8, srcIP, targetIP netip.Addr) {
	var IP4 layers.IPv4
	var IP6 layers.IPv6
	var ICMPV4 layers.ICMPv4
	var ICMPV6 layers.ICMPv6
	var Payload gopacket.Payload

	parser := gopacket.NewDecodingLayerParser(
		layers.LayerTypeIPv4,
		&IP4, &ICMPV4, &Payload,
	)
	if ipv6 {
		parser = gopacket.NewDecodingLayerParser(
			layers.LayerTypeIPv6,
			&IP6, &ICMPV6, &Payload,
		)
	}
	decoded := []gopacket.LayerType{}
	err := parser.DecodeLayers(buf, &decoded)
	var unsupportedErr gopacket.UnsupportedLayerType
	if errors.As(err, &unsupportedErr) {
		err = nil
	}
	require.NoError(t, err)

	if ipv6 {
		require.Equal(t, srcIP.Compare(netip.MustParseAddr(IP6.SrcIP.String())), 0)
		require.Equal(t, targetIP.Compare(netip.MustParseAddr(IP6.DstIP.String())), 0)
	} else {
		require.Equal(t, srcIP.Compare(netip.MustParseAddr(IP4.SrcIP.String())), 0)
		require.Equal(t, targetIP.Compare(netip.MustParseAddr(IP4.DstIP.String())), 0)
		require.Equal(t, ttl, uint8(ICMPV4.Seq))
	}
}

func mockICMPResp(t *testing.T, hopIP net.IP, ttl uint8, echoID uint16, icmpInfo packets.ICMPInfo, timeExceeded bool) []byte {
	ipLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      ttl,
		Protocol: layers.IPProtocolICMPv4,
		DstIP:    icmpInfo.ICMPPair.SrcAddr.AsSlice(),
		SrcIP:    hopIP,
	}

	var icmpLayer *layers.ICMPv4
	if timeExceeded {
		icmpLayer = &layers.ICMPv4{
			TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded),
		}
	} else {
		icmpLayer = &layers.ICMPv4{
			TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoReply, 0),
			Id:       echoID,
			Seq:      uint16(ttl),
		}
	}

	var payload gopacket.Payload
	if timeExceeded {
		innerIPLayer := &layers.IPv4{
			Version:  4,
			Length:   20,
			TTL:      ttl,
			Id:       echoID,
			Protocol: layers.IPProtocolICMPv4,
			SrcIP:    icmpInfo.ICMPPair.SrcAddr.AsSlice(),
			DstIP:    icmpInfo.ICMPPair.DstAddr.AsSlice(),
		}

		innerICMPEcho := &layers.ICMPv4{
			TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0),
			Id:       echoID,
			Seq:      uint16(ttl),
		}

		echoPayload := gopacket.Payload("hello")

		innerBuf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
		err := gopacket.SerializeLayers(innerBuf, opts,
			innerIPLayer,
			innerICMPEcho,
			echoPayload,
		)
		require.NoError(t, err)

		payload = innerBuf.Bytes()
	} else {
		payload = gopacket.Payload("hello")
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err := gopacket.SerializeLayers(buf, opts,
		ipLayer,
		icmpLayer,
		payload,
	)
	require.NoError(t, err)

	return buf.Bytes()
}

func mockICMPv6Resp(t *testing.T, hopIP net.IP, ttl uint8, echoID uint16, icmpInfo packets.ICMPInfo, timeExceeded bool) []byte {
	ipv6Layer := &layers.IPv6{
		Version:    6,
		HopLimit:   ttl,
		NextHeader: layers.IPProtocolICMPv6,
		SrcIP:      hopIP,
		DstIP:      icmpInfo.ICMPPair.SrcAddr.AsSlice(),
	}

	var icmpv6Layer *layers.ICMPv6
	var payload gopacket.Payload

	if timeExceeded {
		icmpv6Layer = &layers.ICMPv6{
			TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeTimeExceeded, 0),
		}
		err := icmpv6Layer.SetNetworkLayerForChecksum(ipv6Layer)
		require.NoError(t, err)

		innerIPv6 := &layers.IPv6{
			Version:    6,
			HopLimit:   ttl,
			NextHeader: layers.IPProtocolICMPv6,
			SrcIP:      icmpInfo.ICMPPair.SrcAddr.AsSlice(),
			DstIP:      icmpInfo.ICMPPair.DstAddr.AsSlice(),
		}

		innerICMPv6 := &layers.ICMPv6{
			TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeEchoRequest, 0),
		}
		err = innerICMPv6.SetNetworkLayerForChecksum(innerIPv6)
		require.NoError(t, err)

		innerEcho := &layers.ICMPv6Echo{
			Identifier: echoID,
			SeqNumber:  uint16(ttl),
		}

		echoPayload := gopacket.Payload("hello")

		innerBuf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
		err = gopacket.SerializeLayers(innerBuf, opts,
			innerIPv6,
			innerICMPv6,
			innerEcho,
			echoPayload,
		)
		require.NoError(t, err)

		payload = innerBuf.Bytes()
	} else {
		icmpv6Layer = &layers.ICMPv6{
			TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeEchoReply, 0),
		}
		err := icmpv6Layer.SetNetworkLayerForChecksum(ipv6Layer)
		require.NoError(t, err)
		buf := make([]byte, 4+5)
		binary.BigEndian.PutUint16(buf[0:2], echoID)
		binary.BigEndian.PutUint16(buf[2:4], uint16(ttl))
		copy(buf[4:], "hello")
		payload = buf
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}

	var err error
	if timeExceeded {
		err = gopacket.SerializeLayers(buf, opts,
			ipv6Layer,
			icmpv6Layer,
			// there are 4 unused bytes in the ICMP payload before the message body
			// https://en.wikipedia.org/wiki/ICMPv6#Format
			gopacket.Payload([]byte{0, 0, 0, 0}),
			payload,
		)
	} else {
		err = gopacket.SerializeLayers(buf, opts,
			ipv6Layer,
			icmpv6Layer,
			payload,
		)
	}
	require.NoError(t, err)

	return buf.Bytes()
}

func mockRead(mockSource *packets.MockSource, packet []byte) {
	mockSource.EXPECT().Read(gomock.Any()).DoAndReturn(func(buf []byte) (int, error) {
		n := copy(buf, packet)
		return n, nil
	})
}

func TestICMPDriverTwoHops(t *testing.T) {
	driver, mockSink, mockSource := initTest(t, false)

	// *** TTL=1 -- get back an ICMP TTL exceeded
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		expectIDs(t, buf, false, 1, driver.localAddr, driver.params.Target)
		require.Equal(t, driver.params.Target.Compare(addrPort.Addr()), 0)
		return nil
	})

	// trigger the mock
	err := driver.SendProbe(1)
	require.NoError(t, err)

	// make the source return an ICMP TTL exceeded
	hopIP := net.ParseIP("42.42.42.42")
	icmpResp := mockICMPResp(t, hopIP, 1, driver.echoID, packets.ICMPInfo{
		ICMPPair: packets.IPPair{
			SrcAddr: driver.localAddr,
			DstAddr: driver.params.Target,
		},
	}, true)

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

	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		expectIDs(t, buf, false, 2, driver.localAddr, driver.params.Target)
		require.Equal(t, driver.params.Target.Compare(addrPort.Addr()), 0)
		return nil
	})

	// send the second packet
	err = driver.SendProbe(2)
	require.NoError(t, err)

	mockSource.EXPECT().SetReadDeadline(gomock.Any()).Return(nil)
	icmpResp = mockICMPResp(t, driver.params.Target.AsSlice(), 2, driver.echoID, packets.ICMPInfo{
		ICMPPair: packets.IPPair{
			SrcAddr: driver.localAddr,
			DstAddr: driver.params.Target,
		},
	}, false)
	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.NoError(t, err)
	require.Equal(t, uint8(2), probeResp.TTL)
	require.Equal(t, driver.params.Target.AsSlice(), probeResp.IP.AsSlice())
	require.True(t, probeResp.IsDest)
}

func TestICMPDriverTwoHopsV6(t *testing.T) {
	driver, mockSink, mockSource := initTest(t, true)

	// *** TTL=1 -- get back an ICMP TTL exceeded
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		expectIDs(t, buf, true, 1, driver.localAddr, driver.params.Target)
		require.Equal(t, driver.params.Target.Compare(addrPort.Addr()), 0)
		return nil
	})

	// trigger the mock
	err := driver.SendProbe(1)
	require.NoError(t, err)

	// make the source return an ICMP TTL exceeded
	hopIP := net.ParseIP("2001:0db8:85a3::8a2e:0370:7334")
	icmpResp := mockICMPv6Resp(t, hopIP, 1, driver.echoID, packets.ICMPInfo{
		ICMPPair: packets.IPPair{
			SrcAddr: driver.localAddr,
			DstAddr: driver.params.Target,
		},
	}, true)

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

	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(buf []byte, addrPort netip.AddrPort) error {
		expectIDs(t, buf, true, 2, driver.localAddr, driver.params.Target)
		require.Equal(t, driver.params.Target.Compare(addrPort.Addr()), 0)
		return nil
	})

	// send the second packet
	err = driver.SendProbe(2)
	require.NoError(t, err)

	mockSource.EXPECT().SetReadDeadline(gomock.Any()).Return(nil)
	icmpResp = mockICMPv6Resp(t, driver.params.Target.AsSlice(), 2, driver.echoID, packets.ICMPInfo{
		ICMPPair: packets.IPPair{
			SrcAddr: driver.localAddr,
			DstAddr: driver.params.Target,
		},
	}, false)
	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.NoError(t, err)
	require.Equal(t, uint8(2), probeResp.TTL)
	require.Equal(t, driver.params.Target.AsSlice(), probeResp.IP.AsSlice())
	require.True(t, probeResp.IsDest)
}

func TestICMPDriverICMPMismatchedIP(t *testing.T) {
	driver, mockSink, mockSource := initTest(t, false)
	mockSource.EXPECT().SetReadDeadline(gomock.Any()).AnyTimes().Return(nil)
	mockSink.EXPECT().WriteTo(gomock.Any(), gomock.Any()).Return(nil)

	// trigger the mock
	err := driver.SendProbe(1)
	require.NoError(t, err)

	hopIP := net.ParseIP("42.42.42.42")
	badIP := netip.MustParseAddr("8.8.8.8")
	icmpResp := mockICMPResp(t, hopIP, 1, driver.echoID, packets.ICMPInfo{
		ICMPPair: packets.IPPair{
			SrcAddr: driver.localAddr,
			DstAddr: badIP,
		},
	}, true)

	mockRead(mockSource, icmpResp)

	probeResp, err := driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)

	icmpResp = mockICMPResp(t, hopIP, 1, driver.echoID, packets.ICMPInfo{
		ICMPPair: packets.IPPair{
			SrcAddr: badIP,
			DstAddr: driver.params.Target,
		},
	}, true)

	mockRead(mockSource, icmpResp)

	probeResp, err = driver.ReceiveProbe(1 * time.Second)
	require.Nil(t, probeResp)
	require.ErrorIs(t, err, common.ErrPacketDidNotMatchTraceroute)
}
