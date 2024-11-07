// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client contains the client for the API exposed by system-probe
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

var (
	// ErrNotImplemented is an error used when system-probe is attempted to be accessed on an unsupported OS
	ErrNotImplemented = errors.New("system-probe unsupported")
)

// Get returns a http client configured to talk to the system-probe
var Get = funcs.MemoizeArgNoError[string, *http.Client](get)

func get(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:          2,
			IdleConnTimeout:       idleConnTimeout,
			DialContext:           DialContextFunc(socketPath),
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}
}

// GetCheck returns data unmarshalled from JSON to T, from the specified module at the /<module>/check endpoint.
func GetCheck[T any](client *http.Client, module types.ModuleName) (T, error) {
	var data T
	req, err := http.NewRequest("GET", URL(module, "/check"), nil)
	if err != nil {
		return data, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return data, fmt.Errorf("conn request failed: url %s, status code: %d", req.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	err = json.Unmarshal(body, &data)
	return data, err
}

// URL constructs a system-probe URL given the specified module and endpoint.
// Do not include any query parameters here.
func URL(module types.ModuleName, endpoint string) string {
	sysurl, _ := url.JoinPath("http://sysprobe", string(module), endpoint)
	return sysurl
}

// ReadAllResponseBody reads the entire HTTP response body as a byte slice
func ReadAllResponseBody(resp *http.Response) ([]byte, error) {
	// if we are not able to determine the content length
	// we read the whole body without pre-allocation
	if resp.ContentLength <= 0 {
		return io.ReadAll(resp.Body)
	}

	// if we know the content length we pre-allocate the buffer
	var buf bytes.Buffer
	buf.Grow(int(resp.ContentLength))

	_, err := buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
