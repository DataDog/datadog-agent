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
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

const (
	minTLSVersion = tls.VersionTLS12
)

type RunnerHttpClient struct{}

type RunnerHttpClientConfig struct {
	MaxRedirect            int
	Transport              *RunnerHttpTransportConfig
	AllowIMDSEndpoints     bool
	HTTPTimeout            time.Duration
	HTTPDomainRestrictions []privateconnection.HTTPDomainAllowlist
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
		Timeout:   clientConfig.HTTPTimeout,
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

	if !clientConfig.AllowIMDSEndpoints {
		client = httpclient.WrapClient(client, newIMDSBlockerAuthorizer())
	}
	// The transport guard runs before the initial request and every redirect,
	// including redirects that copy credential headers or replay the body.
	if len(clientConfig.HTTPDomainRestrictions) > 0 {
		client.Transport = &credentialDomainTransport{
			inner:       client.Transport,
			credentials: privateconnection.PrivateCredentials{HTTPDomainRestrictions: clientConfig.HTTPDomainRestrictions},
		}
	}
	return client, nil
}

type credentialDomainTransport struct {
	inner       http.RoundTripper
	credentials privateconnection.PrivateCredentials
}

func (t *credentialDomainTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.credentials.ValidateHTTPURL(req.URL); err != nil {
		return nil, err
	}
	return t.inner.RoundTrip(req)
}
