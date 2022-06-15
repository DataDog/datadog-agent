package common

import (
	"hash/fnv"
)

// Flow contains flow info used for aggregation
// json annotations are used in AsJSONString() for debugging purpose
type Flow struct {
	Namespace    string
	FlowType     FlowType
	SamplingRate uint64
	Direction    uint32

	// Exporter information
	ExporterAddr []byte

	// Flow time
	StartTimestamp uint64 // in seconds
	EndTimestamp   uint64 // in seconds

	// Size of the sampled packet
	Bytes   uint64
	Packets uint64

	// Source/destination addresses
	SrcAddr []byte // FLOW KEY
	DstAddr []byte // FLOW KEY

	// Layer 3 protocol (IPv4/IPv6/ARP/MPLS...)
	EtherType uint32

	// Layer 4 protocol
	IPProtocol uint32 // FLOW KEY

	// Flags
	TCPFlags uint32 `json:"tcp_flags"`

	// Ports for UDP and TCP
	SrcPort uint32 // FLOW KEY
	DstPort uint32 // FLOW KEY

	// SNMP Interface Index
	InputInterface  uint32 // FLOW KEY
	OutputInterface uint32

	// Mac Address
	SrcMac uint64
	DstMac uint64

	// Mask
	SrcMask uint32
	DstMask uint32

	// Ethernet information
	Tos uint32 // FLOW KEY

	NextHop []byte // FLOW KEY
}

// AggregationHash return a hash used as aggregation key
func (f *Flow) AggregationHash() uint64 {
	h := fnv.New64()
	h.Write([]byte(f.Namespace))             //nolint:errcheck
	h.Write(f.ExporterAddr)                  //nolint:errcheck
	h.Write(f.SrcAddr)                       //nolint:errcheck
	h.Write(f.DstAddr)                       //nolint:errcheck
	h.Write(Uint32ToBytes(f.SrcPort))        //nolint:errcheck
	h.Write(Uint32ToBytes(f.DstPort))        //nolint:errcheck
	h.Write(Uint32ToBytes(f.IPProtocol))     //nolint:errcheck
	h.Write(Uint32ToBytes(f.Tos))            //nolint:errcheck
	h.Write(Uint32ToBytes(f.InputInterface)) //nolint:errcheck
	return h.Sum64()
}
