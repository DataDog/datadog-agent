// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package common contains common structures and constants used for synthetics test scheduler.
package common

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

// ConfigRequest represents the type configuration for a network test.
type ConfigRequest interface {
	GetSubType() payload.Protocol
}

// NetworkConfigRequest represents the generic part of the network test configuration.
type NetworkConfigRequest struct {
	SourceService      *string `json:"source_service,omitempty"`
	DestinationService *string `json:"destination_service,omitempty"`
	ProbeCount         *int    `json:"probe_count,omitempty"`
	TracerouteCount    *int    `json:"traceroute_count,omitempty"`
	MaxTTL             *int    `json:"max_ttl,omitempty"`
	Timeout            *int    `json:"timeout,omitempty"` // in seconds
}

// UDPConfigRequest represents a udp network test configuration.
type UDPConfigRequest struct {
	Host string `json:"host"`
	Port *int   `json:"port,omitempty"`
	NetworkConfigRequest
}

// GetSubType returns the udp subtype.
func (u UDPConfigRequest) GetSubType() payload.Protocol {
	return payload.ProtocolUDP
}

// TCPConfigRequest represents a tcp network test configuration.
type TCPConfigRequest struct {
	Host      string            `json:"host"`
	Port      *int              `json:"port,omitempty"`
	TCPMethod payload.TCPMethod `json:"tcp_method"` // "prefer_sack" | "syn" | "sack"
	NetworkConfigRequest
}

// GetSubType returns the tcp subtype.
func (t TCPConfigRequest) GetSubType() payload.Protocol {
	return payload.ProtocolTCP
}

// ICMPConfigRequest represents a icmp network test configuration.
type ICMPConfigRequest struct {
	Host string `json:"host"`
	NetworkConfigRequest
}

// GetSubType returns the icmp subtype.
func (i ICMPConfigRequest) GetSubType() payload.Protocol {
	return payload.ProtocolICMP
}

// SyntheticsTestConfig represents the whole config of a network test.
type SyntheticsTestConfig struct {
	Version int    `json:"version"`
	Type    string `json:"type"`

	Config struct {
		Assertions []Assertion   `json:"assertions"`
		Request    ConfigRequest `json:"request"`
	} `json:"config"`

	Interval int    `json:"tick_every"`
	OrgID    int    `json:"org_id"`
	MainDC   string `json:"main_dc"`
	PublicID string `json:"public_id"`
	ResultID string `json:"result_id"`
	RunType  string `json:"run_type"`
}

// Operator represents a comparison operator for assertions.
type Operator string

const (
	// OperatorIs checks equality.
	OperatorIs Operator = "is"
	// OperatorIsNot checks inequality.
	OperatorIsNot Operator = "isNot"
	// OperatorMoreThan checks if greater than target.
	OperatorMoreThan Operator = "moreThan"
	// OperatorMoreThanOrEqual checks if greater than or equal to target.
	OperatorMoreThanOrEqual Operator = "moreThanOrEqual"
	// OperatorLessThan checks if less than target.
	OperatorLessThan Operator = "lessThan"
	// OperatorLessThanOrEqual checks if less than or equal to target.
	OperatorLessThanOrEqual Operator = "lessThanOrEqual"
)

// AssertionType represents the type of metric being asserted in a network test.
type AssertionType string

const (
	// AssertionTypeNetworkHops represents a network hops assertion.
	AssertionTypeNetworkHops AssertionType = "multiNetworkHop"
	// AssertionTypeLatency represents a latency assertion.
	AssertionTypeLatency AssertionType = "latency"
	// AssertionTypePacketLoss represents a packet loss percentage assertion.
	AssertionTypePacketLoss AssertionType = "packetLossPercentage"
	// AssertionTypePacketJitter represents a packet jitter assertion.
	AssertionTypePacketJitter AssertionType = "jitter"
)

// AssertionSubType represents the aggregation type for an assertion.
type AssertionSubType string

const (
	// AssertionSubTypeAverage represents the average value of the metric.
	AssertionSubTypeAverage AssertionSubType = "avg"
	// AssertionSubTypeMin represents the minimum value of the metric.
	AssertionSubTypeMin AssertionSubType = "min"
	// AssertionSubTypeMax represents the maximum value of the metric.
	AssertionSubTypeMax AssertionSubType = "max"
)

// Assertion represents a single condition to be checked in a network test.
type Assertion struct {
	Operator Operator          `json:"operator"`
	Property *AssertionSubType `json:"property,omitempty"`
	Target   string            `json:"target"`
	Type     AssertionType     `json:"type"`
}

// UnmarshalJSON is a Custom unmarshal for SyntheticsTestConfig
func (c *SyntheticsTestConfig) UnmarshalJSON(data []byte) error {
	type rawConfig struct {
		Version int    `json:"version"`
		Type    string `json:"type"`
		Subtype string `json:"subtype"`

		Config struct {
			Assertions []Assertion     `json:"assertions"`
			Request    json.RawMessage `json:"request"`
		} `json:"config"`

		OrgID    int    `json:"org_id"`
		MainDC   string `json:"main_dc"`
		PublicID string `json:"public_id"`
		ResultID string `json:"result_id"`
		RunType  string `json:"run_type"`
		Interval int    `json:"tick_every"`
	}

	var tmp rawConfig
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	c.Version = tmp.Version
	c.Type = tmp.Type
	c.OrgID = tmp.OrgID
	c.MainDC = tmp.MainDC
	c.PublicID = tmp.PublicID
	c.ResultID = tmp.ResultID
	c.RunType = tmp.RunType
	c.Interval = tmp.Interval
	c.Config.Assertions = tmp.Config.Assertions

	switch payload.Protocol(tmp.Subtype) {
	case payload.ProtocolUDP:
		var req UDPConfigRequest
		if err := json.Unmarshal(tmp.Config.Request, &req); err != nil {
			return err
		}
		c.Config.Request = req
	case payload.ProtocolTCP:
		var req TCPConfigRequest
		if err := json.Unmarshal(tmp.Config.Request, &req); err != nil {
			return err
		}
		c.Config.Request = req
	case payload.ProtocolICMP:
		var req ICMPConfigRequest
		if err := json.Unmarshal(tmp.Config.Request, &req); err != nil {
			return err
		}
		c.Config.Request = req
	default:
		// unknown subtype, return error
		return fmt.Errorf("unknown subtype: %s", tmp.Subtype)
	}

	return nil
}
