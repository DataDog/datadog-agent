// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package api

import (
	"crypto/x509"
	"testing"
)

// TrustOIDCExchangeCertificate makes OIDC exchanges trust cert for the rest of the test.
func TrustOIDCExchangeCertificate(t testing.TB, cert *x509.Certificate) {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	previous := oidcExchangeRootCAs
	oidcExchangeRootCAs = pool
	t.Cleanup(func() { oidcExchangeRootCAs = previous })
}
