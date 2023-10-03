// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type Log struct {
	collectedTime time.Time
	Message       string   `json:"message"`
	Status        string   `json:"status"`
	Timestamp     int      `json:"timestamp"`
	HostName      string   `json:"hostname"`
	Service       string   `json:"service"`
	Source        string   `json:"source"`
	Tags          []string `json:"tags"`
}

func (l *Log) name() string {
	return l.Service
}

// GetTags return the tags from a payload
func (l *Log) GetTags() []string {
	return l.Tags
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (l *Log) GetCollectedTime() time.Time {
	return l.collectedTime
}

// ParseLogPayload return the parsed logs from payload
func ParseLogPayload(payload api.Payload) (logs []*Log, err error) {
	if len(payload.Data) == 0 {
		// logs can submit with empty data
		return []*Log{}, nil
	}
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	logs = []*Log{}
	err = json.Unmarshal(enflated, &logs)
	if err != nil {
		return nil, err
	}
	for _, l := range logs {
		l.collectedTime = payload.Timestamp
	}
	return logs, err
}

type LogAggregator struct {
	Aggregator[*Log]
}

func NewLogAggregator() LogAggregator {
	return LogAggregator{
		Aggregator: newAggregator(ParseLogPayload),
	}
}
