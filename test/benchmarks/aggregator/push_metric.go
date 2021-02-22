// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"

	log "github.com/cihub/seelog"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

func createMetric(value float64, tags []string, name string, t int64) datadog.Metric {
	unit := "millisecond"
	metricType := "gauge"
	hostname, _ := os.Hostname()

	return datadog.Metric{
		Metric: &name,
		Points: []datadog.DataPoint{{float64(t), value}},
		Type:   &metricType,
		Host:   &hostname,
		Tags:   tags,
		Unit:   &unit,
	}
}

func pushMetricsToDatadog(apiKey string, results []datadog.Metric) {
	client := datadog.NewClient(apiKey, "")
	err := client.PostMetrics(results)
	if err != nil {
		log.Errorf("Could not post metrics: %s", err)
	}
}
