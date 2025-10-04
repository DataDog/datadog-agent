// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package httpclient provides a HTTP client that can be used to make HTTP requests
package httpclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

const (
	httpRequestAuthTimeout = time.Second * 30
	minTLSVersion          = tls.VersionTLS12
)

// RunnerHTTPClient is a client that can be used to make HTTP requests
type RunnerHTTPClient struct{}

// RunnerHTTPClientConfig is a configuration for the RunnerHTTPClient
type RunnerHTTPClientConfig struct {
	MaxRedirect        int
	Transport          *RunnerHTTPTransportConfig
	AllowIMDSEndpoints bool
}

// RunnerHTTPTransportConfig is a configuration for the RunnerHTTPClient transport
type RunnerHTTPTransportConfig struct {
	InsecureSkipVerify bool
}

// NewRunnerHTTPClient creates a new RunnerHTTPClient
func NewRunnerHTTPClient(clientConfig *RunnerHTTPClientConfig) (*http.Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: minTLSVersion,
		},
	}

	if clientConfig.Transport != nil {
		if clientConfig.Transport.InsecureSkipVerify {
			transport.TLSClientConfig.InsecureSkipVerify = true
		}
	}

	client := &http.Client{
		Timeout:   httpRequestAuthTimeout,
		Transport: transport,
	}
	if clientConfig.MaxRedirect != 0 {
		client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
			if len(via) >= clientConfig.MaxRedirect {
				return fmt.Errorf("stopped after %d redirects", clientConfig.MaxRedirect)
			}
			return nil
		}
	}
	// FIXME NEED TO USE THE IMDS BLOCKER
	return client, nil
}
