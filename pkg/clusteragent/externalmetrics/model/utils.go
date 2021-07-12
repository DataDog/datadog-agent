// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package model

import (
	"strconv"
)

func parseDatadogMetricValue(s string) (float64, error) {
	if len(s) == 0 {
		return 0, nil
	}

	return strconv.ParseFloat(s, 64)
}

func formatDatadogMetricValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
