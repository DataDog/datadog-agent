// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type CheckRun struct {
	Check     string   `json:"check"`
	HostName  string   `json:"host_name"`
	Timestamp int      `json:"timestamp"`
	Status    int      `json:"status"`
	Message   string   `json:"message"`
	Tags      []string `json:"tags"`
}

func (cr *CheckRun) name() string {
	return cr.Check
}

func (cr *CheckRun) GetTags() []string {
	return cr.Tags
}

func parseCheckRunPayload(payload api.Payload) (checks []*CheckRun, err error) {
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	checks = []*CheckRun{}
	err = json.Unmarshal(enflated, &checks)
	return checks, err
}

type CheckRunAggregator struct {
	Aggregator[*CheckRun]
}

func NewCheckRunAggregator() CheckRunAggregator {
	return CheckRunAggregator{
		Aggregator: newAggregator(parseCheckRunPayload),
	}
}
