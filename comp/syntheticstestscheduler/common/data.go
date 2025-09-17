// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package common contains common structures and constants used for synthetics test scheduler.
package common

import (
	"encoding/json"
	"fmt"
)

// SubType represents the network type of network test.
type SubType string

const (
	// SubTypeUDP represents a udp network test.
	SubTypeUDP SubType = "udp"
	// SubTypeTCP represents a tcp network test.
	SubTypeTCP SubType = "tcp"
	// SubTypeICMP represents a icmp network test.
	SubTypeICMP SubType = "icmp"
)

// TCPMethod represents the type of tcp network to establish.
type TCPMethod string

const (
	// TCPMethodPreferSACK represents a preference to sack tcp method if available for network test.
	TCPMethodPreferSACK TCPMethod = "prefer_sack"
	// TCPMethodSYN represents a syn tcp method for network test.
	TCPMethodSYN TCPMethod = "syn"
	// TCPMethodSACK represents a sack tcp method for network test.
	TCPMethodSACK TCPMethod = "sack"
)

// ConfigRequest represents the type configuration for a network test.
type ConfigRequest interface {
	GetSubType() SubType
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
func (u UDPConfigRequest) GetSubType() SubType {
	return SubTypeUDP
}

// TCPConfigRequest represents a tcp network test configuration.
type TCPConfigRequest struct {
	Host      string    `json:"host"`
	Port      *int      `json:"port,omitempty"`
	TCPMethod TCPMethod `json:"tcp_method"` // "prefer_sack" | "syn" | "sack"
	NetworkConfigRequest
}

// GetSubType returns the tcp subtype.
func (t TCPConfigRequest) GetSubType() SubType {
	return SubTypeTCP
}

// ICMPConfigRequest represents a icmp network test configuration.
type ICMPConfigRequest struct {
	Host string `json:"host"`
	NetworkConfigRequest
}

// GetSubType returns the icmp subtype.
func (i ICMPConfigRequest) GetSubType() SubType {
	return SubTypeICMP
}

// SyntheticsTestConfig represents the whole config of a network test.
type SyntheticsTestConfig struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Subtype string `json:"subtype"`

	Config struct {
		Assertions []Assertion   `json:"assertions"`
		Request    ConfigRequest `json:"request"`
	} `json:"config"`

	Interval int    `json:"tick_every"`
	OrgID    int    `json:"orgID"`
	MainDC   string `json:"mainDC"`
	PublicID string `json:"publicID"`
}

type Operator string

const (
	OperatorIs               = "is"
	OperatorIsNot            = "isNot"
	OperatorMoreThan         = "moreThan"
	OperatorMoreThanOrEquals = "moreThanOrEquals"
	OperatorLessThan         = "lessThan"
	OperatorLessThanOrEquals = "lessThanOrEquals"
)

type AssertionType string

const (
	AssertionTypeNetworkHops  AssertionType = "networkHops"
	AssertionTypeLatency      AssertionType = "latency"
	AssertionTypePacketLoss   AssertionType = "packetLossPercentage"
	AssertionTypePacketJitter AssertionType = "jitter"
)

type AssertionSubType string

const (
	AssertionSubTypeAverage AssertionSubType = "avg"
	AssertionSubTypeMin     AssertionSubType = "min"
	AssertionSubTypeMax     AssertionSubType = "max"
)

type Assertion struct {
	Operator Operator         `json:"operator"`
	Property AssertionSubType `json:"property"`
	Target   string           `json:"target"`
	Type     AssertionType    `json:"type"`
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

		OrgID    int    `json:"orgID"`
		MainDC   string `json:"mainDC"`
		PublicID string `json:"publicID"`
	}

	var tmp rawConfig
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	c.Version = tmp.Version
	c.Type = tmp.Type
	c.Subtype = tmp.Subtype
	c.OrgID = tmp.OrgID
	c.MainDC = tmp.MainDC
	c.PublicID = tmp.PublicID
	c.Config.Assertions = tmp.Config.Assertions

	switch SubType(tmp.Subtype) {
	case SubTypeUDP:
		var req UDPConfigRequest
		if err := json.Unmarshal(tmp.Config.Request, &req); err != nil {
			return err
		}
		c.Config.Request = req
	case SubTypeTCP:
		var req TCPConfigRequest
		if err := json.Unmarshal(tmp.Config.Request, &req); err != nil {
			return err
		}
		c.Config.Request = req
	case SubTypeICMP:
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
