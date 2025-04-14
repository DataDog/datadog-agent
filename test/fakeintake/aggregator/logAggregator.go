// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type tags []string

// Log represents a log payload
type Log struct {
	collectedTime time.Time
	Message       string `json:"message"`
	Status        string `json:"status"`
	Timestamp     int    `json:"timestamp"`
	HostName      string `json:"hostname"`
	Service       string `json:"service"`
	Source        string `json:"ddsource"`
	Tags          tags   `json:"ddtags"`
}

func (t *tags) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*t = tags(strings.Split(s, ","))
	return nil
}

func (t tags) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.Join(t, ","))
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
	fmt.Println("ANDREWQIAN ParseLogPayload", payload)
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		fmt.Println("ANDREWQIAN ParseLogPayload ERROR???", len(payload.Data), bytes.Equal(payload.Data, []byte("{}")))
		// logs can submit with empty data or empty JSON object
		return []*Log{}, nil
	}
	fmt.Println("ANDREWQIAN ParseLogPayload 2")
	inflated, err := inflate(payload.Data, payload.Encoding)
	fmt.Println("ANDREWQIAN ParseLogPayload 3", inflated, err)
	if err != nil {
		fmt.Println("ANDREWQIAN ParseLogPayload ERROR 2", err)
		return nil, err
	}
	logs = []*Log{}
	err = json.Unmarshal(inflated, &logs)
	fmt.Println("ANDREWQIAN ParseLogPayload 4", logs, err)
	if err != nil {
		fmt.Println("ANDREWQIAN ParseLogPayload ERROR 3", err)
		return nil, err
	}
	for _, l := range logs {
		l.collectedTime = payload.Timestamp
	}
	fmt.Println("ANDREWQIAN ParseLogPayload 5", logs, err)
	return logs, err
}

// LogAggregator is an aggregator for logs
type LogAggregator struct {
	Aggregator[*Log]
}

// NewLogAggregator returns a new LogAggregator
func NewLogAggregator() LogAggregator {
	return LogAggregator{
		Aggregator: newAggregator(ParseLogPayload),
	}
}
