// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/httpclient"
)

const (
	httpRequestAuthTimeout = time.Second * 30
	minTLSVersion          = tls.VersionTLS12
)

type RunnerHttpClient struct{}

type RunnerHttpClientConfig struct {
	MaxRedirect        int
	Transport          *RunnerHttpTransportConfig
	AllowIMDSEndpoints bool
}

type RunnerHttpTransportConfig struct {
	InsecureSkipVerify bool
}

func NewRunnerHttpClient(clientConfig *RunnerHttpClientConfig) (*http.Client, error) {
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
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= clientConfig.MaxRedirect {
				return fmt.Errorf("stopped after %d redirects", clientConfig.MaxRedirect)
			}
			return nil
		}
	}

	if clientConfig.AllowIMDSEndpoints {
		return client, nil
	}
	authorizer := newIMDSBlockerAuthorizer()
	return httpclient.WrapClient(client, authorizer), nil
}
