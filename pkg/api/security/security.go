// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package security implements cryptographic certificates and auth token
package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	authTokenName                 = "auth_token"
	authTokenMinimalLen           = 32
	clusterAgentAuthTokenFilename = "cluster_agent.auth_token"
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
			Organization: []string{"Datadog, Inc."},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
	}

	return &template, nil
}

// GenerateRootCert generates a root certificate
func GenerateRootCert(hosts []string, bits int) (cert *x509.Certificate, certPEM []byte, rootKey *rsa.PrivateKey, err error) {
	// print the caller to identify what is calling this function
	if _, file, line, ok := runtime.Caller(1); ok {
		log.Infof("[%s:%d] Generating root certificate for hosts %v", file, line, strings.Join(hosts, ", "))
	}

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
	rootCertTmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign
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

// GetAuthTokenFilepath returns the path to the auth_token file.
func GetAuthTokenFilepath(config configModel.Reader) string {
	if config.GetString("auth_token_file_path") != "" {
		return config.GetString("auth_token_file_path")
	}
	return filepath.Join(filepath.Dir(config.ConfigFileUsed()), authTokenName)
}

// FetchAuthToken gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func FetchAuthToken(config configModel.Reader) (string, error) {
	return fetchAuthToken(config, false)
}

// CreateOrFetchToken gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func CreateOrFetchToken(config configModel.Reader) (string, error) {
	return fetchAuthToken(config, true)
}

func fetchAuthToken(config configModel.Reader, tokenCreationAllowed bool) (string, error) {
	authTokenFile := GetAuthTokenFilepath(config)

	// Create a new token if it doesn't exist and if permitted by calling func
	if _, e := os.Stat(authTokenFile); os.IsNotExist(e) && tokenCreationAllowed {
		// print the caller to identify what is calling this function
		if _, file, line, ok := runtime.Caller(2); ok {
			log.Infof("[%s:%d] Creating a new authentication token", file, line)
		}
		key := make([]byte, authTokenMinimalLen)
		_, e = rand.Read(key)
		if e != nil {
			return "", fmt.Errorf("can't create agent authentication token value: %s", e)
		}

		// Write the auth token to the auth token file (platform-specific)
		e = saveAuthToken(hex.EncodeToString(key), authTokenFile)
		if e != nil {
			return "", fmt.Errorf("error writing authentication token file on fs: %s", e)
		}
		log.Infof("Saved a new authentication token to %s", authTokenFile)
	}
	// Read the token
	authTokenRaw, e := os.ReadFile(authTokenFile)
	if e != nil {
		return "", fmt.Errorf("unable to read authentication token file: " + e.Error())
	}

	// Do some basic validation
	authToken := strings.TrimSpace(string(authTokenRaw))
	if len(authToken) < authTokenMinimalLen {
		return "", fmt.Errorf("invalid authentication token: must be at least %d characters in length", authTokenMinimalLen)
	}

	return authToken, nil
}

// GetClusterAgentAuthToken load the authentication token from:
// 1st. the configuration value of "cluster_agent.auth_token" in datadog.yaml
// 2nd. from the filesystem
// If using the token from the filesystem, the token file must be next to the datadog.yaml
// with the filename: cluster_agent.auth_token, it will fail if the file does not exist
func GetClusterAgentAuthToken(config configModel.Reader) (string, error) {
	return getClusterAgentAuthToken(config, false)
}

// CreateOrGetClusterAgentAuthToken load the authentication token from:
// 1st. the configuration value of "cluster_agent.auth_token" in datadog.yaml
// 2nd. from the filesystem
// If using the token from the filesystem, the token file must be next to the datadog.yaml
// with the filename: cluster_agent.auth_token, if such file does not exist it will be
// created and populated with a newly generated token.
func CreateOrGetClusterAgentAuthToken(config configModel.Reader) (string, error) {
	return getClusterAgentAuthToken(config, true)
}

func getClusterAgentAuthToken(config configModel.Reader, tokenCreationAllowed bool) (string, error) {
	authToken := config.GetString("cluster_agent.auth_token")
	if authToken != "" {
		log.Infof("Using configured cluster_agent.auth_token")
		return authToken, validateAuthToken(authToken)
	}

	// load the cluster agent auth token from filesystem
	tokenAbsPath := filepath.Join(configUtils.ConfFileDirectory(config), clusterAgentAuthTokenFilename)
	log.Debugf("Empty cluster_agent.auth_token, loading from %s", tokenAbsPath)

	// Create a new token if it doesn't exist
	if _, e := os.Stat(tokenAbsPath); os.IsNotExist(e) && tokenCreationAllowed {
		key := make([]byte, authTokenMinimalLen)
		_, e = rand.Read(key)
		if e != nil {
			return "", fmt.Errorf("can't create cluster agent authentication token value: %s", e)
		}

		// Write the auth token to the auth token file (platform-specific)
		e = saveAuthToken(hex.EncodeToString(key), tokenAbsPath)
		if e != nil {
			return "", fmt.Errorf("error writing authentication token file on fs: %s", e)
		}
		log.Infof("Saved a new authentication token for the Cluster Agent at %s", tokenAbsPath)
	}

	_, err := os.Stat(tokenAbsPath)
	if err != nil {
		return "", fmt.Errorf("empty cluster_agent.auth_token and cannot find %q: %s", tokenAbsPath, err)
	}
	b, err := os.ReadFile(tokenAbsPath)
	if err != nil {
		return "", fmt.Errorf("empty cluster_agent.auth_token and cannot read %s: %s", tokenAbsPath, err)
	}
	log.Debugf("cluster_agent.auth_token loaded from %s", tokenAbsPath)

	authToken = string(b)
	return authToken, validateAuthToken(authToken)
}

func validateAuthToken(authToken string) error {
	if len(authToken) < authTokenMinimalLen {
		return fmt.Errorf("cluster agent authentication token length must be greater than %d, curently: %d", authTokenMinimalLen, len(authToken))
	}
	return nil
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(token, tokenPath string) error {
	log.Infof("Saving a new authentication token in %s", tokenPath)
	if err := os.WriteFile(tokenPath, []byte(token), 0o600); err != nil {
		return err
	}

	perms, err := filesystem.NewPermission()
	if err != nil {
		return err
	}

	if err := perms.RestrictAccessToUser(tokenPath); err != nil {
		log.Errorf("Failed to write auth token acl %s", err)
		return err
	}

	log.Infof("Wrote auth token in %s", tokenPath)
	return nil
}
