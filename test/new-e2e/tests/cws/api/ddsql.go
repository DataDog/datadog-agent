// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// JSONAPIPayload is a struct that represents the body of a JSON API request
type JSONAPIPayload[Attr any] struct {
	Data JSONAPIPayloadData[Attr] `json:"data"`
}

// JSONAPIPayloadData is a struct that represents the data field of a JSON API request
type JSONAPIPayloadData[Attr any] struct {
	Type      string `json:"type"`
	Attribute Attr   `json:"attributes"`
}

// DDSQLTableQueryParams is a struct that represents a DDSQL table query
type DDSQLTableQueryParams struct {
	DefaultStart    int    `json:"default_start"`
	DefaultEnd      int    `json:"default_end"`
	DefaultInterval int    `json:"default_interval"`
	Query           string `json:"query"`
}

// DDSQLTableResponse is a struct that represents a DDSQL table response
type DDSQLTableResponse struct {
	Data []DataEntry `json:"data"`
}

// DataEntry is a struct that represents a data entry in a DDSQL table response
type DataEntry struct {
	Type       string     `json:"type"`
	Attributes Attributes `json:"attributes"`
}

// Attributes is a struct that represents the attributes of a data entry in a DDSQL table response
type Attributes struct {
	Columns []Column `json:"columns"`
}

// Column is a struct that represents a column of a data entry in a DDSQL table response
type Column struct {
	Name   string        `json:"name"`
	Type   string        `json:"type"`
	Values []interface{} `json:"values"`
}

// DDSQLClient is a struct that represents a DDSQL client
type DDSQLClient struct {
	http   http.Client
	apiKey string
	appKey string
}

// NewDDSQLClient returns a new DDSQL client
func NewDDSQLClient(apiKey, appKey string) *DDSQLClient {
	return &DDSQLClient{
		http:   http.Client{},
		apiKey: apiKey,
		appKey: appKey,
	}
}

// Do executes a DDSQL query, returning a DDSQL table response
func (c *DDSQLClient) Do(query string) (*DDSQLTableResponse, error) {
	now := time.Now()
	params := DDSQLTableQueryParams{
		DefaultStart:    int(now.Add(-1 * time.Hour).UnixMilli()),
		DefaultEnd:      int(now.UnixMilli()),
		DefaultInterval: 20000,
		Query:           query,
	}
	payload := JSONAPIPayload[DDSQLTableQueryParams]{
		Data: JSONAPIPayloadData[DDSQLTableQueryParams]{
			Type:      "ddsql_table_request",
			Attribute: params,
		},
	}

	reqData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://app.datadoghq.com/api/v2/ddql/table", bytes.NewBuffer(reqData))
	if err != nil {
		return nil, err
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("content-type", "application/json")
	req.Header.Add("DD-API-KEY", c.apiKey)
	req.Header.Add("DD-APPLICATION-KEY", c.appKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ddSQLResp DDSQLTableResponse
	err = json.Unmarshal(data, &ddSQLResp)
	if err != nil {
		return nil, err
	}

	return &ddSQLResp, nil
}
