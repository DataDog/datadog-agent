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
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/httpclient"
)

const (
	minTLSVersion = tls.VersionTLS12
)

type RunnerHttpClient struct{}

type RunnerHttpClientConfig struct {
	MaxRedirect        int
	Transport          *RunnerHttpTransportConfig
	AllowIMDSEndpoints bool
	HTTPTimeout        time.Duration
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
