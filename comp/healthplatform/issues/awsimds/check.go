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
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

// Separate timeouts keep connection failures distinct from dropped token responses.
var (
	dialTimeout     = 2 * time.Second
	responseTimeout = 2 * time.Second
)

// probe detects a token response timeout after a successful TCP handshake.
func probe() (bool, error) {
	conn, err := net.DialTimeout("tcp", imdsAddress, dialTimeout)
	if err != nil {
		return false, nil
	}
	defer conn.Close()

	req, err := http.NewRequest(http.MethodPut, ec2.TokenURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set(ec2.TokenTTLHeader, "21600")

	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return conn, nil
		},
		ResponseHeaderTimeout: responseTimeout,
		DisableKeepAlives:     true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err == nil {
		// Any HTTP response proves the token endpoint is reachable.
		resp.Body.Close()
		return false, nil
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout(), nil
}
