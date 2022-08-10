//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package payload

// Device contains device (exporter) details
// easyjson:json
type Device struct {
	IP        string `json:"ip"`
	Namespace string `json:"namespace"`
}

// Endpoint contains source or destination endpoint details
// easyjson:json
type Endpoint struct {
	IP   string `json:"ip"`
	Port uint32 `json:"port"`
	Mac  string `json:"mac"`
	Mask string `json:"mask"`
}

// NextHop contains next hop details
// easyjson:json
type NextHop struct {
	IP string `json:"ip"`
}

// Interface contains interface details
// easyjson:json
type Interface struct {
	Index uint32 `json:"index"`
}

// ObservationPoint contains ingress or egress observation point
// easyjson:json
type ObservationPoint struct {
	Interface Interface `json:"interface"`
}

// FlowPayload contains network devices flows
// easyjson:json
type FlowPayload struct {
	FlowType     string           `json:"type"`
	SamplingRate uint64           `json:"sampling_rate"`
	Direction    string           `json:"direction"`
	Start        uint64           `json:"start"` // in seconds
	End          uint64           `json:"end"`   // in seconds
	Bytes        uint64           `json:"bytes"`
	Packets      uint64           `json:"packets"`
	EtherType    string           `json:"ether_type,omitempty"`
	IPProtocol   string           `json:"ip_protocol"`
	Device       Device           `json:"device"`
	Source       Endpoint         `json:"source"`
	Destination  Endpoint         `json:"destination"`
	Ingress      ObservationPoint `json:"ingress"`
	Egress       ObservationPoint `json:"egress"`
	Host         string           `json:"host"`
	TCPFlags     []string         `json:"tcp_flags,omitempty"`
	NextHop      NextHop          `json:"next_hop,omitempty"`
}
