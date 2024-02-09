// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Metadata is a type that represents a metadata payload
type MetadataPayload struct {
	Hostname      string                 `json:"hostname"`
	Timestamp     int64                  `json:"timestamp"`
	Metadata      map[string]interface{} `json:"agent_metadata"`
	collectedTime time.Time
}

func (mp *MetadataPayload) name() string {
	return mp.Hostname
}

// GetTags return the tags from a payload
func (mp *MetadataPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (mp *MetadataPayload) GetCollectedTime() time.Time {
	return mp.collectedTime
}

// ParseMetadataPayload return the parsed metadata from payload
func ParseMetadataPayload(payload api.Payload) (metadataPayloads []*MetadataPayload, err error) {
	if bytes.Equal(payload.Data, []byte("{}")) {
		// metadata can submit empty JSON object
		return []*MetadataPayload{}, nil
	}

	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}

	var metadataPayload = &MetadataPayload{
		collectedTime: payload.Timestamp,
	}
	json.Unmarshal(enflated, metadataPayload)

	return []*MetadataPayload{metadataPayload}, nil
}

// MetadataAggregator is a type that represents a metadata aggregator
type MetadataAggregator struct {
	Aggregator[*MetadataPayload]
}

// NewMetadataAggregator returns a new metadata aggregator
func NewMetadataAggregator() MetadataAggregator {
	return MetadataAggregator{
		Aggregator: newAggregator(ParseMetadataPayload),
	}
}
