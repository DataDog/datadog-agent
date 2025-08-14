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
	"fmt"
	"net"
	"net/http"
	"net/url"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgapiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"

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

	// initClusterTLSConfig is used to initialize the cluster TLS configuration.
	// Since we are not creating the IPC certificate, we don't need to pass the certificate fetcher options.
	// This function will only load and set the cluster CA to the global variable.
	_, err = initClusterTLSConfig(reqs)
	if err != nil {
		return Provides{}, fmt.Errorf("error while setting up cluster TLS config: %w", err)
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

	// initClusterTLSConfig is used to initialize the cluster TLS configuration and returns certificate fetcher options.
	// The cluster CA is used to sign the IPC certificate.
	// This way either client from localhost or from the cluster can authenticate server.
	certificateFetcherOptions, err := initClusterTLSConfig(reqs)
	if err != nil {
		return Provides{}, fmt.Errorf("error while setting up cluster TLS config: %w", err)
	}

	ipccert, ipckey, err := cert.FetchOrCreateIPCCert(ctx, reqs.Conf, certificateFetcherOptions...)
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

// initClusterTLSConfig initializes the cluster TLS configuration and returns certificate fetcher options.
// This function performs the following steps:
//
// 1. Retrieves configuration values for TLS verification and cluster CA file path
// 2. Validates the configuration - returns early if both TLS verification is disabled and no CA path is set
// 3. Ensures TLS verification cannot be enabled without a cluster CA file path
// 4. If a cluster CA path is provided:
//   - Reads the cluster CA certificate and private key from the specified path
//   - Configures certificate fetcher options with the CA and external IP
//   - If TLS verification is enabled, creates a secure TLS config with the CA as root certificate
//
// 5. Sets the client TLS configuration globally for cross-node communication
func initClusterTLSConfig(reqs Requires) ([]cert.CertificateFetcherOption, error) {
	// Validate configuration early
	enableTLSVerification := reqs.Conf.GetBool("cluster_trust_chain.enable_tls_verification")
	clusterCAPath := reqs.Conf.GetString("cluster_trust_chain.ca_cert_file_path")
	clusterCAKeyPath := reqs.Conf.GetString("cluster_trust_chain.ca_key_file_path")

	var certificateFetcherOptions []cert.CertificateFetcherOption
	clusterClientTLSConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// early return if TLS verification is disabled and no cluster CA is provided
	if !enableTLSVerification && clusterCAPath == "" && clusterCAKeyPath == "" {
		pkgapiutil.SetCrossNodeClientTLSConfig(clusterClientTLSConfig)
		return certificateFetcherOptions, nil
	}

	// It's not possible to enable TLS verification without a cluster CA
	if enableTLSVerification && (clusterCAPath == "" || clusterCAKeyPath == "") {
		return certificateFetcherOptions, fmt.Errorf("cluster_trust_chain.enable_tls_verification cannot be true if cluster_trust_chain.ca_cert_file_path or cluster_trust_chain.ca_key_file_path is not set")
	}

	if clusterCAPath != "" {
		caCertPEM, caCert, caPrivKey, err := cert.ReadClusterCA(clusterCAPath, clusterCAKeyPath)
		if err != nil {
			return certificateFetcherOptions, fmt.Errorf("unable to read cluster CA cert and key: %w", err)
		}

		// Setting certificate signing by Cluster CA only if the flavor is ClusterAgent or CLC Runner
		if flavor.GetFlavor() == flavor.ClusterAgent || configsetup.IsCLCRunner(reqs.Conf) {
			// Getting the right IP address
			sanIP, err := retrieveExternalIPs(reqs.Conf)
			if err != nil {
				return certificateFetcherOptions, fmt.Errorf("unable to retrieve external IPs: %w", err)
			}

			certificateFetcherOptions = append(certificateFetcherOptions,
				cert.WithClusterCA(caCert, caPrivKey),
				cert.WithExternalIPs(sanIP),
			)
		}

		// if TLS verification is enabled, we need to add the cluster CA to the client TLS config
		if enableTLSVerification {
			certPool := x509.NewCertPool()
			if ok := certPool.AppendCertsFromPEM(caCertPEM); !ok {
				return certificateFetcherOptions, fmt.Errorf("unable to generate certPool from cluster CA cert")
			}
			clusterClientTLSConfig = &tls.Config{
				RootCAs: certPool,
			}
		}
	}

	// set the client TLS config to the global variable
	pkgapiutil.SetCrossNodeClientTLSConfig(clusterClientTLSConfig)

	return certificateFetcherOptions, nil
}

// retrieveExternalIPs retrieves the IP used by clients to contact DCA and CLCRunner servers
// This function support only CLC Runner and Cluster Agent
func retrieveExternalIPs(config config.Component) (net.IP, error) {
	if flavor.GetFlavor() == flavor.ClusterAgent {
		clusterAgentEndpoint, err := configutils.GetClusterAgentEndpoint()
		if err != nil {
			return nil, fmt.Errorf("unable to get cluster agent endpoint: %w", err)
		}
		parsedURL, err := url.Parse(clusterAgentEndpoint)
		if err != nil {
			return nil, fmt.Errorf("unable to parse cluster agent endpoint URL: %w", err)
		}
		podIP, _, err := net.SplitHostPort(parsedURL.Host)
		if err != nil {
			return nil, fmt.Errorf("unable to get pod IP from cluster agent endpoint: %w", err)
		}

		return net.ParseIP(podIP), nil
	}

	if host := config.GetString("clc_runner_host"); host != "" {
		return net.ParseIP(host), nil
	}

	return nil, fmt.Errorf("unable to retrieve external IP")
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
