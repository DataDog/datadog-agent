// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type Log struct {
	Message   string   `json:"message"`
	Status    string   `json:"status"`
	Timestamp int      `json:"timestamp"`
	HostName  string   `json:"hostname"`
	Service   string   `json:"service"`
	Source    string   `json:"source"`
	Tags      []string `json:"tags"`
}

func (l *Log) name() string {
	return l.Service
}

func (l *Log) GetTags() []string {
	return l.Tags
}

func parseLogPayload(payload api.Payload) (logs []*Log, err error) {
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
	return logs, err
}

type LogAggregator struct {
	Aggregator[*Log]
}

func NewLogAggregator() LogAggregator {
	return LogAggregator{
		Aggregator: newAggregator(parseLogPayload),
	}
}
