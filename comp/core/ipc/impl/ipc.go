// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ipcimpl implements the IPC component.
package ipcimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgapiutil "github.com/DataDog/datadog-agent/pkg/api/util"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// Requires defines the dependencies for the ipc component
type Requires struct {
	Conf config.Component
	Log  log.Component
}

// Provides defines the output of the ipc component
type Provides struct {
	Comp       ipc.Component
	HTTPClient ipc.HTTPClient
}

type ipcComp struct {
	logger          log.Component
	conf            config.Component
	client          ipc.HTTPClient
	token           string
	tlsClientConfig *tls.Config
	tlsServerConfig *tls.Config
}

// NewReadOnlyComponent creates a new ipc component by trying to read the auth artifacts on filesystem.
// If the auth artifacts are not found, it will return an error.
func NewReadOnlyComponent(reqs Requires) (Provides, error) {
	reqs.Log.Debug("Loading IPC artifacts")
	var err error
	token, err := pkgtoken.FetchAuthToken(reqs.Conf)
	if err != nil {
		return Provides{}, fmt.Errorf("unable to fetch auth token (please check that the Agent is running, this file is normally generated during the first run of the Agent service): %s", err)
	}
	ipccert, ipckey, err := cert.FetchIPCCert(reqs.Conf)
	if err != nil {
		return Provides{}, fmt.Errorf("unable to fetch IPC certificate (please check that the Agent is running, this file is normally generated during the first run of the Agent service): %s", err)
	}

	return buildIPCComponent(reqs, token, ipccert, ipckey)
}

// NewReadWriteComponent creates a new ipc component by trying to read the auth artifacts on filesystem,
// and if they are not found, it will create them.
func NewReadWriteComponent(reqs Requires) (Provides, error) {
	reqs.Log.Debug("Loading or creating IPC artifacts")
	authTimeout := reqs.Conf.GetDuration("auth_init_timeout")
	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()
	reqs.Log.Infof("starting to load the IPC auth primitives (timeout: %v)", authTimeout)

	var err error
	token, err := pkgtoken.FetchOrCreateAuthToken(ctx, reqs.Conf)
	if err != nil {
		return Provides{}, fmt.Errorf("error while creating or fetching auth token: %w", err)
	}
	ipccert, ipckey, err := cert.FetchOrCreateIPCCert(ctx, reqs.Conf)
	if err != nil {
		return Provides{}, fmt.Errorf("error while creating or fetching IPC cert: %w", err)
	}

	return buildIPCComponent(reqs, token, ipccert, ipckey)
}

// NewInsecureComponent creates an IPC component instance suitable for specific commands
// (like 'flare' or 'diagnose') that must function even when the main Agent isn't running
// or IPC artifacts (like auth tokens) are missing or invalid.
//
// This constructor *always* succeeds, unlike NewReadWriteComponent or NewReadOnlyComponent
// which might fail if artifacts are absent or incorrect.
// However, the resulting IPC component instance might be non-functional or only partially
// functional, potentially leading to failures later, such as rejected connections during
// the IPC handshake if communication with the core Agent is attempted.
//
// WARNING: This constructor is intended *only* for edge cases like diagnostics and flare generation.
// Using it in standard agent processes or commands that rely on stable IPC communication
// will likely lead to unexpected runtime errors or security issues.
func NewInsecureComponent(reqs Requires) Provides {
	reqs.Log.Debug("Loading IPC artifacts (insecure)")
	provides, err := NewReadOnlyComponent(reqs)
	if err == nil {
		return provides
	}

	reqs.Log.Warnf("Failed to create ipc component: %v", err)

	httpClient := ipchttp.NewClient("", &tls.Config{}, reqs.Conf)

	return Provides{
		Comp: &ipcComp{
			logger: reqs.Log,
			conf:   reqs.Conf,
			client: httpClient,
			// Insecure component does not have a valid token or TLS configs
			// This is expected, as it is used for diagnostics and flare generation
			token:           "",
			tlsClientConfig: &tls.Config{},
			tlsServerConfig: &tls.Config{},
		},
		HTTPClient: httpClient,
	}
}

// GetAuthToken returns the session token
func (ipc *ipcComp) GetAuthToken() string {
	return ipc.token
}

// GetTLSClientConfig return a TLS configuration with the IPC certificate for http.Client
func (ipc *ipcComp) GetTLSClientConfig() *tls.Config {
	return ipc.tlsClientConfig.Clone()
}

// GetTLSServerConfig return a TLS configuration with the IPC certificate for http.Server
func (ipc *ipcComp) GetTLSServerConfig() *tls.Config {
	return ipc.tlsServerConfig.Clone()
}

func (ipc *ipcComp) HTTPMiddleware(next http.Handler) http.Handler {
	return ipchttp.NewHTTPMiddleware(func(format string, params ...interface{}) {
		ipc.logger.Errorf(format, params...)
	}, ipc.GetAuthToken())(next)
}

func (ipc *ipcComp) GetClient() ipc.HTTPClient {
	return ipc.client
}

func buildIPCComponent(reqs Requires, token string, ipccert, ipckey []byte) (Provides, error) {
	tlsClientConfig, tlsServerConfig, err := cert.GetTLSConfigFromCert(ipccert, ipckey)
	if err != nil {
		return Provides{}, fmt.Errorf("error while setting TLS configs: %w", err)
	}

	// printing the fingerprint of the loaded auth stack is useful to troubleshoot IPC issues
	printAuthSignature(reqs.Log, token, ipccert, ipckey)

	httpClient := ipchttp.NewClient(token, tlsClientConfig, reqs.Conf)

	if err := setupClusterTLSConfig(reqs, tlsServerConfig); err != nil {
		return Provides{}, fmt.Errorf("error while setting up cluster TLS config: %w", err)
	}

	return Provides{
		Comp: &ipcComp{
			logger:          reqs.Log,
			conf:            reqs.Conf,
			client:          httpClient,
			token:           token,
			tlsClientConfig: tlsClientConfig,
			tlsServerConfig: tlsServerConfig,
		},
		HTTPClient: httpClient,
	}, nil
}

// setupClusterTLSConfig sets up TLS configurations for cluster communication.
//
// This function performs the following operations when cluster CA configuration is present:
// 1. Generates a service certificate signed by the cluster CA for server authentication
// 2. Adds the generated certificate to the server TLS config
// 3. Configures the client TLS config to trust the cluster CA
//
// Security consideration: The cluster CA is kept separate from the global IPC client
// TLS configuration to maintain isolation. This design ensures that a compromise of
// the cluster CA does not affect the security of other IPC communications.
func setupClusterTLSConfig(reqs Requires, tlsServerConfig *tls.Config) error {
	clusterClientTLSConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	clusterCAPath := reqs.Conf.GetString("cluster_agent.cluster_ca_file_path")
	enableTLSVerification := reqs.Conf.GetBool("cluster_agent.enable_tls_verification")

	// Validate configuration early
	if enableTLSVerification && clusterCAPath == "" {
		return fmt.Errorf("cluster_agent.enable_tls_verification cannot be true if cluster_agent.cluster_ca_file_path is not set")
	}

	// If no cluster CA is configured, we can skip the rest of the function
	if clusterCAPath == "" {
		pkgapiutil.SetCrossNodeClientTLSConfig(clusterClientTLSConfig)
		return nil
	}

	caCertPath := filepath.Join(clusterCAPath, "cert.pem")
	caKeyPath := filepath.Join(clusterCAPath, "key.pem")

	// Read the cluster CA cert and key
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return fmt.Errorf("unable to read cluster CA cert file: %w", err)
	}
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return fmt.Errorf("unable to read cluster CA key file: %w", err)
	}

	// Parse the cluster CA cert
	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return fmt.Errorf("unable to decode cluster CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("unable to parse cluster CA cert file: %w", err)
	}

	// Parse the cluster CA key
	block, _ = pem.Decode(caKeyPEM)
	if block == nil {
		return fmt.Errorf("unable to decode cluster CA key PEM")
	}
	if block.Type != "EC PRIVATE KEY" {
		return fmt.Errorf("unsupported cluster CA key type: %s", block.Type)
	}

	caKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("unable to parse cluster CA key file: %w", err)
	}

	// Generate a service certificate signed by the cluster CA

	// TODO IPC: add pod IP to the certificate
	serviceCertPEM, serviceKeyPEM, err := cert.GenerateCertFromCA(caCert, caKey, []net.IP{})
	if err != nil {
		return fmt.Errorf("unable to generate service certificate: %w", err)
	}

	// Generate a TLS certificate from the service certificate and key
	tlsCert, err := tls.X509KeyPair(serviceCertPEM, serviceKeyPEM)
	if err != nil {
		return fmt.Errorf("unable to generate x509 cert from service cert and key: %w", err)
	}

	// Adding the cluster certificate to the server TLS config
	tlsServerConfig.Certificates = append(tlsServerConfig.Certificates, tlsCert)

	// Enable cross-node TLS verification if configured
	if enableTLSVerification {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(caCertPEM); !ok {
			return fmt.Errorf("unable to generate certPool from cluster CA cert")
		}
		clusterClientTLSConfig = &tls.Config{
			RootCAs: certPool,
		}
	}

	pkgapiutil.SetCrossNodeClientTLSConfig(clusterClientTLSConfig)
	return nil
}

// printAuthSignature computes and logs the authentication signature for the given token and IPC certificate/key.
// It uses SHA-256 to hash the concatenation of the token, IPC certificate, and IPC key.
func printAuthSignature(logger log.Component, token string, ipccert, ipckey []byte) {
	h := sha256.New()

	_, err := h.Write(bytes.Join([][]byte{[]byte(token), ipccert, ipckey}, []byte{}))
	if err != nil {
		logger.Warnf("error while computing auth signature: %v", err)
	}

	sign := h.Sum(nil)
	logger.Infof("successfully loaded the IPC auth primitives (fingerprint: %.8x)", sign)
}
