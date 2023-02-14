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

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type Client struct {
	fakeIntakeURL string
}

// NewClient creates a new fake intake client
// fakeIntakeURL: the host of the fake Datadog intake server
func NewClient(fakeIntakeURL string) *Client {
	return &Client{
		fakeIntakeURL: strings.TrimSuffix(fakeIntakeURL, "/"),
	}
}

func (c *Client) GetFakePayloads(endpoint string) (rawPayloads [][]byte, err error) {
	resp, err := http.Get(fmt.Sprintf("%s/fakeintake/payloads?endpoint=%s", c.fakeIntakeURL, endpoint))
	if err != nil {
		return nil, err
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
