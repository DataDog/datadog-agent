// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap

// Package model holds model related files
package model

import (
	"encoding/binary"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

/*
func (e *RawPacketEvent) cDump(data []byte) {
	fmt.Printf("unsigned char data[] = {")
	for i := 0; i < len(data); i++ {
		if i%16 == 0 {
			fmt.Printf("\n")
		}
		fmt.Printf("0x%02x", data[i])
		if i < len(data)-1 {
			fmt.Printf(",")
		}
	}
	fmt.Printf("};\n")
}
*/

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RawPacketEvent) UnmarshalBinary(data []byte) (int, error) {
	read, err := e.NetworkContext.Device.UnmarshalBinary(data)
	if err != nil {
		return 0, ErrNotEnoughData
	}
	data = data[read:]

	e.Size = binary.NativeEndian.Uint32(data)
	data = data[4:]
	e.Data = data
	e.CaptureInfo.InterfaceIndex = int(e.NetworkContext.Device.IfIndex)
	e.CaptureInfo.Length = int(e.NetworkContext.Size)
	e.CaptureInfo.CaptureLength = len(data)

	//e.cDump(data)

	packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.DecodeOptions{NoCopy: true, Lazy: true, DecodeStreamsAsDatagrams: true})
	if layer := packet.Layer(layers.LayerTypeIPv4); layer != nil {
		if rl, ok := layer.(*layers.IPv4); ok {
			e.L3Protocol = unix.ETH_P_IP
			e.Source.IPNet = *eval.IPNetFromIP(rl.SrcIP)
			e.Destination.IPNet = *eval.IPNetFromIP(rl.DstIP)
		}
	} else if layer := packet.Layer(layers.LayerTypeIPv6); layer != nil {
		if rl, ok := layer.(*layers.IPv4); ok {
			e.L3Protocol = unix.ETH_P_IPV6
			e.Source.IPNet = *eval.IPNetFromIP(rl.SrcIP)
			e.Destination.IPNet = *eval.IPNetFromIP(rl.DstIP)
		}
	}

	if layer := packet.Layer(layers.LayerTypeUDP); layer != nil {
		if rl, ok := layer.(*layers.UDP); ok {
			e.L4Protocol = unix.IPPROTO_UDP
			e.Source.Port = uint16(rl.SrcPort)
			e.Destination.Port = uint16(rl.DstPort)
		}
	} else if layer := packet.Layer(layers.LayerTypeTCP); layer != nil {
		if rl, ok := layer.(*layers.TCP); ok {
			e.L4Protocol = unix.IPPROTO_TCP
			e.Source.Port = uint16(rl.SrcPort)
			e.Destination.Port = uint16(rl.DstPort)
		}
	}

	if layer := packet.Layer(layers.LayerTypeTLS); layer != nil {
		if rl, ok := layer.(*layers.TLS); ok {
			if len(rl.AppData) > 0 {
				e.TLSContext.Version = uint16(rl.AppData[0].Version)
			}
		}
	}

	return len(data), nil
}
