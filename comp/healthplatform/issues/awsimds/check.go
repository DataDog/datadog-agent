// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package awsimds

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

// Separate timeouts keep connection failures distinct from dropped IMDS responses.
var (
	dialTimeout     = 2 * time.Second
	responseTimeout = 2 * time.Second
)

// metadataURL is the IMDSv1 GET endpoint, derived from the shared token URL.
var metadataURL = strings.Replace(ec2.TokenURL, "/latest/api/token", "/latest/meta-data/instance-id", 1)

// probe reports true when the agent cannot retrieve EC2 metadata over IMDS.
func probe() (bool, error) {
	// A failed handshake is not the hop-limit symptom (non-AWS host, refused, etc.).
	conn, err := net.DialTimeout("tcp", imdsAddress, dialTimeout)
	if err != nil {
		return false, nil
	}
	conn.Close()

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, network, imdsAddress)
			},
			ResponseHeaderTimeout: responseTimeout,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()

	// An IMDSv1 GET reflects what the agent retrieves by default; its response is not
	// subject to the IMDSv2 hop limit, so a success here means metadata is available.
	status, err := probeStatus(client, http.MethodGet, metadataURL, nil)
	if err != nil {
		return isTimeout(err), nil
	}
	if status == http.StatusOK {
		return false, nil
	}

	// IMDSv1 is unavailable (HttpTokens=required, or blocked by Kube2IAM/kiam): metadata
	// now depends on the IMDSv2 token, whose PUT is dropped by a low hop limit and rejected
	// (401/403) by an IMDS-blocking intermediary.
	status, err = probeStatus(client, http.MethodPut, ec2.TokenURL, map[string]string{ec2.TokenTTLHeader: "21600"})
	if err != nil {
		return isTimeout(err), nil
	}
	// Only a successful token response means the agent can obtain metadata.
	return status != http.StatusOK, nil
}

// probeStatus issues one request and returns its status code, closing the body.
func probeStatus(client *http.Client, method, url string, headers map[string]string) (int, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// isTimeout reports whether err is a network timeout (dropped response or unreachable host).
func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
