// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

type metricType int

const (
	unknownMetricType metricType = iota
	gauge
	count
	rate
	monotonicCount
	histogram
	historate
)

type metricSender func(string, float64, string, []string)

func getMetricFunction(sender sender.Sender, method metricType) (metricSender, error) {
	if sender == nil {
		return nil, fmt.Errorf("sender is nil")
	}
	methods := map[metricType]metricSender{
		gauge:          sender.Gauge,
		count:          sender.Count,
		rate:           sender.Rate,
		monotonicCount: sender.MonotonicCount,
		histogram:      sender.Histogram,
		historate:      sender.Historate,
	}
	if val, ok := methods[method]; ok {
		return val, nil
	}
	return nil, fmt.Errorf("Invalid metric function code %d", method)
}

func getMetricFunctionCode(name string) (metricType, error) {
	metricTypes := map[string]metricType{
		"gauge":           gauge,
		"count":           count,
		"rate":            rate,
		"monotonic_count": monotonicCount,
		"histogram":       histogram,
		"historate":       historate,
	}
	if val, ok := metricTypes[name]; ok {
		return val, nil
	}
	return unknownMetricType, fmt.Errorf("unknown metric type: %s", name)
}

func sendMetric(c *Check, method metricType, metric string, value float64, tags []string) {
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("%s failed to get metric sender %s", err)
	}
	metricFunction, err := getMetricFunction(sender, method)
	if err != nil {
		log.Errorf("failed to get metric function: %s", err)
	}
	metricFunction(metric, value, c.dbHostname, tags)
}

func sendMetricWithDefaultTags(c *Check, method metricType, metric string, value float64) {
	sendMetric(c, method, metric, value, c.tags)
}

func sendServiceCheck(c *Check, service string, status servicecheck.ServiceCheckStatus, message string) {
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("%s failed to get metric sender %s", err)
		return
	}

	sender.ServiceCheck(service, status, c.dbHostname, c.tags, message)
}

func commit(c *Check) {
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("%s failed to get metric sender %s", err)
		return
	}
	sender.Commit()
}
