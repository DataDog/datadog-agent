// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package kubernetes

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
)

// Kubernetes constants
const (
	ServiceAccountPath      = "/var/run/secrets/kubernetes.io/serviceaccount"
	ServiceAccountTokenPath = ServiceAccountPath + "/token"
)

// IsServiceAccountTokenAvailable returns if a service account token is available on disk
func IsServiceAccountTokenAvailable() bool {
	_, err := os.Stat(ServiceAccountTokenPath)
	return err == nil
}

// GetBearerToken reads the serviceaccount token
func GetBearerToken(authTokenPath string) (string, error) {
	token, err := ioutil.ReadFile(authTokenPath)
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
	caCert, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(caCert)
	if ok == false {
		return caCertPool, fmt.Errorf("fail to load certificate authority: %s", certPath)
	}
	return caCertPool, nil
}
