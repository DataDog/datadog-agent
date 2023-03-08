// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type Client struct {
	fakeIntakeURL string

	metricAggregator   aggregator.MetricAggregator
	checkRunAggregator aggregator.CheckRunAggregator
	logAggregator      aggregator.LogAggregator
}

// NewClient creates a new fake intake client
// fakeIntakeURL: the host of the fake Datadog intake server
func NewClient(fakeIntakeURL string) *Client {
	return &Client{
		fakeIntakeURL:      strings.TrimSuffix(fakeIntakeURL, "/"),
		metricAggregator:   aggregator.NewMetricAggregator(),
		checkRunAggregator: aggregator.NewCheckRunAggregator(),
		logAggregator:      aggregator.NewLogAggregator(),
	}
}

func (c *Client) getMetrics() error {
	payloads, err := c.getFakePayloads("api/v2/metrics")
	if err != nil {
		return err
	}
	return c.metricAggregator.UnmarshallPayloads(payloads)
}

func (c *Client) getCheckRuns() error {
	payloads, err := c.getFakePayloads("api/v1/check_run")
	if err != nil {
		return err
	}
	return c.checkRunAggregator.UnmarshallPayloads(payloads)
}

func (c *Client) getLogs() error {
	payloads, err := c.getFakePayloads("api/v2/logs")
	if err != nil {
		return err
	}
	return c.logAggregator.UnmarshallPayloads(payloads)
}

func (c *Client) getFakePayloads(endpoint string) (rawPayloads []api.Payload, err error) {
	resp, err := http.Get(fmt.Sprintf("%s/fakeintake/payloads?endpoint=%s", c.fakeIntakeURL, endpoint))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error querying fake payloads, status code %s", resp.Status)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var response api.APIFakeIntakePayloadsGETResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	return response.Payloads, nil
}

func (c *Client) GetServerHealth() error {
	resp, err := http.Get(fmt.Sprintf("%s/fakeintake/health", c.fakeIntakeURL))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error code %v", resp.StatusCode)
	}
	return nil
}

func (c *Client) GetMetric(name string) ([]*aggregator.MetricSeries, error) {
	err := c.getMetrics()
	if err != nil {
		return nil, err
	}
	return c.metricAggregator.GetPayloadsByName(name), nil
}

type MatchOpt[P aggregator.PayloadItem] func(payloads []P) []P

func WithTags[P aggregator.PayloadItem](tags []string) MatchOpt[P] {
	return func(payloads []P) []P {
		var ret []P
		for _, payload := range payloads {
			if aggregator.AreTagsSubsetOfOtherTags(tags, payload.GetTags()) {
				ret = append(ret, payload)
			}
		}
		return ret
	}
}

func WithMetricValueLowerThan(minValue float64) MatchOpt[*aggregator.MetricSeries] {
	return func(metrics []*aggregator.MetricSeries) []*aggregator.MetricSeries {
		var ret []*aggregator.MetricSeries
		for _, metric := range metrics {
			for _, point := range metric.Points {
				if point.Value < minValue {
					ret = append(ret, metric)
				}
			}
		}
		return ret
	}
}

func WithMetricValueHigherThan(maxValue float64) MatchOpt[*aggregator.MetricSeries] {
	return func(metrics []*aggregator.MetricSeries) []*aggregator.MetricSeries {
		var ret []*aggregator.MetricSeries
		for _, metric := range metrics {
			for _, point := range metric.Points {
				if point.Value > maxValue {
					ret = append(ret, metric)
				}
			}
		}
		return ret
	}
}

func (c *Client) FilterMetrics(name string, options ...MatchOpt[*aggregator.MetricSeries]) ([]*aggregator.MetricSeries, error) {
	metrics, err := c.GetMetric(name)
	if err != nil {
		return nil, err
	}
	// apply filters one after the other
	for _, matchOpt := range options {
		metrics = matchOpt(metrics)
	}
	return metrics, nil
}

func (c *Client) GetLog(name string) ([]*aggregator.Log, error) {
	err := c.getLogs()
	if err != nil {
		return nil, err
	}
	return c.logAggregator.GetPayloadsByName(name), nil
}

func WithMessageContaining(content string) MatchOpt[*aggregator.Log] {
	return func(logs []*aggregator.Log) []*aggregator.Log {
		var ret []*aggregator.Log
		for _, log := range logs {
			if strings.Contains(log.Message, content) {
				ret = append(ret, log)
			}
		}
		return ret
	}
}

func WithMessageMatching(pattern string) MatchOpt[*aggregator.Log] {
	return func(logs []*aggregator.Log) []*aggregator.Log {
		var ret []*aggregator.Log
		for _, log := range logs {
			matched, err := regexp.MatchString(pattern, log.Message)
			if err != nil {
				fmt.Printf("error matching log message %s with pattern %s: %v", log.Message, pattern, err)
				continue
			}
			if matched {
				ret = append(ret, log)
			}
		}
		return ret
	}
}

func (c *Client) FilterLogs(name string, options ...MatchOpt[*aggregator.Log]) ([]*aggregator.Log, error) {
	logs, err := c.GetLog(name)
	if err != nil {
		return nil, err
	}
	// apply filters one after the other
	for _, matchOpt := range options {
		logs = matchOpt(logs)
	}
	return logs, nil
}

func (c *Client) GetCheckRun(name string) ([]*aggregator.CheckRun, error) {
	err := c.getCheckRuns()
	if err != nil {
		return nil, err
	}
	return c.checkRunAggregator.GetPayloadsByName(name), nil
}
