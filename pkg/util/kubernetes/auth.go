// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// Kubernetes constants
const (
	DefaultServiceAccountPath      = "/var/run/secrets/kubernetes.io/serviceaccount"
	DefaultServiceAccountTokenPath = DefaultServiceAccountPath + "/token"
	DefaultServiceAccountCAPath    = DefaultServiceAccountPath + "/ca.crt"
)

// GetBearerToken reads the serviceaccount token
func GetBearerToken(authTokenPath string) (string, error) {
	token, err := os.ReadFile(authTokenPath)
	if err != nil {
		return "", fmt.Errorf("could not read token from %s: %s", authTokenPath, err)
	}
	return string(token), nil
}

// GetCertificates loads the certificate and the private key
func GetCertificates(certFilePath, keyFilePath string) ([]tls.Certificate, error) {
	var certs []tls.Certificate
	cert, err := tls.LoadX509KeyPair(certFilePath, keyFilePath)
	if err != nil {
		return certs, err
	}
	return append(certs, cert), nil
}

// GetCertificateAuthority loads the issuing certificate authority
func GetCertificateAuthority(certPath string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caCert)
	if !ok {
		return caCertPool, fmt.Errorf("fail to load certificate authority: %s", certPath)
	}
	return caCertPool, nil
}
