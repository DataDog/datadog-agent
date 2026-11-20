// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
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
	AllowExpiredCertificate bool
}

func NewRunnerHttpClient(clientConfig *RunnerHttpClientConfig) (*http.Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: minTLSVersion,
		},
	}

	if clientConfig.Transport != nil {
		if clientConfig.Transport.AllowExpiredCertificate {
			// crypto/tls cannot relax only the validity-period check, so verification moves to VerifyConnection.
			transport.TLSClientConfig.InsecureSkipVerify = true
			transport.TLSClientConfig.VerifyConnection = verifyAllowingExpiredCertificate(nil, time.Now)
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

// verifyAllowingExpiredCertificate verifies chain and hostname against roots (system roots when nil).
// An expired server certificate is verified as of its own expiry, so the whole chain must have been valid then.
func verifyAllowingExpiredCertificate(roots *x509.CertPool, now func() time.Time) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("tls: server presented no certificates")
		}
		// ServerName is empty for IP-literal hosts, and x509 skips hostname verification for an empty DNSName.
		if cs.ServerName == "" {
			return errors.New("tls: allowExpiredCertificate requires a DNS hostname to verify the server certificate against")
		}
		leaf := cs.PeerCertificates[0]
		intermediates := x509.NewCertPool()
		for _, cert := range cs.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		// Derive the instant from the leaf only: platform verifiers (Windows, macOS) attribute every expiry to the leaf.
		at := now()
		if leaf.NotAfter.Before(at) {
			at = leaf.NotAfter
		}
		_, err := leaf.Verify(x509.VerifyOptions{
			DNSName:       cs.ServerName,
			Roots:         roots,
			Intermediates: intermediates,
			CurrentTime:   at,
		})
		return err
	}
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
