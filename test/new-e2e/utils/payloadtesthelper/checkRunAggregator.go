// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package payloadtesthelper

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
	checkRunByName map[string][]*checkRun
}

func NewCheckRunAggregator() CheckRunAggregator {
	return CheckRunAggregator{
		checkRunByName: map[string][]*checkRun{},
	}
}

func (agg *CheckRunAggregator) UnmarshallPayloads(body []byte) error {
	checks, err := unmarshallPayloads(body, parseCheckRunPayload)
	if err != nil {
		return err
	}
	agg.checkRunByName = checks
	return nil
}

func (agg *CheckRunAggregator) ContainsCheckName(name string) bool {
	_, found := agg.checkRunByName[name]
	return found
}

func (agg *CheckRunAggregator) ContainsCheckNameAndTags(name string, tags []string) bool {
	checks, found := agg.checkRunByName[name]
	if !found {
		return false
	}

	for _, serie := range checks {
		if areTagsSubsetOfOtherTags(tags, serie.Tags) {
			return true
		}
	}

	return false
}
