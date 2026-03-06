// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
		return nil, errors.New("sender is nil")
	}
	switch method {
	case gauge:
		return sender.Gauge, nil
	case count:
		return sender.Count, nil
	case rate:
		return sender.Rate, nil
	case monotonicCount:
		return sender.MonotonicCount, nil
	case histogram:
		return sender.Histogram, nil
	case historate:
		return sender.Historate, nil
	default:
		return nil, fmt.Errorf("Invalid metric function code %d", method)
	}
}

func getMetricFunctionCode(name string) (metricType, error) {
	switch name {
	case "gauge":
		return gauge, nil
	case "count":
		return count, nil
	case "rate":
		return rate, nil
	case "monotonic_count":
		return monotonicCount, nil
	case "histogram":
		return histogram, nil
	case "historate":
		return historate, nil
	default:
		return unknownMetricType, fmt.Errorf("unknown metric type: %s", name)
	}
}

func sendMetric(c *Check, method metricType, metric string, value float64, tags []string) {
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("failed to get metric sender: %s", err)
		return
	}
	metricFunction, err := getMetricFunction(sender, method)
	if err != nil {
		log.Errorf("failed to get metric function: %s", err)
		return
	}
	metricFunction(metric, value, c.dbHostname, tags)
}

func sendMetricWithTimestamp(c *Check, method metricType, metric string, value float64, tags []string, timestamp float64) {
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("failed to get metric sender: %s", err)
		return
	}
	switch method {
	case gauge:
		if err := sender.GaugeWithTimestamp(metric, value, c.dbHostname, tags, timestamp); err != nil {
			log.Errorf("failed to send gauge with timestamp: %s", err)
		}
	case count:
		if err := sender.CountWithTimestamp(metric, value, c.dbHostname, tags, timestamp); err != nil {
			log.Errorf("failed to send count with timestamp: %s", err)
		}
	default:
		log.Warnf("metric_timestamp is not supported for metric type of %s, falling back to submission without timestamp", metric)
		sendMetric(c, method, metric, value, tags)
	}
}

func sendMetricWithDefaultTags(c *Check, method metricType, metric string, value float64) {
	sendMetric(c, method, metric, value, c.tags)
}

func sendServiceCheck(c *Check, service string, status servicecheck.ServiceCheckStatus, message string) {
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("failed to get metric sender: %s", err)
		return
	}

	sender.ServiceCheck(service, status, c.dbHostname, c.tags, message)
}

func commit(c *Check) {
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("failed to get metric sender: %s", err)
		return
	}
	sender.Commit()
}
