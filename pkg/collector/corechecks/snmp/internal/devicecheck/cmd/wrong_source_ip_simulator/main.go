// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

// Wrong Source IP Simulator
//
// This program simulates unexpected network behavior where a device responds to
// SNMP requests from a different IP than the one it was queried on.
//
// This can occur in certain network configurations (load balancers, NAT devices,
// devices with multiple interfaces and asymmetric routing, etc.).
//
// When you query IP A, the device responds from IP B. This breaks standard
// "connected" UDP sockets (which expect responses from the same IP they sent to)
// but works with "unconnected" UDP sockets.
//
// Usage:
//
//	go run main.go -listen 127.0.0.1:1161 -respond 127.0.0.2:0 -community public
//
// Then configure your SNMP check to query 127.0.0.1:1161.
// The response will come from 127.0.0.2, which will cause a timeout with
// connected sockets but succeed with unconnected sockets.
//
// Prerequisites:
//   - You need two IPs on your machine. On macOS, you can add a loopback alias:
//     sudo ifconfig lo0 alias 127.0.0.2
//   - On Linux:
//     sudo ip addr add 127.0.0.2/8 dev lo
package main

import (
	"encoding/asn1"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

// SNMP constants
const (
	snmpVersion2c = 1 // SNMPv2c version number in the packet

	// PDU types
	pduGetRequest     = 0
	pduGetNextRequest = 1
	pduGetResponse    = 2

	// sysObjectID OID: 1.3.6.1.2.1.1.2.0
	sysObjectIDOID = "1.3.6.1.2.1.1.2.0"
	// sysDescr OID: 1.3.6.1.2.1.1.1.0
	sysDescrOID = "1.3.6.1.2.1.1.1.0"
	// Device reachability check OID used by the agent
	deviceReachableOID = "1.3.6.1.2.1.1.3.0" // sysUpTime
)

// Simulated device values
var deviceValues = map[string]interface{}{
	sysObjectIDOID:     asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 9, 1, 1}, // Cisco
	sysDescrOID:        "Wrong Source IP Simulator - Tests unconnected UDP socket fallback",
	deviceReachableOID: 123456, // sysUpTime in ticks
}

func main() {
	listenAddr := flag.String("listen", "127.0.0.1:1161", "Address to listen for SNMP requests (IP:port)")
	respondAddr := flag.String("respond", "127.0.0.2:0", "Address to send responses FROM (IP:port, use port 0 for ephemeral)")
	community := flag.String("community", "public", "SNMP community string to accept")
	flag.Parse()

	fmt.Println("=== Wrong Source IP Simulator ===")
	fmt.Printf("Listening on:      %s\n", *listenAddr)
	fmt.Printf("Responding from:   %s\n", *respondAddr)
	fmt.Printf("Community string:  %s\n", *community)
	fmt.Println()
	fmt.Println("This simulates unexpected network behavior where a device responds")
	fmt.Println("from a different IP than the one queried.")
	fmt.Println("Connected UDP sockets will timeout; unconnected sockets will work.")
	fmt.Println()
	fmt.Println("To add loopback alias (if needed):")
	fmt.Println("  macOS: sudo ifconfig lo0 alias 127.0.0.2")
	fmt.Println("  Linux: sudo ip addr add 127.0.0.2/8 dev lo")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	// Parse addresses
	listenUDPAddr, err := net.ResolveUDPAddr("udp", *listenAddr)
	if err != nil {
		log.Fatalf("Invalid listen address: %v", err)
	}

	respondUDPAddr, err := net.ResolveUDPAddr("udp", *respondAddr)
	if err != nil {
		log.Fatalf("Invalid respond address: %v", err)
	}

	// Create listening socket
	listenConn, err := net.ListenUDP("udp", listenUDPAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", *listenAddr, err)
	}
	defer listenConn.Close()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		listenConn.Close()
		os.Exit(0)
	}()

	buf := make([]byte, 4096)
	for {
		n, clientAddr, err := listenConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			continue
		}

		log.Printf("Received %d bytes from %s", n, clientAddr)

		// Parse SNMP request
		response, err := handleSNMPRequest(buf[:n], *community)
		if err != nil {
			log.Printf("Failed to handle request: %v", err)
			continue
		}

		// Send response from DIFFERENT IP
		err = sendFromDifferentIP(respondUDPAddr, clientAddr, response)
		if err != nil {
			log.Printf("Failed to send response: %v", err)
			continue
		}

		log.Printf("Sent %d byte response from %s to %s", len(response), respondUDPAddr.IP, clientAddr)
	}
}

// sendFromDifferentIP sends a UDP packet from a specific source IP to the client
func sendFromDifferentIP(sourceAddr, destAddr *net.UDPAddr, data []byte) error {
	// Create a new socket bound to the "wrong" IP
	conn, err := net.DialUDP("udp", sourceAddr, destAddr)
	if err != nil {
		return fmt.Errorf("failed to dial from %s: %w", sourceAddr, err)
	}
	defer conn.Close()

	_, err = conn.Write(data)
	return err
}

// handleSNMPRequest parses an SNMP request and generates a response
func handleSNMPRequest(data []byte, expectedCommunity string) ([]byte, error) {
	// Parse the SNMP message (simplified BER/DER parsing)
	msg, err := parseSNMPMessage(data)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Verify community string
	if msg.Community != expectedCommunity {
		return nil, fmt.Errorf("wrong community: got %q, expected %q", msg.Community, expectedCommunity)
	}

	// Build response
	return buildSNMPResponse(msg)
}

// SNMPMessage represents a simplified SNMP message
type SNMPMessage struct {
	Version   int
	Community string
	PDUType   int
	RequestID int
	OIDs      []string
}

// parseSNMPMessage does basic SNMP message parsing
func parseSNMPMessage(data []byte) (*SNMPMessage, error) {
	// SNMP message is a SEQUENCE
	if len(data) < 2 || data[0] != 0x30 {
		return nil, fmt.Errorf("not an SNMP message")
	}

	// Skip SEQUENCE header
	pos := 2
	if data[1] > 0x80 {
		// Long form length
		lenBytes := int(data[1] & 0x7f)
		pos += lenBytes
	}

	msg := &SNMPMessage{}

	// Parse version (INTEGER)
	if data[pos] != 0x02 {
		return nil, fmt.Errorf("expected INTEGER for version")
	}
	pos++
	verLen := int(data[pos])
	pos++
	msg.Version = int(data[pos])
	pos += verLen

	// Parse community (OCTET STRING)
	if data[pos] != 0x04 {
		return nil, fmt.Errorf("expected OCTET STRING for community")
	}
	pos++
	commLen := int(data[pos])
	pos++
	msg.Community = string(data[pos : pos+commLen])
	pos += commLen

	// Parse PDU (CONTEXT-SPECIFIC)
	msg.PDUType = int(data[pos] & 0x1f)
	pos++

	// Skip PDU length
	if data[pos] > 0x80 {
		lenBytes := int(data[pos] & 0x7f)
		pos += 1 + lenBytes
	} else {
		pos++
	}

	// Parse request-id (INTEGER)
	if data[pos] != 0x02 {
		return nil, fmt.Errorf("expected INTEGER for request-id")
	}
	pos++
	reqIDLen := int(data[pos])
	pos++
	for i := 0; i < reqIDLen; i++ {
		msg.RequestID = (msg.RequestID << 8) | int(data[pos+i])
	}
	pos += reqIDLen

	// Skip error-status and error-index
	// error-status (INTEGER)
	pos++
	errStatusLen := int(data[pos])
	pos += 1 + errStatusLen
	// error-index (INTEGER)
	pos++
	errIndexLen := int(data[pos])
	pos += 1 + errIndexLen

	// Parse varbind list
	msg.OIDs = parseVarbindList(data[pos:])

	return msg, nil
}

// parseVarbindList extracts OIDs from the varbind list
func parseVarbindList(data []byte) []string {
	var oids []string

	if len(data) < 2 || data[0] != 0x30 {
		return oids
	}

	pos := 2
	if data[1] > 0x80 {
		lenBytes := int(data[1] & 0x7f)
		pos += lenBytes
	}

	// Iterate through varbinds
	for pos < len(data) {
		if data[pos] != 0x30 {
			break
		}
		pos++
		varbindLen := int(data[pos])
		pos++

		// Parse OID
		if data[pos] == 0x06 {
			pos++
			oidLen := int(data[pos])
			pos++
			oid := decodeOID(data[pos : pos+oidLen])
			oids = append(oids, oid)
			pos += varbindLen - 2 - oidLen
		} else {
			pos += varbindLen
		}
	}

	return oids
}

// decodeOID converts BER-encoded OID to string
func decodeOID(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// First byte encodes first two components
	oid := fmt.Sprintf("%d.%d", data[0]/40, data[0]%40)

	var val int
	for i := 1; i < len(data); i++ {
		val = (val << 7) | int(data[i]&0x7f)
		if data[i]&0x80 == 0 {
			oid += fmt.Sprintf(".%d", val)
			val = 0
		}
	}

	return oid
}

// buildSNMPResponse creates an SNMP response packet
func buildSNMPResponse(req *SNMPMessage) ([]byte, error) {
	// Build varbind list based on request
	var varbinds []byte

	for _, oid := range req.OIDs {
		var responseOID string
		var value interface{}

		if req.PDUType == pduGetNextRequest {
			// For GetNext, return the next OID
			responseOID, value = getNextOID(oid)
		} else {
			// For Get, return the exact OID
			responseOID = oid
			var ok bool
			value, ok = deviceValues[oid]
			if !ok {
				value = nil // noSuchObject
			}
		}

		varbind := encodeVarbind(responseOID, value)
		varbinds = append(varbinds, varbind...)
	}

	// Build varbind list SEQUENCE
	varbindList := encodeSequence(varbinds)

	// Build PDU
	pdu := encodeInteger(req.RequestID)    // request-id
	pdu = append(pdu, encodeInteger(0)...) // error-status
	pdu = append(pdu, encodeInteger(0)...) // error-index
	pdu = append(pdu, varbindList...)

	// Wrap PDU with GetResponse tag (0xa2)
	pduData := append([]byte{0xa2}, encodeLength(len(pdu))...)
	pduData = append(pduData, pdu...)

	// Build message
	message := encodeInteger(req.Version)
	message = append(message, encodeOctetString(req.Community)...)
	message = append(message, pduData...)

	return encodeSequence(message), nil
}

// getNextOID returns the next OID and its value (for GetNext requests)
func getNextOID(oid string) (string, interface{}) {
	// Simple implementation - return sysUpTime for any GetNext
	// This is enough to pass the reachability check
	return deviceReachableOID, deviceValues[deviceReachableOID]
}

// encodeSequence wraps data in a SEQUENCE
func encodeSequence(data []byte) []byte {
	return append(append([]byte{0x30}, encodeLength(len(data))...), data...)
}

// encodeLength encodes length in BER format
func encodeLength(length int) []byte {
	if length < 128 {
		return []byte{byte(length)}
	}
	if length < 256 {
		return []byte{0x81, byte(length)}
	}
	return []byte{0x82, byte(length >> 8), byte(length)}
}

// encodeInteger encodes an integer in BER format
func encodeInteger(val int) []byte {
	if val == 0 {
		return []byte{0x02, 0x01, 0x00}
	}

	var bytes []byte
	v := val
	for v > 0 {
		bytes = append([]byte{byte(v & 0xff)}, bytes...)
		v >>= 8
	}
	// Add leading zero if high bit is set (to keep it positive)
	if bytes[0]&0x80 != 0 {
		bytes = append([]byte{0x00}, bytes...)
	}

	return append([]byte{0x02, byte(len(bytes))}, bytes...)
}

// encodeOctetString encodes a string as OCTET STRING
func encodeOctetString(s string) []byte {
	return append(append([]byte{0x04}, encodeLength(len(s))...), []byte(s)...)
}

// encodeOID encodes an OID string to BER format
func encodeOID(oid string) []byte {
	var components []int
	var val int
	for _, c := range oid + "." {
		if c >= '0' && c <= '9' {
			val = val*10 + int(c-'0')
		} else if c == '.' {
			components = append(components, val)
			val = 0
		}
	}

	if len(components) < 2 {
		return []byte{0x06, 0x00}
	}

	// First two components combined
	encoded := []byte{byte(components[0]*40 + components[1])}

	// Remaining components
	for i := 2; i < len(components); i++ {
		v := components[i]
		if v < 128 {
			encoded = append(encoded, byte(v))
		} else {
			var octets []byte
			for v > 0 {
				octets = append([]byte{byte(v&0x7f) | 0x80}, octets...)
				v >>= 7
			}
			octets[len(octets)-1] &= 0x7f // Clear high bit on last byte
			encoded = append(encoded, octets...)
		}
	}

	return append([]byte{0x06, byte(len(encoded))}, encoded...)
}

// encodeVarbind encodes an OID-value pair
func encodeVarbind(oid string, value interface{}) []byte {
	oidBytes := encodeOID(oid)

	var valueBytes []byte
	switch v := value.(type) {
	case nil:
		// noSuchObject
		valueBytes = []byte{0x80, 0x00}
	case int:
		valueBytes = encodeInteger(v)
	case string:
		valueBytes = encodeOctetString(v)
	case asn1.ObjectIdentifier:
		oidStr := ""
		for i, n := range v {
			if i > 0 {
				oidStr += "."
			}
			oidStr += fmt.Sprintf("%d", n)
		}
		valueBytes = encodeOID(oidStr)
	default:
		valueBytes = []byte{0x05, 0x00} // NULL
	}

	varbind := append(oidBytes, valueBytes...)
	return encodeSequence(varbind)
}
