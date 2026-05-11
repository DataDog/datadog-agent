// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Standalone SNMP simulator that reproduces the AGENT-15001 device crash bug.
//
// It walks an in-memory OID tree normally, but when it receives a GetNext or
// GetBulk for an OID matching the SkipOIDRowsNaive fabrication pattern against
// ipNetToPhysicalTable (e.g. 1.3.6.1.2.1.4.35.1.4.<ifIndex>.1.5), it simulates
// an Extreme switch crash by ceasing to respond to all subsequent requests.
//
// The crashing OID is produced by SkipOIDRowsNaive when walking rows like
// 1.3.6.1.2.1.4.35.1.4.<ifIndex>.1.4.W.X.Y.Z (addressType=1 IPv4, 4 octets).
// It increments the addressType segment (4) to 5, producing a malformed
// InetAddress that real Extreme hardware cannot handle.
//
// Usage:
//
//	go run . [-addr 127.0.0.1:1162]
//	sudo datadog-agent snmp scan 127.0.0.1:1162 -C public
package main

import (
	"flag"
	"log"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/gosnmp/gosnmp"
)

// crashPattern matches the fabricated ipNetToPhysicalTable OID produced by
// SkipOIDRowsNaive: 1.3.6.1.2.1.4.35.1.4.<ifIndex>.1.5
var crashPattern = regexp.MustCompile(`^\.?1\.3\.6\.1\.2\.1\.4\.35\.1\.4\.\d+\.1\.5$`)

func main() {
	addr := flag.String("addr", "127.0.0.1:1162", "UDP address to listen on")
	flag.Parse()

	oids := []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.2.2.1.1.1",
		"1.3.6.1.2.1.2.2.1.1.2",
		"1.3.6.1.2.1.2.2.1.2.1",
		"1.3.6.1.2.1.2.2.1.2.2",
		// ipNetToPhysicalTable rows: .1.4.<ifIndex>.1.4.<IPv4 octets>
		// IPs without .1. in the address so SkipOIDRowsNaive finds the
		// addressType .1. prefix and fabricates ...1.5 — the malformed OID.
		"1.3.6.1.2.1.4.35.1.4.1000007.1.4.10.0.0.5",
		"1.3.6.1.2.1.4.35.1.4.1000007.1.4.10.0.0.6",
		"1.3.6.1.2.1.99.0",
	}
	sort.Slice(oids, func(i, j int) bool { return oidLess(oids[i], oids[j]) })

	conn, err := net.ListenPacket("udp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer conn.Close()
	log.Printf("crashing SNMP server listening on udp://%s", conn.LocalAddr())
	log.Printf("scan with: sudo datadog-agent snmp scan %s -C public", conn.LocalAddr())

	codec := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: "public"}
	var crashed atomic.Bool
	buf := make([]byte, 65535)

	for {
		n, src, err := conn.ReadFrom(buf)
		if err != nil {
			log.Fatalf("read: %v", err)
		}
		if crashed.Load() {
			log.Printf("(crashed) dropped %d-byte packet from %s", n, src)
			continue
		}
		req, err := codec.SnmpDecodePacket(buf[:n])
		if err != nil {
			log.Printf("decode error: %v", err)
			continue
		}
		for _, v := range req.Variables {
			if crashPattern.MatchString(v.Name) {
				log.Printf("  ** simulating EXOS crash on %s — dropping this and all future packets **", v.Name)
				crashed.Store(true)
				break
			}
		}
		if crashed.Load() {
			continue
		}
		resp := buildResponse(req, oids)
		out, err := resp.MarshalMsg()
		if err != nil {
			log.Printf("marshal error: %v", err)
			continue
		}
		_, _ = conn.WriteTo(out, src)
	}
}

func buildResponse(req *gosnmp.SnmpPacket, oids []string) *gosnmp.SnmpPacket {
	var responses []gosnmp.SnmpPDU
	switch req.PDUType {
	case gosnmp.GetNextRequest:
		for _, v := range req.Variables {
			pdu := lookupNext(v.Name, oids)
			log.Printf("GetNext(%s) -> %s", v.Name, pdu.Name)
			responses = append(responses, pdu)
		}
	case gosnmp.GetBulkRequest:
		for _, v := range req.Variables {
			current := v.Name
			log.Printf("GetBulk(%s, max=%d)", v.Name, req.MaxRepetitions)
			for i := uint32(0); i < req.MaxRepetitions; i++ {
				pdu := lookupNext(current, oids)
				responses = append(responses, pdu)
				if pdu.Type == gosnmp.EndOfMibView {
					break
				}
				current = pdu.Name
			}
		}
	case gosnmp.GetRequest:
		for _, v := range req.Variables {
			responses = append(responses, gosnmp.SnmpPDU{Name: v.Name, Type: gosnmp.NoSuchObject})
		}
	}
	return &gosnmp.SnmpPacket{
		Version:   req.Version,
		Community: req.Community,
		PDUType:   gosnmp.GetResponse,
		RequestID: req.RequestID,
		Variables: responses,
	}
}

func lookupNext(requested string, oids []string) gosnmp.SnmpPDU {
	requested = strings.TrimLeft(requested, ".")
	for _, oid := range oids {
		if oidLess(requested, oid) {
			return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.OctetString, Value: []byte("ok")}
		}
	}
	return gosnmp.SnmpPDU{Name: requested, Type: gosnmp.EndOfMibView}
}

func oidLess(a, b string) bool {
	aSeg := strings.Split(strings.TrimLeft(a, "."), ".")
	bSeg := strings.Split(strings.TrimLeft(b, "."), ".")
	n := len(aSeg)
	if len(bSeg) < n {
		n = len(bSeg)
	}
	for i := 0; i < n; i++ {
		ai, _ := strconv.Atoi(aSeg[i])
		bi, _ := strconv.Atoi(bSeg[i])
		if ai != bi {
			return ai < bi
		}
	}
	return len(aSeg) < len(bSeg)
}
