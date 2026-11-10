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

// maxExpiryRetries bounds the extra chain verifications for expired certificates.
const maxExpiryRetries = 4

// verifyAllowingExpiredCertificate verifies chain and hostname against roots (system roots when nil),
// accepting a chain that would have verified at an earlier expiry instant of one of its certificates.
func verifyAllowingExpiredCertificate(roots *x509.CertPool, now func() time.Time) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("tls: server presented no certificates")
		}
		// ServerName is empty for IP-literal hosts, and x509 skips hostname verification for an empty DNSName.
		if cs.ServerName == "" {
			return errors.New("tls: allowExpiredCertificate requires a DNS hostname to verify the server certificate against")
		}
		intermediates := x509.NewCertPool()
		for _, cert := range cs.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		opts := x509.VerifyOptions{
			DNSName:       cs.ServerName,
			Roots:         roots,
			Intermediates: intermediates,
			CurrentTime:   now(),
		}
		leaf := cs.PeerCertificates[0]
		_, err := leaf.Verify(opts)
		for retries := 0; err != nil && retries < maxExpiryRetries; retries++ {
			var invalidErr x509.CertificateInvalidError
			if !errors.As(err, &invalidErr) || invalidErr.Reason != x509.Expired || invalidErr.Cert == nil {
				return err
			}
			// Follow the failing certificate, including omitted roots, so unrelated peers cannot consume retries.
			at := invalidErr.Cert.NotAfter
			if !at.Before(opts.CurrentTime) || at.Before(leaf.NotBefore) {
				return err
			}
			opts.CurrentTime = at
			_, err = leaf.Verify(opts)
		}
		return err
	}
}
