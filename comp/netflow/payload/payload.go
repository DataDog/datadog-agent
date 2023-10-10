// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package payload defines the JSON payload we send to the events platform.
package payload

// Device contains device details (device sending NetFlow flows)
type Device struct {
	Namespace string `json:"namespace"`
}

// Exporter contains NetFlow exporter details
type Exporter struct {
	IP string `json:"ip"`
}

// Endpoint contains source or destination endpoint details
type Endpoint struct {
	IP   string `json:"ip"`
	Port string `json:"port"` // Port number can be zero/positive or `*` (ephemeral port)
	Mac  string `json:"mac"`
	Mask string `json:"mask"`
}

// NextHop contains next hop details
type NextHop struct {
	IP string `json:"ip"`
}

// Interface contains interface details
type Interface struct {
	Index uint32 `json:"index"`
}

// ObservationPoint contains ingress or egress observation point
type ObservationPoint struct {
	Interface Interface `json:"interface"`
}

// FlowPayload contains network devices flows
type FlowPayload struct {
	FlushTimestamp int64            `json:"flush_timestamp"`
	FlowType       string           `json:"type"`
	SamplingRate   uint64           `json:"sampling_rate"`
	Direction      string           `json:"direction"`
	Start          uint64           `json:"start"` // in seconds
	End            uint64           `json:"end"`   // in seconds
	Bytes          uint64           `json:"bytes"`
	Packets        uint64           `json:"packets"`
	EtherType      string           `json:"ether_type,omitempty"`
	IPProtocol     string           `json:"ip_protocol"`
	Device         Device           `json:"device"`
	Exporter       Exporter         `json:"exporter"`
	Source         Endpoint         `json:"source"`
	Destination    Endpoint         `json:"destination"`
	Ingress        ObservationPoint `json:"ingress"`
	Egress         ObservationPoint `json:"egress"`
	Host           string           `json:"host"`
	TCPFlags       []string         `json:"tcp_flags,omitempty"`
	NextHop        NextHop          `json:"next_hop,omitempty"`
}
