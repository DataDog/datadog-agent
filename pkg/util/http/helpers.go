// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// Get is a high level helper to query an url and return its body as a string
func Get(ctx context.Context, url string, headers map[string]string, timeout time.Duration) (string, error) {
	client := http.Client{
		Transport: CreateHTTPTransport(),
		Timeout:   timeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	for header, value := range headers {
		req.Header.Add(header, value)
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code %d trying to GET %s", res.StatusCode, url)
	}

	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("error while reading response from oraclecloud metadata endpoint: %s", err)
	}

	return string(all), nil
}
