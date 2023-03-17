// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package testutil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

type MockNetflowHeader struct {
	Version        uint16
	FlowCount      uint16
	SysUptime      uint32
	UnixSec        uint32
	UnixMsec       uint32
	FlowSequence   uint32
	EngineType     uint8
	EngineId       uint8
	SampleInterval uint16
}

type MockNetflowPayload struct {
	SrcIP          uint32
	DstIP          uint32
	NextHopIP      uint32
	SnmpInIndex    uint16
	SnmpOutIndex   uint16
	NumPackets     uint32
	NumOctets      uint32
	SysUptimeStart uint32
	SysUptimeEnd   uint32
	SrcPort        uint16
	DstPort        uint16
	Padding1       uint8
	TcpFlags       uint8
	IpProtocol     uint8
	IpTos          uint8
	SrcAsNumber    uint16
	DstAsNumber    uint16
	SrcPrefixMask  uint8
	DstPrefixMask  uint8
	Padding2       uint16
}

type MockNetflowPacket struct {
	Header  MockNetflowHeader
	Records []MockNetflowPayload
}

// BuildNetFlow5Payload builds netflow 5 payload
func BuildNetFlow5Payload(data MockNetflowPacket) []byte {
	buffer := new(bytes.Buffer)
	err := binary.Write(buffer, binary.BigEndian, &data.Header)
	if err != nil {
		log.Println("Writing netflow header failed:", err)
	}
	for _, record := range data.Records {
		err := binary.Write(buffer, binary.BigEndian, &record)
		if err != nil {
			log.Println("Writing netflow record failed:", err)
		}
	}
	return buffer.Bytes()
}

func GenerateNetflow5Packet(baseTime time.Time, records int) MockNetflowPacket {
	uptime := 100 * time.Second
	return MockNetflowPacket{
		Header:  CreateNFlowHeader(records, baseTime, uptime),
		Records: CreateNFlowPayload(records, baseTime, uptime),
	}
}

// CreateNFlowHeader netflow header
func CreateNFlowHeader(recordCount int, baseTime time.Time, uptime time.Duration) MockNetflowHeader {

	nanoSeconds := baseTime.UnixNano()
	sec := nanoSeconds / int64(time.Second)
	nsec := nanoSeconds - sec*int64(time.Second)

	flowSequence := uint32(recordCount) // TODO: use incremental flowSequence for multiple packets

	h := new(MockNetflowHeader)
	h.Version = 5
	h.FlowCount = uint16(recordCount)
	h.SysUptime = uint32(uptime.Milliseconds())
	h.UnixSec = uint32(sec)
	h.UnixMsec = uint32(nsec)
	h.FlowSequence = flowSequence
	h.EngineType = 1
	h.EngineId = 0
	h.SampleInterval = 0
	return *h
}

func CreateNFlowPayload(recordCount int, baseTime time.Time, uptime time.Duration) []MockNetflowPayload {
	payload := make([]MockNetflowPayload, recordCount)
	for i := 0; i < recordCount; i++ {
		payload[i] = CreateCustomRandomFlow(i, uptime)
	}
	return payload
}

func CreateCustomRandomFlow(index int, uptime time.Duration) MockNetflowPayload {
	payload := new(MockNetflowPayload)
	payload.SrcIP = IPtoUint32("10.0.0.1")
	payload.DstIP = IPtoUint32(fmt.Sprintf("20.0.0.%d", index))
	payload.SrcPort = 50000
	payload.DstPort = 8080

	payload.SysUptimeStart = uint32(uptime.Milliseconds())
	payload.SysUptimeEnd = uint32(uptime.Milliseconds())

	payload.NumPackets = 10
	payload.NumOctets = 194
	payload.IpProtocol = 6
	payload.SnmpInIndex = 1
	payload.SnmpOutIndex = 7
	payload.TcpFlags = 22

	return *payload
}

func IPtoUint32(s string) uint32 {
	ip := net.ParseIP(s)
	return binary.BigEndian.Uint32(ip.To4())
}
