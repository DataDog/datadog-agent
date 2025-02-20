// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix && pcap && cgo

// Package model holds model related files
package model

import (
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

/*

 */

func BenchmarkPcapReproducer(b *testing.B) {
	filters := []string{
		"not port 42",
		"port 5555",
		"tcp[((tcp[12:1] & 0xf0) >> 2):4] = 0x47455420",
		"icmp[icmptype] != icmp-echo and icmp[icmptype] != icmp-echoreply",
		"port ftp or ftp-data",
		"tcp[tcpflags] & (tcp-syn|tcp-fin) != 0 and not src and dst net 192.168.1.0/24",
		"tcp port 80 and (((ip[2:2] - ((ip[0]&0xf)<<2)) - ((tcp[12]&0xf0)>>2)) != 0)",
		"ether[0] & 1 = 0 and ip[16] >= 224",
		"udp port 67 and port 68",
		"((port 67 or port 68) and (udp[38:4] = 0x3e0ccf08))",
		"portrange 21-23",
		"tcp[13] & 8!=0",
		"",
	}

	captureLength := 256 // sizeof(struct raw_packet_t.data)

	for range b.N {
		for _, filter := range filters {
			filter, err := pcap.NewBPF(layers.LinkTypeEthernet, captureLength, filter)
			if err != nil {
				b.Errorf("failed to compile packet filter `%s`: %v", filter, err)
			}
		}
	}
}
