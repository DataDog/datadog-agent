// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"encoding/json"
)

type root struct {
	Aggregator aggregator `json:"aggregator"`
}

type aggregator struct {
	Metrics []metric `json:"metrics"`
}

type metric struct {
	Host           string   `json:"host"`
	Interval       int      `json:"interval"`
	Metric         string   `json:"metric"`
	Points         [][]int  `json:"points"`
	SourceTypeName string   `json:"source_type_name"`
	Tags           []string `json:"tags"`
	Type           string   `json:"type"`
}

func parseCheckOutput(check []byte) []root {
	var data []root
	if err := json.Unmarshal([]byte(check), &data); err != nil {
		return nil
	}

	return data
}
