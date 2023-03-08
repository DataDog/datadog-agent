package netflow

import (
	"bytes"
	"encoding/binary"
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

//Marshall NetflowData into a buffer
func BuildNFlowPayload(data MockNetflowPacket) bytes.Buffer {
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
	return *buffer
}

func GenerateNetflow5Packet(startTime time.Time, records int) MockNetflowPacket {
	return MockNetflowPacket{
		Header:  CreateNFlowHeader(records, startTime),
		Records: CreateNFlowPayload(records),
	}
}

//Generate and initialize netflow header
func CreateNFlowHeader(recordCount int, startTime time.Time) MockNetflowHeader {

	t := time.Now().UnixNano()
	sec := t / int64(time.Second)
	nsec := t - sec*int64(time.Second)
	//startTime := time.Now().UnixNano()
	sysUptime := uint32((t-startTime.UnixNano())/int64(time.Millisecond)) + 1000
	flowSequence := uint32(recordCount) // TODO: use incremental flowSequence for multiple packets

	// log.Infof("Time: %d; Seconds: %d; Nanoseconds: %d\n", t, sec, nsec)
	// log.Infof("StartTime: %d; sysUptime: %d", StartTime, sysUptime)
	// log.Infof("FlowSequence %d", flowSequence)

	h := new(MockNetflowHeader)
	h.Version = 5
	h.FlowCount = uint16(recordCount)
	h.SysUptime = sysUptime
	h.UnixSec = uint32(sec)
	h.UnixMsec = uint32(nsec)
	h.FlowSequence = flowSequence
	h.EngineType = 1
	h.EngineId = 0
	h.SampleInterval = 0
	return *h
}

func CreateNFlowPayload(recordCount int) []MockNetflowPayload {
	payload := make([]MockNetflowPayload, recordCount)
	for i := 0; i < recordCount; i++ {
		payload[i] = CreateCustomRandomFlow(i)
	}
	return payload
}

func CreateCustomRandomFlow(index int) MockNetflowPayload {
	payload := new(MockNetflowPayload)
	payload.SrcIP = IPtoUint32("1.1.1.1")
	payload.DstIP = IPtoUint32("1.1.1.2")
	payload.SrcPort = 50000 + uint16(index)
	payload.DstPort = 80

	FillCommonFields(payload, 6, 32)
	return *payload
}

// patch up the common fields of the packets
func FillCommonFields(
	payload *MockNetflowPayload,
	ipProtocol int,
	srcPrefixMask int) MockNetflowPayload {

	// Fill template with values not filled by caller
	// payload.SrcIP = IPtoUint32("10.154.20.12")
	// payload.DstIP = IPtoUint32("77.12.190.94")
	// payload.NextHopIP = IPtoUint32("150.20.145.1")
	// payload.SrcPort = uint16(9010)
	// payload.DstPort = uint16(MYSQL_PORT)
	// payload.SnmpInIndex = genRandUint16(UINT16_MAX)
	// payload.SnmpOutIndex = genRandUint16(UINT16_MAX)
	payload.NumPackets = 1
	payload.NumOctets = 10
	// payload.SysUptimeStart = rand.Uint32()
	// payload.SysUptimeEnd = rand.Uint32()
	payload.Padding1 = 0
	payload.IpProtocol = uint8(ipProtocol)
	payload.IpTos = 0
	payload.SrcAsNumber = 1
	payload.DstAsNumber = 2

	payload.SrcPrefixMask = uint8(srcPrefixMask)
	payload.DstPrefixMask = uint8(32)
	payload.Padding2 = 0

	payload.SnmpInIndex = 10
	payload.SnmpOutIndex = 20

	// TODO: Add SysUptimeEnd and SysUptimeStart
	//uptime := int(sysUptime)
	//payload.SysUptimeEnd = uint32(uptime - randomNum(10, 500))
	//payload.SysUptimeStart = payload.SysUptimeEnd - uint32(randomNum(10, 500))

	// log.Infof("S&D : %x %x %d, %d", payload.SrcIP, payload.DstIP, payload.DstPort, payload.SnmpInIndex)
	// log.Infof("Time: %d %d %d", sysUptime, payload.SysUptimeStart, payload.SysUptimeEnd)

	return *payload
}

func IPtoUint32(s string) uint32 {
	ip := net.ParseIP(s)
	return binary.BigEndian.Uint32(ip.To4())
}
