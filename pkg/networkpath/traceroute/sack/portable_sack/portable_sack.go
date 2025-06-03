package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func main() {
	// Your raw packet as a hex string
	rawHex := "6000000000093a0126001f1805b0b805e650d39fe773d649200148604860000000000000000088888000ff7bcb78000101"

	// Decode the hex string to bytes
	packetData, err := hex.DecodeString(rawHex)
	if err != nil {
		log.Fatalf("Failed to decode hex: %v", err)
	}

	// Set up decoding layers
	var (
		ipv6    layers.IPv6
		icmpv6  layers.ICMPv6
		payload gopacket.Payload
	)

	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv6, &ipv6, &icmpv6, &payload)
	decoded := []gopacket.LayerType{}

	err = parser.DecodeLayers(packetData, &decoded)
	var unsupportedErr gopacket.UnsupportedLayerType
	if errors.As(err, &unsupportedErr) {

		err = nil
	}
	if err != nil {
		log.Fatalf("Failed to parse packet: %v", err)
	}

	for _, layerType := range decoded {
		switch layerType {
		case layers.LayerTypeIPv6:
			fmt.Println("IPv6:")
			fmt.Printf("  Src: %s\n", ipv6.SrcIP)
			fmt.Printf("  Dst: %s\n", ipv6.DstIP)
			fmt.Printf("  HopLimit: %d\n", ipv6.HopLimit)
			fmt.Printf("  NextHeader: %s\n", ipv6.NextHeader)
		case layers.LayerTypeICMPv6:
			fmt.Println("ICMPv6:")
			fmt.Printf("  Type: %s\n", icmpv6.TypeCode.Type())
			fmt.Printf("  Code: %d\n", icmpv6.TypeCode.Code())
			fmt.Printf("  Checksum: 0x%x\n", icmpv6.Checksum)
		case gopacket.LayerTypePayload:
			fmt.Printf("Payload: %x\n", payload.LayerContents())
		}
	}
}
