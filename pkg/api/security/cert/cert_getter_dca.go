// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

// clusterCAData holds the cluster CA configuration and certificate data
type clusterCAData struct {
	enableTLSVerification bool
	caCert                *x509.Certificate
	caPrivKey             any
}

// readClusterCA reads the cluster CA certificate and key from the given path
func readClusterCA(caCertPath, caKeyPath string) (*x509.Certificate, any, error) {
	var caCert *x509.Certificate

	// Read the cluster CA cert and key
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to read cluster CA cert file: %w", err)
	}
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to read cluster CA key file: %w", err)
	}

	// Parse the cluster CA cert
	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return nil, nil, errors.New("unable to decode cluster CA cert PEM")
	}
	caCert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse cluster CA cert file: %w", err)
	}

	// Parse the cluster CA key
	block, _ = pem.Decode(caKeyPEM)
	if block == nil {
		return nil, nil, errors.New("unable to decode cluster CA key PEM")
	}

	var caPrivKey any
	var caParseErr error

	switch block.Type {
	case "PRIVATE KEY":
		caPrivKey, caParseErr = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		caPrivKey, caParseErr = x509.ParseECPrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		caPrivKey, caParseErr = x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, nil, fmt.Errorf("unsupported cluster CA key type: %s", block.Type)
	}

	if caParseErr != nil {
		return nil, nil, fmt.Errorf("unable to parse cluster CA key file: %w", caParseErr)
	}

	return caCert, caPrivKey, nil
}

// parseClusterCACertPEM decodes a base64-encoded PEM certificate string and
// returns the parsed x509 certificate.
func parseClusterCACertPEM(base64PEM string) (*x509.Certificate, error) {
	pemBytes, err := base64.StdEncoding.DecodeString(base64PEM)
	if err != nil {
		return nil, fmt.Errorf("unable to base64-decode cluster CA cert PEM: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("unable to PEM-decode cluster CA cert")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse cluster CA cert: %w", err)
	}
	return cert, nil
}

// readClusterCAConfig reads cluster CA configuration and files from disk once
// Returns nil if no cluster CA is configured
func readClusterCAConfig(config configModel.Reader) (*clusterCAData, error) {
	enableTLSVerification := config.GetBool("cluster_trust_chain.enable_tls_verification")
	clusterCAPath := config.GetString("cluster_trust_chain.ca_cert_file_path")
	clusterCAKeyPath := config.GetString("cluster_trust_chain.ca_key_file_path")

	// If file paths are configured, read from disk
	if clusterCAPath != "" && clusterCAKeyPath != "" {
		caCert, caPrivKey, err := readClusterCA(clusterCAPath, clusterCAKeyPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read cluster CA cert and key: %w", err)
		}

		return &clusterCAData{
			enableTLSVerification: enableTLSVerification,
			caCert:                caCert,
			caPrivKey:             caPrivKey,
		}, nil
	}

	// Fallback: check for base64-encoded PEM in config (e.g. injected via env var by sidecar webhook)
	caCertPEM := config.GetString("cluster_trust_chain.ca_cert_pem")
	if caCertPEM != "" {
		cert, err := parseClusterCACertPEM(caCertPEM)
		if err != nil {
			return nil, fmt.Errorf("unable to parse cluster CA cert from PEM config: %w", err)
		}
		return &clusterCAData{
			enableTLSVerification: enableTLSVerification,
			caCert:                cert,
			caPrivKey:             nil,
		}, nil
	}

	// No cluster CA configured (not an error)
	return &clusterCAData{
		enableTLSVerification: enableTLSVerification,
	}, nil
}

// buildClusterClientTLSConfig creates the TLS configuration for cluster client communication
// using pre-read cluster CA data
func (c *clusterCAData) buildClusterClientTLSConfig() (*tls.Config, error) {
	// Default to insecure configuration
	if !c.enableTLSVerification {
		return &tls.Config{
			InsecureSkipVerify: true,
		}, nil
	}

	// If TLS verification is enabled, configure proper certificate validation
	// It's not possible to have TLS verification enabled without a CA certificate
	if c.caCert == nil {
		return nil, errors.New("cluster_trust_chain.enable_tls_verification cannot be true if cluster_trust_chain.ca_cert_file_path or cluster_trust_chain.ca_cert_pem is not set")
	}

	// If the cluster agent is running, it must have the private key to it's certificate
	if c.caPrivKey == nil && flavor.GetFlavor() == flavor.ClusterAgent {
		return nil, errors.New("cluster_trust_chain.enable_tls_verification cannot be true if cluster_trust_chain.ca_key_file_path is not set on the cluster agent")
	}

	clusterClientCertPool := x509.NewCertPool()
	clusterClientCertPool.AddCert(c.caCert)
	return &tls.Config{
		RootCAs: clusterClientCertPool,
	}, nil
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
	var dnsNames []string

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

		// Add Kubernetes service DNS names for the cluster agent
		serviceName := config.GetString("cluster_agent.kubernetes_service_name")
		ns := namespace.GetResourcesNamespace()
		if serviceName != "" && ns != "" {
			dnsNames = []string{
				serviceName,
				fmt.Sprintf("%s.%s", serviceName, ns),
				fmt.Sprintf("%s.%s.svc", serviceName, ns),
				fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, ns),
			}
		}
	} else if isCLC {
		// If the process is a CLC Runner, add the CLC Runner host to the SANs
		clcRunnerHost := config.GetString("clc_runner_host")
		if clcRunnerHost == "" {
			return errors.New("clc_runner_host is not set")
		}
		serverHost = clcRunnerHost
	}

	// Determine if the server host is an IP address or DNS name and add to appropriate SANs
	ip := net.ParseIP(serverHost)
	if ip != nil {
		factory.additionalIPs = []net.IP{ip}
	} else if serverHost != "" {
		dnsNames = append(dnsNames, serverHost)
	}

	// Add all collected DNS names to the factory
	if len(dnsNames) > 0 {
		factory.additionalDNSNames = dnsNames
	}

	return nil
}
