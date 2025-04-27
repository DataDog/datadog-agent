// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package security implements cryptographic certificates and auth token
package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
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
		return nil, fmt.Errorf("generating random key: %w", err)
	}

	return privKey, nil
}

// CertTemplate create x509 certificate template
func CertTemplate() (*x509.Certificate, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
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

type authtokenFactory struct {
}

func (authtokenFactory) Generate() (string, []byte, error) {
	key := make([]byte, authTokenMinimalLen)
	_, err := rand.Read(key)
	if err != nil {
		return "", nil, fmt.Errorf("can't create agent authentication token value: %v", err.Error())
	}

	// convert the raw token to an hex string
	token := hex.EncodeToString(key)

	return token, []byte(token), nil
}

func (authtokenFactory) Deserialize(raw []byte) (string, error) {
	return string(raw), nil
}

// GetAuthTokenFilepath returns the path to the auth_token file.
func GetAuthTokenFilepath(config configModel.Reader) string {
	if config.GetString("auth_token_file_path") != "" {
		return config.GetString("auth_token_file_path")
	}
	return filepath.Join(filepath.Dir(config.ConfigFileUsed()), authTokenName)
}

// FetchAuthToken gets the authentication token from the auth token file
// Requires that the config has been set up before calling
func FetchAuthToken(config configModel.Reader) (string, error) {
	return filesystem.TryFetchArtifact(GetAuthTokenFilepath(config), &authtokenFactory{}) // TODO IPC: replace this call by FetchArtifact to retry until the artifact is successfully retrieved or the context is done
}

// FetchOrCreateAuthToken gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
// It takes a context to allow for cancellation or timeout of the operation
func FetchOrCreateAuthToken(ctx context.Context, config configModel.Reader) (string, error) {
	return filesystem.FetchOrCreateArtifact(ctx, GetAuthTokenFilepath(config), &authtokenFactory{})
}

// GetClusterAgentAuthToken load the authentication token from:
// 1st. the configuration value of "cluster_agent.auth_token" in datadog.yaml
// 2nd. from the filesystem
// If using the token from the filesystem, the token file must be next to the datadog.yaml
// with the filename: cluster_agent.auth_token, it will fail if the file does not exist
func GetClusterAgentAuthToken(config configModel.Reader) (string, error) {
	return getClusterAgentAuthToken(context.Background(), config, false)
}

// CreateOrGetClusterAgentAuthToken load the authentication token from:
// 1st. the configuration value of "cluster_agent.auth_token" in datadog.yaml
// 2nd. from the filesystem
// If using the token from the filesystem, the token file must be next to the datadog.yaml
// with the filename: cluster_agent.auth_token, if such file does not exist it will be
// created and populated with a newly generated token.
func CreateOrGetClusterAgentAuthToken(ctx context.Context, config configModel.Reader) (string, error) {
	return getClusterAgentAuthToken(ctx, config, true)
}

func getClusterAgentAuthToken(ctx context.Context, config configModel.Reader, tokenCreationAllowed bool) (string, error) {
	authToken := config.GetString("cluster_agent.auth_token")
	if authToken != "" {
		log.Infof("Using configured cluster_agent.auth_token")
		return authToken, validateAuthToken(authToken)
	}

	// load the cluster agent auth token from filesystem
	location := filepath.Join(configUtils.ConfFileDirectory(config), clusterAgentAuthTokenFilename)
	log.Debugf("Empty cluster_agent.auth_token, loading from %s", location)
	if tokenCreationAllowed {
		return filesystem.FetchOrCreateArtifact(ctx, location, &authtokenFactory{})
	}
	authToken, err := filesystem.TryFetchArtifact(location, &authtokenFactory{})
	if err != nil {
		return "", fmt.Errorf("failed to load cluster agent auth token: %v", err)
	}
	return authToken, validateAuthToken(authToken)
}

func validateAuthToken(authToken string) error {
	if len(authToken) < authTokenMinimalLen {
		return fmt.Errorf("cluster agent authentication token must be at least %d characters long, currently: %d", authTokenMinimalLen, len(authToken))
	}
	return nil
}
