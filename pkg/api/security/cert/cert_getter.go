// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cert provide useful functions to generate certificates
package cert

import (
	"bytes"
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
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
	caCert      *x509.Certificate
	caPrivKey   crypto.Signer
	externalIPs []net.IP
}

func (f certificateFactory) Generate() (Certificate, []byte, error) {
	cert, err := generateCertKeyPair(f.caCert, f.caPrivKey, f.externalIPs...)
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
func FetchIPCCert(config configModel.Reader) ([]byte, []byte, error) {
	cert, err := filesystem.TryFetchArtifact(getCertFilepath(config), &certificateFactory{}) // TODO IPC: replace this call by FetchArtifact to retry until the artifact is successfully retrieved or the context is done
	return cert.cert, cert.key, err
}

// CertificateFetcherOption allows to configure the certificate factory
type CertificateFetcherOption func(certificateFactory *certificateFactory)

// FetchOrCreateIPCCert loads or creates certificate file used to authenticate IPC communicates
// It takes a context to allow for cancellation or timeout of the operation
func FetchOrCreateIPCCert(ctx context.Context, config configModel.Reader, options ...CertificateFetcherOption) ([]byte, []byte, error) {

	certificateFactory := certificateFactory{}

	for _, option := range options {
		option(&certificateFactory)
	}

	cert, err := filesystem.FetchOrCreateArtifact(ctx, getCertFilepath(config), certificateFactory)
	return cert.cert, cert.key, err
}

// WithClusterCA sets the cluster CA certificate and key to be used for for signing the IPC certificate
func WithClusterCA(caCert *x509.Certificate, caPrivKey crypto.Signer) CertificateFetcherOption {
	return func(certificateFactory *certificateFactory) {
		certificateFactory.caCert = caCert
		certificateFactory.caPrivKey = caPrivKey
	}
}

// WithExternalIPs sets the external IPs to be added to the IPC certificate SANs
func WithExternalIPs(externalIPs ...net.IP) CertificateFetcherOption {
	return func(certificateFactory *certificateFactory) {
		certificateFactory.externalIPs = externalIPs
	}
}

// ReadClusterCA reads the cluster CA certificate and key from the given path
func ReadClusterCA(caCertPath, caKeyPath string) ([]byte, *x509.Certificate, crypto.Signer, error) {
	var caCert *x509.Certificate
	var caPrivKey crypto.Signer

	// Read the cluster CA cert and key
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to read cluster CA cert file: %w", err)
	}
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to read cluster CA key file: %w", err)
	}

	// Parse the cluster CA cert
	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return nil, nil, nil, fmt.Errorf("unable to decode cluster CA cert PEM")
	}
	caCert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to parse cluster CA cert file: %w", err)
	}

	// Parse the cluster CA key
	block, _ = pem.Decode(caKeyPEM)
	if block == nil {
		return nil, nil, nil, fmt.Errorf("unable to decode cluster CA key PEM")
	}

	switch block.Type {
	case "EC PRIVATE KEY":
		caPrivKey, err = x509.ParseECPrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		caPrivKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, nil, nil, fmt.Errorf("unsupported cluster CA key type: %s", block.Type)
	}

	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to parse cluster CA key file: %w", err)
	}

	return caCertPEM, caCert, caPrivKey, nil
}

// GetTLSConfigFromCert returns the TLS configs for the client and server using the provided IPC certificate and key.
// It returns the client and server TLS configurations, or an error if the certificate or key cannot be parsed.
// It expects the certificate and key to be in PEM format.
func GetTLSConfigFromCert(ipccert, ipckey []byte) (*tls.Config, *tls.Config, error) {
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ipccert); !ok {
		return nil, nil, fmt.Errorf("Unable to generate certPool from PERM IPC cert")
	}
	tlsCert, err := tls.X509KeyPair(ipccert, ipckey)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to generate x509 cert from PERM IPC cert and key")
	}

	clientTLSConfig := &tls.Config{
		RootCAs:      certPool,
		Certificates: []tls.Certificate{tlsCert},
	}

	serverTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		// The server parses the client certificate but does not make any verification, this is useful for telemetry
		ClientAuth: tls.RequestClientCert,
		// The server will accept any client certificate signed by the IPC CA
		ClientCAs: certPool,
	}

	return clientTLSConfig, serverTLSConfig, nil
}
