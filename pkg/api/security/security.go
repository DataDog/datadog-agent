// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package security implements cryptographic certificates and auth token
package security

import (
	"crypto/rsa"
	"crypto/x509"
)

const (
	authTokenName                 = "auth_token"
	authTokenMinimalLen           = 32
	clusterAgentAuthTokenFilename = "cluster_agent.auth_token"
)

// GenerateKeyPair create a public/private keypair
func GenerateKeyPair(bits int) (*rsa.PrivateKey, error) {
	panic("not called")
}

// CertTemplate create x509 certificate template
func CertTemplate() (*x509.Certificate, error) {
	panic("not called")
}

// GenerateRootCert generates a root certificate
func GenerateRootCert(hosts []string, bits int) (cert *x509.Certificate, certPEM []byte, rootKey *rsa.PrivateKey, err error) {
	panic("not called")
}

// GetAuthTokenFilepath returns the path to the auth_token file.
func GetAuthTokenFilepath() string {
	panic("not called")
}

// FetchAuthToken gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func FetchAuthToken() (string, error) {
	panic("not called")
}

// CreateOrFetchToken gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func CreateOrFetchToken() (string, error) {
	panic("not called")
}

func fetchAuthToken(tokenCreationAllowed bool) (string, error) {
	panic("not called")
}

// GetClusterAgentAuthToken load the authentication token from:
// 1st. the configuration value of "cluster_agent.auth_token" in datadog.yaml
// 2nd. from the filesystem
// If using the token from the filesystem, the token file must be next to the datadog.yaml
// with the filename: cluster_agent.auth_token, it will fail if the file does not exist
func GetClusterAgentAuthToken() (string, error) {
	panic("not called")
}

// CreateOrGetClusterAgentAuthToken load the authentication token from:
// 1st. the configuration value of "cluster_agent.auth_token" in datadog.yaml
// 2nd. from the filesystem
// If using the token from the filesystem, the token file must be next to the datadog.yaml
// with the filename: cluster_agent.auth_token, if such file does not exist it will be
// created and populated with a newly generated token.
func CreateOrGetClusterAgentAuthToken() (string, error) {
	panic("not called")
}

func getClusterAgentAuthToken(tokenCreationAllowed bool) (string, error) {
	panic("not called")
}

func validateAuthToken(authToken string) error {
	panic("not called")
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(token, tokenPath string) error {
	panic("not called")
}
