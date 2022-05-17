package common

import (
	"encoding/json"
	"hash/fnv"
	"strconv"
)

// Flow contains flow info used for aggregation
// json annotations are used in AsJSONString() for debugging purpose
type Flow struct {
	Namespace    string   `json:"namespace"`
	FlowType     FlowType `json:"type"`
	SamplingRate uint64   `json:"sampling_rate"`
	Direction    uint32   `json:"direction"`

	// Exporter information
	ExporterAddr string `json:"exporter_addr"`

	// Flow time
	StartTimestamp uint64 `json:"start_timestamp"` // in seconds
	EndTimestamp   uint64 `json:"end_timestamp"`   // in seconds

	// Size of the sampled packet
	Bytes   uint64 `json:"bytes"`
	Packets uint64 `json:"packets"`

	// Source/destination addresses
	SrcAddr string `json:"src_addr"` // FLOW KEY
	DstAddr string `json:"dst_addr"` // FLOW KEY

	// Layer 3 protocol (IPv4/IPv6/ARP/MPLS...)
	EtherType uint32 `json:"ether_type"`

	// Layer 4 protocol
	IPProtocol uint32 `json:"ip_protocol"` // FLOW KEY

	// Ports for UDP and TCP
	SrcPort uint32 `json:"src_port"` // FLOW KEY
	DstPort uint32 `json:"dst_port"` // FLOW KEY

	// SNMP Interface Index
	InputInterface  uint32 `json:"input_interface"` // FLOW KEY
	OutputInterface uint32 `json:"output_interface"`

	// Mac Address
	SrcMac uint64 `json:"src_mac"`
	DstMac uint64 `json:"dst_mac"`

	// Mask
	SrcMask uint32 `json:"src_mask"`
	DstMask uint32 `json:"dst_mask"`

	// Ethernet information
	Tos uint32 `json:"tos"` // FLOW KEY

	NextHop string `json:"next_hop"` // FLOW KEY
}

// AggregationHash return a hash used as aggregation key
func (f *Flow) AggregationHash() string {
	h := fnv.New64()
	h.Write([]byte(f.SrcAddr))                           //nolint:errcheck
	h.Write([]byte(f.DstAddr))                           //nolint:errcheck
	h.Write([]byte(strconv.Itoa(int(f.SrcPort))))        //nolint:errcheck
	h.Write([]byte(strconv.Itoa(int(f.DstPort))))        //nolint:errcheck
	h.Write([]byte(strconv.Itoa(int(f.IPProtocol))))     //nolint:errcheck
	h.Write([]byte(strconv.Itoa(int(f.Tos))))            //nolint:errcheck
	h.Write([]byte(strconv.Itoa(int(f.InputInterface)))) //nolint:errcheck
	return strconv.FormatUint(h.Sum64(), 16)
}

// AsJSONString returns a JSON string or "" in case of error during the Marshaling
// Used in debug logs. Marshalling to json can be costly if called in critical path.
func (f *Flow) AsJSONString() string {
	s, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return string(s)
}

// TelemetryTags return tags used for telemetry
func (f *Flow) TelemetryTags() []string {
	return []string{
		"exporter:" + f.ExporterAddr,
		"namespace:" + f.Namespace,
		"flow_type:" + string(f.FlowType),
	}
}
