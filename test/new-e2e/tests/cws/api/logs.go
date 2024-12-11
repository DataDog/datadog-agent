// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api provides test helpers to interact with the Datadog API
package api

import (
	"errors"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

func (c *Client) listLogs(query string) (*datadogV2.LogsListResponse, error) {
	body := datadogV2.LogsListRequest{
		Filter: &datadogV2.LogsQueryFilter{
			From:  datadog.PtrString("now-15m"),
			Query: datadog.PtrString(query),
			To:    datadog.PtrString("now"),
		},
		Page: &datadogV2.LogsListRequestPage{
			Limit: datadog.PtrInt32(25),
		},
		Sort: datadogV2.LOGSSORT_TIMESTAMP_ASCENDING.Ptr(),
	}

	request := datadogV2.NewListLogsOptionalParameters().WithBody(body)
	api := datadogV2.NewLogsApi(c.api)

	result, r, err := api.ListLogs(c.ctx, *request)
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

func (c *Client) getLastMatchingLog(query string) (*datadogV2.LogAttributes, error) {
	resp, err := c.listLogs(query)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) > 0 {
		return resp.Data[len(resp.Data)-1].Attributes, nil
	}
	return nil, ErrNoLogFound
}
