// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api provides test helpers to interact with the Datadog API
package api

import (
	"errors"

	"github.com/DataDog/datadog-api-client-go/api/v2/datadog"
)

func (c *Client) listLogs(query string) (*datadog.LogsListResponse, error) {
	sort := datadog.LOGSSORT_TIMESTAMP_ASCENDING
	body := datadog.LogsListRequest{
		Filter: &datadog.LogsQueryFilter{
			From:  datadog.PtrString("now-15m"),
			Query: &query,
			To:    datadog.PtrString("now"),
		},
		Page: &datadog.LogsListRequestPage{
			Limit: datadog.PtrInt32(25),
		},
		Sort: &sort,
	}
	request := datadog.ListLogsOptionalParameters{
		Body: &body,
	}

	result, r, err := c.api.LogsApi.ListLogs(c.ctx, request)
	if r != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ErrNoLogFound is returned when no log is found
var ErrNoLogFound = errors.New("no log found")

func (c *Client) getLastMatchingLog(query string) (*datadog.LogAttributes, error) {
	resp, err := c.listLogs(query)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) > 0 {
		return resp.Data[len(resp.Data)-1].Attributes, nil
	}
	return nil, ErrNoLogFound
}
