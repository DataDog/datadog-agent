// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"crypto/tls"
	"crypto/x509"
)

// Kubernetes constants
const (
	DefaultServiceAccountPath      = "/var/run/secrets/kubernetes.io/serviceaccount"
	DefaultServiceAccountTokenPath = DefaultServiceAccountPath + "/token"
	DefaultServiceAccountCAPath    = DefaultServiceAccountPath + "/ca.crt"
)

// GetBearerToken reads the serviceaccount token
func GetBearerToken(authTokenPath string) (string, error) {
	panic("not called")
}

// GetCertificates loads the certificate and the private key
func GetCertificates(certFilePath, keyFilePath string) ([]tls.Certificate, error) {
	panic("not called")
}

// GetCertificateAuthority loads the issuing certificate authority
func GetCertificateAuthority(certPath string) (*x509.CertPool, error) {
	panic("not called")
}
