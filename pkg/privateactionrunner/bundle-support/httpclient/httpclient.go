// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/httpclient"
)

const (
	minTLSVersion = tls.VersionTLS12
	// defaultMaxRedirects matches the limit net/http applies when CheckRedirect is nil.
	defaultMaxRedirects = 10
)

type RunnerHttpClient struct{}

type RunnerHttpClientConfig struct {
	MaxRedirect        int
	Transport          *RunnerHttpTransportConfig
	AllowIMDSEndpoints bool
	HTTPTimeout        time.Duration
	// SameOriginRedirectsOnly makes the client refuse redirects that leave the
	// scheme, host and port of the first request, except an http to https
	// upgrade on the same host and default ports.
	SameOriginRedirectsOnly bool
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
	if clientConfig.SameOriginRedirectsOnly {
		maxRedirect := clientConfig.MaxRedirect
		if maxRedirect == 0 {
			maxRedirect = defaultMaxRedirects
		}
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirect {
				return fmt.Errorf("stopped after %d redirects", maxRedirect)
			}
			if !isSameOriginRedirect(via[0].URL, req.URL) {
				return fmt.Errorf("refusing redirect from %s to a different origin %s", via[0].URL.Host, req.URL.Host)
			}
			return nil
		}
	} else if clientConfig.MaxRedirect != 0 {
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

// isSameOriginRedirect reports whether a redirect to target keeps the origin of
// the first request. Upgrading http to https on default ports is accepted since
// reverse proxies in front of a service commonly enforce it.
func isSameOriginRedirect(first, target *url.URL) bool {
	if !strings.EqualFold(first.Hostname(), target.Hostname()) {
		return false
	}
	firstScheme, targetScheme := strings.ToLower(first.Scheme), strings.ToLower(target.Scheme)
	firstPort, targetPort := effectivePort(first), effectivePort(target)
	if firstScheme == targetScheme {
		return firstPort != "" && firstPort == targetPort
	}
	return firstScheme == "http" && targetScheme == "https" && firstPort == "80" && targetPort == "443"
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	}
	return ""
}
