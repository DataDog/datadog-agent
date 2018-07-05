// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"

	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	authTokenName                 = "auth_token"
	authTokenMinimalLen           = 32
	clusterAgentAuthTokenFilename = "cluster_agent_auth_token"
)

// GenerateKeyPair create a public/private keypair
func GenerateKeyPair(bits int) (*rsa.PrivateKey, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("generating random key: %v", err)
	}

	return privKey, nil
}

// CertTemplate create x509 certificate template
func CertTemplate() (*x509.Certificate, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %s", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Datadoc, Inc."},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
	}

	return &template, nil
}

// GenerateRootCert generates a root certificate
func GenerateRootCert(hosts []string, bits int) (
	cert *x509.Certificate, certPEM []byte, rootKey *rsa.PrivateKey, err error) {

	rootCertTmpl, err := CertTemplate()
	if err != nil {
		return
	}

	rootKey, err = GenerateKeyPair(bits)
	if err != nil {
		return
	}

	// describe what the certificate will be used for
	rootCertTmpl.IsCA = true
	rootCertTmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	rootCertTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			rootCertTmpl.IPAddresses = append(rootCertTmpl.IPAddresses, ip)
		} else {
			rootCertTmpl.DNSNames = append(rootCertTmpl.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, rootCertTmpl, rootCertTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		return
	}
	// parse the resulting certificate so we can use it again
	cert, err = x509.ParseCertificate(certDER)
	if err != nil {
		return
	}
	// PEM encode the certificate (this is a standard TLS encoding)
	b := pem.Block{Type: "CERTIFICATE", Bytes: certDER}
	certPEM = pem.EncodeToMemory(&b)
	return
}

// FetchAuthToken gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func FetchAuthToken() (string, error) {
	var authTokenFile string
	if config.Datadog.GetString("auth_token_file_path") != "" {
		authTokenFile = config.Datadog.GetString("auth_token_file_path")
	} else {
		authTokenFile = filepath.Join(filepath.Dir(config.Datadog.ConfigFileUsed()), authTokenName)
	}

	// Create a new token if it doesn't exist
	if _, e := os.Stat(authTokenFile); os.IsNotExist(e) {
		key := make([]byte, authTokenMinimalLen)
		_, e = rand.Read(key)
		if e != nil {
			return "", fmt.Errorf("error creating authentication token: %s", e)
		}

		// Write the auth token to the auth token file (platform-specific)
		e = saveAuthToken(hex.EncodeToString(key), authTokenFile)
		if e != nil {
			return "", fmt.Errorf("error creating authentication token: %s", e)
		}
		log.Infof("Saved a new authentication token to %s", authTokenFile)
	}

	// Read the token
	authTokenRaw, e := ioutil.ReadFile(authTokenFile)
	if e != nil {
		return "", fmt.Errorf("unable to access authentication token: " + e.Error())
	}

	// Do some basic validation
	authToken := string(authTokenRaw)
	if len(authToken) < authTokenMinimalLen {
		return "", fmt.Errorf("invalid authentication token: must be at least %d characters in length", authTokenMinimalLen)
	}

	return authToken, nil
}

// DeleteAuthToken removes auth_token file (test clean up)
func DeleteAuthToken() error {
	authTokenFile := filepath.Join(filepath.Dir(config.Datadog.ConfigFileUsed()), authTokenName)
	return os.Remove(authTokenFile)
}

// GetClusterAgentAuthToken load the authentication token from:
// 1st. the configuration value of "cluster_agent.auth_token" in datadog.yaml
// 2nd. from the filesystem
// If using the token from the filesystem, the token file must be next to the datadog.yaml
// with the filename: cluster_agent_auth_token
func GetClusterAgentAuthToken() (string, error) {
	authToken := config.Datadog.GetString("cluster_agent.auth_token")
	if authToken != "" {
		log.Debugf("Using configured cluster_agent.auth_token")
		return authToken, validateAuthToken(authToken)
	}

	// load the cluster agent auth token from filesystem
	tokenAbsPath := filepath.Join(config.FileUsedDir(), clusterAgentAuthTokenFilename)
	log.Debugf("Empty cluster_agent_auth_token, loading from %s", tokenAbsPath)

	// Create a new token if it doesn't exist
	if _, e := os.Stat(tokenAbsPath); os.IsNotExist(e) {
		key := make([]byte, authTokenMinimalLen)
		_, e = rand.Read(key)
		if e != nil {
			return "", fmt.Errorf("error creating authentication token: %s", e)
		}

		// Write the auth token to the auth token file (platform-specific)
		e = saveAuthToken(hex.EncodeToString(key), tokenAbsPath)
		if e != nil {
			return "", fmt.Errorf("error creating authentication token: %s", e)
		}
		log.Infof("Saved a new authentication token to %s", tokenAbsPath)
	}

	_, err := os.Stat(tokenAbsPath)
	if err != nil {
		return "", fmt.Errorf("empty cluster_agent_auth_token and cannot find %q: %s", tokenAbsPath, err)
	}
	b, err := ioutil.ReadFile(tokenAbsPath)
	if err != nil {
		return "", fmt.Errorf("empty cluster_agent_auth_token and cannot read %s: %s", tokenAbsPath, err)
	}
	log.Debugf("cluster_agent_auth_token loaded from %s", tokenAbsPath)

	authToken = string(b)
	return authToken, validateAuthToken(authToken)
}

func validateAuthToken(authToken string) error {
	if len(authToken) < authTokenMinimalLen {
		return fmt.Errorf("cluster agent authentication token length must be greater than %d, curently: %d", authTokenMinimalLen, len(authToken))
	}
	return nil
}
