// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

package net

// TODO: fix broken test
// func TestIPv6MarshalBinary(t *testing.T) {
// 	want := []byte{
// 		0x60, 0x00, 0x00, 0x00, // version, tclass, flow label
// 		0, 8, // payload length
// 		17,                                                                                             // next header
// 		5,                                                                                              // hop limit
// 		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, // src 2001:db8:1::10
// 		0x20, 0x01, 0x48, 0x60, 0x48, 0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x88, 0x88, // dst 2001:4860:4860::8888
// 		// udp
// 		0xab, 0xcd, 0xbc, 0xde, 0x00, 0x08, 0x00, 0x00,
// 	}
// 	iph := IPv6{
// 		Version:    6,
// 		PayloadLen: 8,
// 		NextHeader: ProtoUDP,
// 		HopLimit:   5,
// 		Src:        net.ParseIP("2001:db8:1::10"),
// 		Dst:        net.ParseIP("2001:4860:4860::8888"),
// 	}
// 	udp := UDP{
// 		Src: 0xabcd,
// 		Dst: 0xbcde,
// 	}
// 	iph.SetNext(&udp)
// 	b, err := iph.MarshalBinary()
// 	require.NoError(t, err)
// 	require.Equal(t, want, b)
// }

// TODO: fix broken test
// func TestIPv6UnmarshalBinary(t *testing.T) {
// 	data := []byte{
// 		0x61, 0x4a, 0xbc, 0xde, // version, tclass, flow label
// 		0, 8, // payload length
// 		17,                                                                                             // next header
// 		5,                                                                                              // hop limit
// 		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, // src 2001:db8:1::10
// 		0x20, 0x01, 0x48, 0x60, 0x48, 0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x88, 0x88, // dst 2001:4860:4860::8888
// 		// udp
// 		0xab, 0xcd, 0xbc, 0xde, 0x00, 0x08, 0x00, 0x00,
// 	}
// 	var ip IPv6
// 	err := ip.UnmarshalBinary(data)
// 	require.NoError(t, err)
// 	assert.Equal(t, 6, ip.Version)
// 	assert.Equal(t, 0x14, ip.TrafficClass)
// 	assert.Equal(t, 0xabcde, ip.FlowLabel)
// 	assert.Equal(t, 8, ip.PayloadLen)
// 	assert.Equal(t, ProtoUDP, ip.NextHeader)
// 	assert.Equal(t, net.ParseIP("2001:db8:1::10"), ip.Src)
// 	assert.Equal(t, net.ParseIP("2001:4860:4860::8888"), ip.Dst)
// 	require.NotNil(t, ip.Next())
// 	// TODO check UDP payload
// }
