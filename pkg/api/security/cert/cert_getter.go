// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cert provide useful functions to generate certificates
package cert

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultCertFileName represent the default IPC certificate root name (without .cert or .key)
const defaultCertFileName = "ipc_cert.pem"

// getCertFilepath returns the path to the IPC cert file.
func getCertFilepath(config configModel.Reader) string {
	if configPath := config.GetString("ipc_cert_file_path"); configPath != "" {
		return configPath
	}
	// Since customers who set the "auth_token_file_path" configuration likely prefer to avoid writing it next to the configuration file,
	// we should follow this behavior for the cert/key generation as well to minimize the risk of disrupting IPC functionality.
	if config.GetString("auth_token_file_path") != "" {
		dest := filepath.Join(filepath.Dir(config.GetString("auth_token_file_path")), defaultCertFileName)
		log.Warnf("IPC cert/key created or retrieved next to auth_token_file_path location: %v", dest)
		return dest
	}
	return filepath.Join(filepath.Dir(config.ConfigFileUsed()), defaultCertFileName)
}

type certificateFactory struct {
	caCert             *x509.Certificate
	caPrivKey          any // x509.ParsePKCS8PrivateKey returns as the private key any, and x509.CreateCertificate takes any as the private key argument
	additionalIPs      []net.IP
	additionalDNSNames []string
}

func (f certificateFactory) Generate() (Certificate, []byte, error) {
	cert, err := generateCertKeyPair(f.caCert, f.caPrivKey, f.additionalIPs, f.additionalDNSNames)
	return cert, bytes.Join([][]byte{cert.cert, cert.key}, []byte{}), err
}

func (certificateFactory) Deserialize(raw []byte) (Certificate, error) {
	block, rest := pem.Decode(raw)

	if block == nil || block.Type != "CERTIFICATE" {
		return Certificate{}, log.Error("failed to decode PEM block containing certificate")
	}
	cert := pem.EncodeToMemory(block)

	block, _ = pem.Decode(rest)

	if block == nil || block.Type != "EC PRIVATE KEY" {
		return Certificate{}, log.Error("failed to decode PEM block containing key")
	}

	key := pem.EncodeToMemory(block)

	return Certificate{cert, key}, nil
}

// FetchIPCCert loads certificate file used to authenticate IPC communicates
func FetchIPCCert(config configModel.Reader) (*tls.Config, *tls.Config, *tls.Config, error) {
	// Read cluster CA configuration and files once
	caData, err := readClusterCAConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error reading cluster CA config: %w", err)
	}

	// Build cluster client TLS configuration using pre-read CA data
	clusterClientConfig, err := caData.buildClusterClientTLSConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error building cluster client TLS config: %w", err)
	}

	cert, err := filesystem.TryFetchArtifact(getCertFilepath(config), &certificateFactory{}) // TODO IPC: replace this call by FetchArtifact to retry until the artifact is successfully retrieved or the context is done
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while fetching IPC cert: %w", err)
	}

	clientConfig, serverConfig, err := GetTLSConfigFromCert(cert.cert, cert.key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while setting TLS configs: %w", err)
	}
	return clientConfig, serverConfig, clusterClientConfig, nil
}

// FetchOrCreateIPCCert loads or creates certificate file used to authenticate IPC communicates
// It takes a context to allow for cancellation or timeout of the operation
func FetchOrCreateIPCCert(ctx context.Context, config configModel.Reader) (*tls.Config, *tls.Config, *tls.Config, error) {
	// Read cluster CA configuration and files once
	caData, err := readClusterCAConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error reading cluster CA config: %w", err)
	}

	// Build cluster client TLS configuration using pre-read CA data
	clusterClientConfig, err := caData.buildClusterClientTLSConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error building cluster client TLS config: %w", err)
	}

	// Setup certificate factory with cluster CA and SANs
	var certificateFactory certificateFactory
	if err := caData.setupCertificateFactoryWithClusterCA(config, &certificateFactory); err != nil {
		return nil, nil, nil, fmt.Errorf("error setting up certificate factory with cluster CA: %w", err)
	}

	cert, err := filesystem.FetchOrCreateArtifact(ctx, getCertFilepath(config), certificateFactory)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while fetching or creating IPC cert: %w", err)
	}

	clientConfig, serverConfig, err := GetTLSConfigFromCert(cert.cert, cert.key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error while setting TLS configs: %w", err)
	}
	return clientConfig, serverConfig, clusterClientConfig, err
}

// GetTLSConfigFromCert returns the TLS configs for the client and server using the provided IPC certificate and key.
// It returns the client and server TLS configurations, or an error if the certificate or key cannot be parsed.
// It expects the certificate and key to be in PEM format.
func GetTLSConfigFromCert(ipccert, ipckey []byte) (*tls.Config, *tls.Config, error) {
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ipccert); !ok {
		return nil, nil, errors.New("Unable to generate certPool from PEM IPC cert")
	}
	tlsCert, err := tls.X509KeyPair(ipccert, ipckey)
	if err != nil {
		return nil, nil, errors.New("Unable to generate x509 cert from PERM IPC cert and key")
	}

	clientTLSConfig := &tls.Config{
		RootCAs:      certPool,
		Certificates: []tls.Certificate{tlsCert},
	}

	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		// Strict mTLS: require and verify client certificate (e.g. for RAR, config stream).
		ClientAuth: tls.RequireAndVerifyClientCert,
		// The server accepts only client certificates signed by the IPC CA.
		ClientCAs: certPool,
	}

	return clientTLSConfig, serverTLSConfig, nil
}

// LoadIPCClientTLSConfigFromFile reads an IPC cert file (PEM with certificate and private key)
// and returns a TLS config suitable for use as an mTLS client (e.g. config-stream-client,
// remote-agent) connecting to the agent IPC. The file must contain a CERTIFICATE block
// followed by an EC PRIVATE KEY or PRIVATE KEY block.
func LoadIPCClientTLSConfigFromFile(filePath string) (*tls.Config, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading IPC cert file: %w", err)
	}
	block, rest := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("IPC cert file: missing or invalid CERTIFICATE PEM block")
	}
	certPEM := pem.EncodeToMemory(block)
	block, _ = pem.Decode(rest)
	if block == nil || (block.Type != "EC PRIVATE KEY" && block.Type != "PRIVATE KEY") {
		return nil, errors.New("IPC cert file: missing or invalid private key PEM block")
	}
	keyPEM := pem.EncodeToMemory(block)
	clientConfig, _, err := GetTLSConfigFromCert(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return clientConfig, nil
}
