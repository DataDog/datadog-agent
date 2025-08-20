// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cert

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// clusterCAData holds the cluster CA configuration and certificate data
type clusterCAData struct {
	enableTLSVerification bool
	caCert                *x509.Certificate
	caPrivKey             any
}

// readClusterCAConfig reads cluster CA configuration and files from disk once
// Returns nil if no cluster CA is configured
func readClusterCAConfig(config configModel.Reader) (*clusterCAData, error) {
	enableTLSVerification := config.GetBool("cluster_trust_chain.enable_tls_verification")
	clusterCAPath := config.GetString("cluster_trust_chain.ca_cert_file_path")
	clusterCAKeyPath := config.GetString("cluster_trust_chain.ca_key_file_path")

	// If no cluster CA path is configured, return nil (not an error)
	if clusterCAPath == "" || clusterCAKeyPath == "" {
		return &clusterCAData{
			enableTLSVerification: enableTLSVerification,
		}, nil
	}

	// Read cluster CA certificate and private key from disk
	caCert, caPrivKey, err := ReadClusterCA(clusterCAPath, clusterCAKeyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read cluster CA cert and key: %w", err)
	}

	return &clusterCAData{
		enableTLSVerification: enableTLSVerification,
		caCert:                caCert,
		caPrivKey:             caPrivKey,
	}, nil
}

// buildClusterClientTLSConfig creates the TLS configuration for cluster client communication
// using pre-read cluster CA data
func (c *clusterCAData) buildClusterClientTLSConfig() (*tls.Config, error) {
	// Default to insecure configuration
	clusterClientConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// If TLS verification is enabled, configure proper certificate validation
	if c.enableTLSVerification {
		// It's not possible to have TLS verification enabled without a CA certificate
		if c.caCert == nil || c.caPrivKey == nil {
			return nil, fmt.Errorf("cluster_trust_chain.enable_tls_verification cannot be true if cluster_trust_chain.ca_cert_file_path or cluster_trust_chain.ca_key_file_path is not set")
		}

		clusterClientCertPool := x509.NewCertPool()
		clusterClientCertPool.AddCert(c.caCert)
		clusterClientConfig = &tls.Config{
			RootCAs: clusterClientCertPool,
		}
	}

	return clusterClientConfig, nil
}

// setupCertificateFactoryWithClusterCA configures the certificate factory with cluster CA
// and determines additional SANs based on the agent flavor and configuration
func (c *clusterCAData) setupCertificateFactoryWithClusterCA(config configModel.Reader, factory *certificateFactory) error {
	// Only proceed if cluster CA data is available
	if c.caCert == nil || c.caPrivKey == nil {
		return nil
	}

	factory.caCert = c.caCert
	factory.caPrivKey = c.caPrivKey

	isCLC := config.GetBool("clc_runner_enabled")
	isClusterAgent := flavor.GetFlavor() == flavor.ClusterAgent
	isOther := !isCLC && !isClusterAgent

	// If the process is not a CLC Runner or Cluster Agent, we don't need to add any SANs
	if isOther {
		return nil
	}

	var serverHost string

	// If the process is a Cluster Agent, add the external IP and DNS name to the SANs
	if isClusterAgent {
		clusterAgentEndpoint, err := configutils.GetClusterAgentEndpoint()
		if err != nil {
			return fmt.Errorf("unable to get cluster agent endpoint: %w", err)
		}
		parsedURL, err := url.Parse(clusterAgentEndpoint)
		if err != nil {
			return fmt.Errorf("unable to parse cluster agent endpoint URL: %w", err)
		}

		serverHost, _, err = net.SplitHostPort(parsedURL.Host)
		if err != nil {
			return fmt.Errorf("unable to get pod IP from cluster agent endpoint: %w", err)
		}
	}

	// If the process is a CLC Runner, add the CLC Runner host to the SANs
	if isCLC {
		clcRunnerHost := config.GetString("clc_runner_host")
		if clcRunnerHost == "" {
			return fmt.Errorf("clc_runner_host is not set")
		}
		serverHost = clcRunnerHost
	}

	// Determine if the server host is an IP address or DNS name and add to appropriate SANs
	ip := net.ParseIP(serverHost)
	if ip != nil {
		factory.additionalIPs = []net.IP{ip}
	} else if serverHost != "" {
		factory.additionalDNSNames = []string{serverHost}
	}

	return nil
}
