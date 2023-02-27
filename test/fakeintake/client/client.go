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
	"strings"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type Client struct {
	fakeIntakeURL string

	metricAggregator   aggregator.MetricAggregator
	checkRunAggregator aggregator.CheckRunAggregator
}

// NewClient creates a new fake intake client
// fakeIntakeURL: the host of the fake Datadog intake server
func NewClient(fakeIntakeURL string) *Client {
	return &Client{
		fakeIntakeURL:      strings.TrimSuffix(fakeIntakeURL, "/"),
		metricAggregator:   aggregator.NewMetricAggregator(),
		checkRunAggregator: aggregator.NewCheckRunAggregator(),
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

func (c *Client) getFakePayloads(endpoint string) (rawPayloads [][]byte, err error) {
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
