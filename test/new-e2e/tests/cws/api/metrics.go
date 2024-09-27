// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api provides test helpers to interact with the Datadog API
package api

import (
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

// QueryMetric queries the timeseries using the specified query
func (c *Client) QueryMetric(query string) (*datadogV1.MetricsQueryResponse, error) {
	api := datadogV1.NewMetricsApi(c.api)
	now := time.Now()
	resp, r, err := api.QueryMetrics(c.ctx, now.Add(-1*time.Minute).Unix(), now.Unix(), query)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
