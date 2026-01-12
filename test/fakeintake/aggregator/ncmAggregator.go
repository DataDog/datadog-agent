// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// NCMPayload represents a network config management payload
type NCMPayload struct {
	collectedTime    time.Time
	Namespace        string                `json:"namespace"`
	Configs          []NetworkDeviceConfig `json:"configs"`
	CollectTimestamp int64                 `json:"collect_timestamp"`
}

// NetworkDeviceConfig contains network device configuration for a single device
type NetworkDeviceConfig struct {
	DeviceID   string   `json:"device_id"`
	DeviceIP   string   `json:"device_ip"`
	ConfigType string   `json:"config_type"`
	Timestamp  int64    `json:"timestamp"`
	Tags       []string `json:"tags"`
	Content    string   `json:"content"`
}

func (p *NCMPayload) name() string {
	return p.Namespace
}

// GetTags return the tags from a payload
func (p *NCMPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (p *NCMPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseNCMPayload parses an api.Payload into a list of NCMPayload
func ParseNCMPayload(payload api.Payload) (ncmPayloads []*NCMPayload, err error) {
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		return []*NCMPayload{}, nil
	}
	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}

	var singlePayload NCMPayload
	err = json.Unmarshal(inflated, &singlePayload)
	if err != nil {
		return nil, err
	}
	singlePayload.collectedTime = payload.Timestamp
	ncmPayloads = append(ncmPayloads, &singlePayload)

	return ncmPayloads, nil
}

// NCMAggregator is an Aggregator for NCM payloads
type NCMAggregator struct {
	Aggregator[*NCMPayload]
}

// NewNCMAggregator return a new NCMAggregator
func NewNCMAggregator() NCMAggregator {
	return NCMAggregator{
		Aggregator: newAggregator(ParseNCMPayload),
	}
}
