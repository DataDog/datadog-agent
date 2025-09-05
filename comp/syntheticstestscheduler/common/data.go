// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
)

type Subtype string

const (
	SubtypeUDP  Subtype = "udp"
	SubtypeTCP  Subtype = "tcp"
	SubtypeICMP Subtype = "icmp"
)

type TCPMethod string

const (
	TCPMethodPreferSACK TCPMethod = "prefer_sack"
	TCPMethodSYN        TCPMethod = "syn"
	TCPMethodSACK       TCPMethod = "sack"
)

type ConfigRequest interface {
	GetSubType() Subtype
}

type NetworkConfigRequest struct {
	SourceService      *string `json:"source_service,omitempty"`
	DestinationService *string `json:"destination_service,omitempty"`
	ProbeCount         *int    `json:"probe_count,omitempty"`
	TracerouteCount    *int    `json:"traceroute_count,omitempty"`
	MaxTTL             *int    `json:"max_ttl,omitempty"`
	Timeout            *int    `json:"timeout,omitempty"` // in seconds
}

type UDPConfigRequest struct {
	Host string `json:"host"`
	Port *int   `json:"port,omitempty"`
	NetworkConfigRequest
}

func (u UDPConfigRequest) GetSubType() Subtype {
	return SubtypeUDP
}

type TCPConfigRequest struct {
	Host      string    `json:"host"`
	Port      *int      `json:"port,omitempty"`
	TCPMethod TCPMethod `json:"tcp_method"` // "prefer_sack" | "syn" | "sack"
	NetworkConfigRequest
}

func (t TCPConfigRequest) GetSubType() Subtype {
	return SubtypeTCP
}

type ICMPConfigRequest struct {
	Host string `json:"host"`
	NetworkConfigRequest
}

func (i ICMPConfigRequest) GetSubType() Subtype {
	return SubtypeICMP
}

type SyntheticsTestConfig struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Subtype string `json:"subtype"`

	Config struct {
		Assertions []interface{} `json:"assertions"`
		Request    ConfigRequest `json:"request"`
	} `json:"config"`

	Interval int    `json:"tick_every"`
	OrgID    int    `json:"orgID"`
	MainDC   string `json:"mainDC"`
	PublicID string `json:"publicID"`
}

// UnmarshalJSON is a Custom unmarshal for SyntheticsTestConfig
func (c *SyntheticsTestConfig) UnmarshalJSON(data []byte) error {
	type rawConfig struct {
		Version int    `json:"version"`
		Type    string `json:"type"`
		Subtype string `json:"subtype"`

		Config struct {
			Assertions []interface{}   `json:"assertions"`
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

	switch Subtype(tmp.Subtype) {
	case SubtypeUDP:
		var req UDPConfigRequest
		if err := json.Unmarshal(tmp.Config.Request, &req); err != nil {
			return err
		}
		c.Config.Request = req
	case SubtypeTCP:
		var req TCPConfigRequest
		if err := json.Unmarshal(tmp.Config.Request, &req); err != nil {
			return err
		}
		c.Config.Request = req
	case SubtypeICMP:
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
