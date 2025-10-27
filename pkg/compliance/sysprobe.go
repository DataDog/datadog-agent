// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance implements a specific part of the datadog-agent
// responsible for scanning host and containers and report various
// misconfigurations and compliance issues.
package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance/dbconfig"
)

// SysProbeClient is an interface for fetching database configuration from system probe
type SysProbeClient interface {
	FetchDBConfig(ctx context.Context, pid int32) (*dbconfig.DBResource, error)
}

type RemoteSysProbeClient struct {
	client *http.Client
}

// NewRemoteSysProbeClient creates a new remote system probe client with the given Unix socket address
func NewRemoteSysProbeClient(address string) *RemoteSysProbeClient {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 30 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", address)
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}

	return &RemoteSysProbeClient{
		client: httpClient,
	}
}

// FetchDBConfig fetches database configuration for the given process ID from the remote system probe
func (c *RemoteSysProbeClient) FetchDBConfig(ctx context.Context, pid int32) (*dbconfig.DBResource, error) {
	qs := make(url.Values)
	qs.Add("pid", strconv.FormatInt(int64(pid), 10))
	sysProbeComplianceModuleURL := &url.URL{
		Scheme:   "http",
		Host:     "unix",
		Path:     "/compliance/dbconfig",
		RawQuery: qs.Encode(),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sysProbeComplianceModuleURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error running cross-container benchmark: %s", resp.Status)
	}

	var resource *dbconfig.DBResource
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &resource); err != nil {
		return nil, err
	}

	return resource, nil
}

var _ SysProbeClient = (*RemoteSysProbeClient)(nil)
