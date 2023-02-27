// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
)

type checkRun struct {
	Check     string   `json:"check"`
	HostName  string   `json:"host_name"`
	Timestamp int      `json:"timestamp"`
	Status    int      `json:"status"`
	Message   string   `json:"message"`
	Tags      []string `json:"tags"`
}

func (cr *checkRun) name() string {
	return cr.Check
}

func (cr *checkRun) tags() []string {
	return cr.Tags
}

func parseCheckRunPayload(data []byte) (checks []*checkRun, err error) {
	enflated, err := enflate(data)
	if err != nil {
		return nil, err
	}
	checks = []*checkRun{}
	err = json.Unmarshal(enflated, &checks)
	return checks, err
}

type CheckRunAggregator struct {
	Aggregator[*checkRun]
}

func NewCheckRunAggregator() CheckRunAggregator {
	return CheckRunAggregator{
		Aggregator: newAggregator(parseCheckRunPayload),
	}
}
