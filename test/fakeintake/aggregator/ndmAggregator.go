// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// NDMPayload represents an NDM payload
type NDMPayload struct {
	collectedTime time.Time

	// We redefine NetworkDevicesMetadata from pkg/networkdevice/metadata to not have to export it
	Subnet           string              `json:"subnet,omitempty"`
	Namespace        string              `json:"namespace"`
	Integration      string              `json:"integration"`
	Devices          []DeviceMetadata    `json:"devices,omitempty"`
	Interfaces       []InterfaceMetadata `json:"interfaces,omitempty"`
	Diagnoses        []DiagnosisMetadata `json:"diagnoses,omitempty"`
	CollectTimestamp int64               `json:"collect_timestamp"`
}

// DeviceMetadata contains device metadata
type DeviceMetadata struct {
	ID             string   `json:"id"`
	IDTags         []string `json:"id_tags"`
	Tags           []string `json:"tags"`
	IPAddress      string   `json:"ip_address"`
	Status         int32    `json:"status"`
	PingStatus     int32    `json:"ping_status,omitempty"`
	Name           string   `json:"name,omitempty"`
	Description    string   `json:"description,omitempty"`
	SysObjectID    string   `json:"sys_object_id,omitempty"`
	Location       string   `json:"location,omitempty"`
	Profile        string   `json:"profile,omitempty"`
	ProfileVersion uint64   `json:"profile_version,omitempty"`
	Vendor         string   `json:"vendor,omitempty"`
	Subnet         string   `json:"subnet,omitempty"`
	SerialNumber   string   `json:"serial_number,omitempty"`
	Version        string   `json:"version,omitempty"`
	ProductName    string   `json:"product_name,omitempty"`
	Model          string   `json:"model,omitempty"`
	OsName         string   `json:"os_name,omitempty"`
	OsVersion      string   `json:"os_version,omitempty"`
	OsHostname     string   `json:"os_hostname,omitempty"`
	Integration    string   `json:"integration,omitempty"`
	DeviceType     string   `json:"device_type,omitempty"`
}

// InterfaceMetadata contains interface metadata
type InterfaceMetadata struct {
	DeviceID      string   `json:"device_id"`
	IDTags        []string `json:"id_tags"`
	Index         int32    `json:"index"`
	RawID         string   `json:"raw_id,omitempty"`
	RawIDType     string   `json:"raw_id_type,omitempty"`
	Name          string   `json:"name,omitempty"`
	Alias         string   `json:"alias,omitempty"`
	Description   string   `json:"description,omitempty"`
	MacAddress    string   `json:"mac_address,omitempty"`
	AdminStatus   int      `json:"admin_status,omitempty"`
	OperStatus    int      `json:"oper_status,omitempty"`
	MerakiEnabled *bool    `json:"meraki_enabled,omitempty"`
	MerakiStatus  string   `json:"meraki_status,omitempty"`
}

// Diagnosis contains data for a diagnosis
type Diagnosis struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Code     string `json:"code"`
}

// DiagnosisMetadata contains diagnoses info
type DiagnosisMetadata struct {
	ResourceType string      `json:"resource_type"`
	ResourceID   string      `json:"resource_id"`
	Diagnoses    []Diagnosis `json:"diagnoses"`
}

func (p *NDMPayload) name() string {
	return fmt.Sprintf("%s:%s integration:%s", p.Namespace, p.Subnet, p.Integration)
}

// GetTags returns the tags from a payload
func (p *NDMPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime returns the time when the payload has been collected by the fakeintake server
func (p *NDMPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseNDMPayload parses an api.Payload into a list of NDMPayload
func ParseNDMPayload(payload api.Payload) (ndmPayloads []*NDMPayload, err error) {
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		return []*NDMPayload{}, nil
	}
	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	ndmPayloads = []*NDMPayload{}
	err = json.Unmarshal(inflated, &ndmPayloads)
	if err != nil {
		return nil, err
	}
	for _, n := range ndmPayloads {
		n.collectedTime = payload.Timestamp
	}
	return ndmPayloads, err
}

// NDMAggregator is an Aggregator for NDM payloads
type NDMAggregator struct {
	Aggregator[*NDMPayload]
}

// NewNDMAggregator returns a new NDMAggregator
func NewNDMAggregator() NDMAggregator {
	return NDMAggregator{
		Aggregator: newAggregator(ParseNDMPayload),
	}
}
