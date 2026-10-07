// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"google.golang.org/protobuf/proto"
)

// SDSResultPayload is an sds-result Event Platform payload.
type SDSResultPayload struct {
	sds.SdsResultPayload
	collectedTime time.Time
}

func (p *SDSResultPayload) name() string {
	results := p.GetScanResults()
	if len(results) == 0 {
		return "unknown"
	}
	task := results[0].GetScanMetadata().GetScanTaskMetadata()
	return task.GetTaskId() + ":" + task.GetSubTaskId()
}

// GetTags returns no tags for sds-result payloads.
func (p *SDSResultPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime returns the time when the payload was collected by the fakeintake server.
func (p *SDSResultPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseSDSResult parses an api.Payload into sds-result payloads.
func ParseSDSResult(payload api.Payload) ([]*SDSResultPayload, error) {
	if len(payload.Data) == 0 {
		return []*SDSResultPayload{}, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}

	if len(inflated) == 0 {
		return []*SDSResultPayload{}, nil
	}

	if bytes.Equal(bytes.TrimSpace(inflated), []byte("{}")) {
		return []*SDSResultPayload{}, nil
	}

	result := &SDSResultPayload{
		collectedTime: payload.Timestamp,
	}

	if err := proto.Unmarshal(inflated, &result.SdsResultPayload); err != nil {
		return nil, err
	}

	return []*SDSResultPayload{result}, nil
}

// SDSResultAggregator is an Aggregator for sds-result payloads.
type SDSResultAggregator struct {
	Aggregator[*SDSResultPayload]
}

// NewSDSResultAggregator returns a new SDSResultAggregator.
func NewSDSResultAggregator() SDSResultAggregator {
	return SDSResultAggregator{
		Aggregator: newAggregator(ParseSDSResult),
	}
}
