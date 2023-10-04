// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client implements helpers APIs to interact with a [fakeintake server] from go tests
// Helpers fetch fakeintake endpoints, unpackage payloads and store parsed data in aggregators
//
// # Using fakeintake in go tests
//
// In this example we assert that a fakeintake running at localhost on port 8080 received
// "system.uptime" metrics with tags "app:system" and values in range 4226000 < value < 4226050.
//
//	client := NewClient("http://localhost:8080")
//	metrics, err := client.FilterMetrics("system.uptime",
//			WithTags[*aggregator.MetricSeries]([]string{"app:system"}),
//			WithMetricValueInRange(4226000, 4226050))
//	assert.NoError(t, err)
//	assert.NotEmpty(t, metrics)
//
// In this example we assert that a fakeintake running at localhost on port 8080 received
// logs by service "system" with tags "app:system" and content containing "totoro"
//
//	client := NewClient("http://localhost:8080")
//	logs, err := client.FilterLogs("system",
//			WithTags[*aggregator.Log]([]string{"totoro"}),
//	assert.NoError(t, err)
//	assert.NotEmpty(t, logs)
//
// In this example we assert that a fakeintake running at localhost on port 8080 received
// check runs by name "totoro" with tags "status:ok"
//
//	client := NewClient("http://localhost:8080")
//	logs, err := client.GetCheckRun("totoro")
//	assert.NoError(t, err)
//	assert.NotEmpty(t, logs)
//
// [fakeintake server]: https://pkg.go.dev/github.com/DataDog/datadog-agent@main/test/fakeintake/server
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/client/flare"
)

type Client struct {
	fakeIntakeURL string

	metricAggregator     aggregator.MetricAggregator
	checkRunAggregator   aggregator.CheckRunAggregator
	logAggregator        aggregator.LogAggregator
	connectionAggregator aggregator.ConnectionsAggregator
}

// NewClient creates a new fake intake client
// fakeIntakeURL: the host of the fake Datadog intake server
func NewClient(fakeIntakeURL string) *Client {
	return &Client{
		fakeIntakeURL:        strings.TrimSuffix(fakeIntakeURL, "/"),
		metricAggregator:     aggregator.NewMetricAggregator(),
		checkRunAggregator:   aggregator.NewCheckRunAggregator(),
		logAggregator:        aggregator.NewLogAggregator(),
		connectionAggregator: aggregator.NewConnectionsAggregator(),
	}
}

func (c *Client) getMetrics() error {
	payloads, err := c.getFakePayloads("/api/v2/series")
	if err != nil {
		return err
	}
	return c.metricAggregator.UnmarshallPayloads(payloads)
}

func (c *Client) getCheckRuns() error {
	payloads, err := c.getFakePayloads("/api/v1/check_run")
	if err != nil {
		return err
	}
	return c.checkRunAggregator.UnmarshallPayloads(payloads)
}

func (c *Client) getLogs() error {
	payloads, err := c.getFakePayloads("/api/v2/logs")
	if err != nil {
		return err
	}
	return c.logAggregator.UnmarshallPayloads(payloads)
}

func (c *Client) getConnections() error {
	payloads, err := c.getFakePayloads("/api/v1/connections")
	if err != nil {
		return err
	}
	return c.connectionAggregator.UnmarshallPayloads(payloads)
}

// GetLatestFlare queries the Fake Intake to fetch flares that were sent by a Datadog Agent and returns the latest flare as a Flare struct
// TODO: handle multiple flares / flush when returning latest flare
func (c *Client) GetLatestFlare() (flare.Flare, error) {
	payloads, err := c.getFakePayloads("/support/flare")
	if err != nil {
		return flare.Flare{}, err
	}

	if len(payloads) == 0 {
		return flare.Flare{}, errors.New("no flare available")
	}

	return flare.ParseRawFlare(payloads[len(payloads)-1])
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
	var response api.APIFakeIntakePayloadsRawGETResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	return response.Payloads, nil
}

// GetServerHealth fetches fakeintake health status and returns an error if
// fakeintake is unhealthy
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

func (c *Client) getMetric(name string) ([]*aggregator.MetricSeries, error) {
	err := c.getMetrics()
	if err != nil {
		return nil, err
	}
	return c.metricAggregator.GetPayloadsByName(name), nil
}

// A MatchOpt to filter fakeintake payloads
type MatchOpt[P aggregator.PayloadItem] func(payload P) (bool, error)

// GetMetricNames fetches fakeintake on `/api/v2/series` endpoint and returns
// all received metric names
func (c *Client) GetMetricNames() ([]string, error) {
	err := c.getMetrics()
	if err != nil {
		return []string{}, nil
	}
	return c.metricAggregator.GetNames(), nil
}

// FilterMetrics fetches fakeintake on `/api/v2/series` endpoint and returns
// metrics matching `name` and any [MatchOpt](#MatchOpt) options
func (c *Client) FilterMetrics(name string, options ...MatchOpt[*aggregator.MetricSeries]) ([]*aggregator.MetricSeries, error) {
	metrics, err := c.getMetric(name)
	if err != nil {
		return nil, err
	}
	// apply filters one after the other
	filteredMetrics := []*aggregator.MetricSeries{}
	for _, metric := range metrics {
		matchCount := 0
		for _, matchOpt := range options {
			isMatch, err := matchOpt(metric)
			if err != nil {
				return nil, err
			}
			if !isMatch {
				break
			}
			matchCount++
		}
		if matchCount == len(options) {
			filteredMetrics = append(filteredMetrics, metric)
		}
	}
	return filteredMetrics, nil
}

// WithTags filters by `tags`
func WithTags[P aggregator.PayloadItem](tags []string) MatchOpt[P] {
	return func(payload P) (bool, error) {
		if aggregator.AreTagsSubsetOfOtherTags(tags, payload.GetTags()) {
			return true, nil
		}
		// TODO return similarity error score
		return false, nil
	}
}

// WithMetricValueInRange filters metrics with values in range `minValue < value < maxValue`
func WithMetricValueInRange(minValue float64, maxValue float64) MatchOpt[*aggregator.MetricSeries] {
	return func(metric *aggregator.MetricSeries) (bool, error) {
		isMatch, err := WithMetricValueHigherThan(minValue)(metric)
		if !isMatch || err != nil {
			return isMatch, err
		}
		return WithMetricValueLowerThan(maxValue)(metric)
	}
}

// WithMetricValueLowerThan filters metrics with values lower than `maxValue`
func WithMetricValueLowerThan(maxValue float64) MatchOpt[*aggregator.MetricSeries] {
	return func(metric *aggregator.MetricSeries) (bool, error) {
		for _, point := range metric.Points {
			if point.Value < maxValue {
				return true, nil
			}
		}
		// TODO return similarity error score
		return false, nil
	}
}

// WithMetricValueLowerThan filters metrics with values higher than `minValue`
func WithMetricValueHigherThan(minValue float64) MatchOpt[*aggregator.MetricSeries] {
	return func(metric *aggregator.MetricSeries) (bool, error) {
		for _, point := range metric.Points {
			if point.Value > minValue {
				return true, nil
			}
		}
		// TODO return similarity error score
		return false, nil
	}
}

func (c *Client) getLog(service string) ([]*aggregator.Log, error) {
	err := c.getLogs()
	if err != nil {
		return nil, err
	}
	return c.logAggregator.GetPayloadsByName(service), nil
}

// GetLogNames fetches fakeintake on `/api/v2/logs` endpoint and returns
// all received log service names
func (c *Client) GetLogServiceNames() ([]string, error) {
	err := c.getLogs()
	if err != nil {
		return []string{}, nil
	}
	return c.logAggregator.GetNames(), nil
}

// FilterLogs fetches fakeintake on `/api/v2/logs` endpoint, unpackage payloads and returns
// logs matching `service` and any [MatchOpt](#MatchOpt) options
func (c *Client) FilterLogs(service string, options ...MatchOpt[*aggregator.Log]) ([]*aggregator.Log, error) {
	logs, err := c.getLog(service)
	if err != nil {
		return nil, err
	}
	// apply filters one after the other
	filteredLogs := []*aggregator.Log{}
	for _, log := range logs {
		matchCount := 0
		for _, matchOpt := range options {
			isMatch, err := matchOpt(log)
			if err != nil {
				return nil, err
			}
			if !isMatch {
				break
			}
			matchCount++
		}
		if matchCount == len(options) {
			filteredLogs = append(filteredLogs, log)
		}
	}
	return filteredLogs, nil
}

// WithMessageContaining filters logs by message containing `content`
func WithMessageContaining(content string) MatchOpt[*aggregator.Log] {
	return func(log *aggregator.Log) (bool, error) {
		if strings.Contains(log.Message, content) {
			return true, nil
		}
		// TODO return similarity score in error
		return false, nil
	}
}

// WithMessageMatching filters logs by message matching [regexp](https://pkg.go.dev/regexp) `pattern`
func WithMessageMatching(pattern string) MatchOpt[*aggregator.Log] {
	return func(log *aggregator.Log) (bool, error) {
		matched, err := regexp.MatchString(pattern, log.Message)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
		// TODO return similarity score in error
		return false, nil
	}
}

// GetCheckRunNames fetches fakeintake on `/api/v1/check_run` endpoint and returns
// all received check run names
func (c *Client) GetCheckRunNames() ([]string, error) {
	err := c.getCheckRuns()
	if err != nil {
		return []string{}, nil
	}
	return c.checkRunAggregator.GetNames(), nil
}

// FilterLogs fetches fakeintake on `/api/v1/check_run` endpoint, unpackage payloads and returns
// checks matching `name`
func (c *Client) GetCheckRun(name string) ([]*aggregator.CheckRun, error) {
	err := c.getCheckRuns()
	if err != nil {
		return nil, err
	}
	return c.checkRunAggregator.GetPayloadsByName(name), nil
}

// FlushServerAndResetAggregators sends a request to delete any stored payload
// and resets client's  aggregators
// Call this in between tests to reset the fakeintake status on both client and server side
func (c *Client) FlushServerAndResetAggregators() error {
	err := c.flushPayloads()
	if err != nil {
		return err
	}
	c.checkRunAggregator.Reset()
	c.metricAggregator.Reset()
	c.logAggregator.Reset()
	return nil
}

func (c *Client) flushPayloads() error {
	resp, err := http.Get(fmt.Sprintf("%s/fakeintake/flushPayloads", c.fakeIntakeURL))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error code %v", resp.StatusCode)
	}
	return nil
}

// GetConnections fetches fakeintake on `/api/v1/connections` endpoint and returns
// all received connections
func (c *Client) GetConnections() (conns *aggregator.ConnectionsAggregator, err error) {
	err = c.getConnections()
	if err != nil {
		return nil, err
	}
	return &c.connectionAggregator, nil
}

// GetConnectionsNames fetches fakeintake on `/api/v1/connections` endpoint and returns
// all received connections from hostname+network_id
func (c *Client) GetConnectionsNames() ([]string, error) {
	err := c.getConnections()
	if err != nil {
		return []string{}, err
	}
	return c.connectionAggregator.GetNames(), nil
}
