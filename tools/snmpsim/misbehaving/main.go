// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Standalone SNMP simulator that reproduces the AGENT-14643 infinite-cycle bug.
//
// It walks an in-memory OID tree normally for any real OID, but when handed an
// OID ending in `.0.0.1.5` (the pattern SkipOIDRowsNaive fabricates when walking
// inetCidrRouteTable rows), it returns an OID lexicographically BEFORE the request
// — the protocol violation that triggers the cycle detection error in the legacy scan.
//
// Usage:
//
//	go run . [-addr 127.0.0.1:1161]
//	sudo datadog-agent snmp scan 127.0.0.1:1161 -C public
package main

import (
	"flag"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// fabricatedSuffix is the OID suffix SkipOIDRowsNaive produces when given a
// inetCidrRouteTable row like ...0.0.1.4.172.21.200.192 — namely .0.0.1.5,
// where 4 is the IPv4 addressType and 5 is the (non-existent) next column.
const fabricatedSuffix = ".0.0.1.5"

// preBugOID is returned by the misbehaving server when handed the fabricated
// .1.5 OID. It is lexicographically BEFORE the request — the protocol violation
// that triggers the cycle bug.
const preBugOID = "1.3.6.1.2.1.4.24.7.1.1.1.4.0.0.0.0.0.2.0.0.1.4.172.21.211.204"

func main() {
	addr := flag.String("addr", "127.0.0.1:1161", "UDP address to listen on")
	flag.Parse()

	oids := []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.2.2.1.1.1",
		"1.3.6.1.2.1.2.2.1.1.2",
		"1.3.6.1.2.1.2.2.1.2.1",
		"1.3.6.1.2.1.2.2.1.2.2",
		// inetCidrRouteTable rows — well-formed entries that walk correctly.
		// When SkipOIDRowsNaive processes these it produces ...0.0.1.5 which
		// triggers the misbehaving response below.
		"1.3.6.1.2.1.4.24.7.1.1.1.4.0.0.0.0.0.2.0.0.1.4.10.0.0.0",
		"1.3.6.1.2.1.4.24.7.1.1.1.4.0.0.0.0.0.2.0.0.1.4.172.21.200.192",
		preBugOID,
		"1.3.6.1.2.1.99.0",
	}
	sort.Slice(oids, func(i, j int) bool { return oidLess(oids[i], oids[j]) })

	conn, err := net.ListenPacket("udp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer conn.Close()
	log.Printf("misbehaving SNMP server listening on udp://%s", conn.LocalAddr())
	log.Printf("scan with: sudo datadog-agent snmp scan %s -C public", conn.LocalAddr())

	codec := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: "public"}
	buf := make([]byte, 65535)

	for {
		n, src, err := conn.ReadFrom(buf)
		if err != nil {
			log.Fatalf("read: %v", err)
		}
		req, err := codec.SnmpDecodePacket(buf[:n])
		if err != nil {
			log.Printf("decode error: %v", err)
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

// lookupNext returns the next OID in the tree. For OIDs ending in the
// fabricated suffix it returns a lexicographically-earlier OID — the
// protocol violation that triggers the AGENT-14643 cycle bug.
func lookupNext(requested string, oids []string) gosnmp.SnmpPDU {
	requested = strings.TrimLeft(requested, ".")
	if strings.HasSuffix(requested, fabricatedSuffix) {
		log.Printf("  ** misbehaving response: returning %s (before %s) **", preBugOID, requested)
		return gosnmp.SnmpPDU{Name: preBugOID, Type: gosnmp.OctetString, Value: []byte("buggy")}
	}
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
