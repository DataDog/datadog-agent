// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cert

import (
	"crypto/tls"
	"crypto/x509"
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
	isDCA                 bool
	isCLC                 bool
}

// readClusterCACert reads the cluster CA certificate from the given path
func readClusterCACert(caCertPath string) (*x509.Certificate, error) {
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read cluster CA cert file: %w", err)
	}
	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return nil, errors.New("unable to decode cluster CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse cluster CA cert file: %w", err)
	}
	return caCert, nil
}

// readClusterCAPrivateKey reads the cluster CA private key from the given path
func readClusterCAPrivateKey(caKeyPath string) (any, error) {
	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read cluster CA key file: %w", err)
	}
	block, _ := pem.Decode(caKeyPEM)
	if block == nil {
		return nil, errors.New("unable to decode cluster CA key PEM")
	}
	switch block.Type {
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported cluster CA key type: %s", block.Type)
	}
}

// readClusterCAConfig reads cluster CA configuration and files from disk once
// Returns nil if no cluster CA is configured
func readClusterCAConfig(config configModel.Reader) (*clusterCAData, error) {
	enableTLSVerification := config.GetBool("cluster_trust_chain.enable_tls_verification")
	clusterCAPath := config.GetString("cluster_trust_chain.ca_cert_file_path")
	clusterCAKeyPath := config.GetString("cluster_trust_chain.ca_key_file_path")
	isCLC := config.GetBool("clc_runner_enabled")
	isDCA := flavor.GetFlavor() == flavor.ClusterAgent

	// If no cluster CA path is configured, return nil (not an error)
	if clusterCAPath == "" {
		return &clusterCAData{
			enableTLSVerification: enableTLSVerification,
			isDCA:                 isDCA,
			isCLC:                 isCLC,
		}, nil
	}

	// Read cluster CA certificate from disk
	caCert, err := readClusterCACert(clusterCAPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read cluster CA cert: %w", err)
	}

	if clusterCAKeyPath == "" {
		return &clusterCAData{
			enableTLSVerification: enableTLSVerification,
			caCert:                caCert,
			isDCA:                 isDCA,
			isCLC:                 isCLC,
		}, nil
	}

	// Read cluster CA private key from disk
	caPrivKey, err := readClusterCAPrivateKey(clusterCAKeyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read cluster CA key: %w", err)
	}

	return &clusterCAData{
		enableTLSVerification: enableTLSVerification,
		caCert:                caCert,
		caPrivKey:             caPrivKey,
		isDCA:                 isDCA,
		isCLC:                 isCLC,
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
	// Note: only the cluster agent has a private key for the CA certificate

	if c.caCert == nil {
		return nil, errors.New("cluster_trust_chain.enable_tls_verification cannot be true if cluster_trust_chain.ca_cert_file_path is not set")
	}

	if c.caPrivKey == nil && (c.isDCA || c.isCLC) {
		return nil, errors.New("cluster_trust_chain.enable_tls_verification cannot be true if cluster_trust_chain.ca_key_file_path is not set")
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
	// Only proceed if complete cluster CA data is available (cert + key)
	if c.caCert == nil || c.caPrivKey == nil {
		return nil
	}

	factory.caCert = c.caCert
	factory.caPrivKey = c.caPrivKey

	// If the process is not a CLC Runner or Cluster Agent, we don't need to add any SANs
	if !(c.isDCA || c.isCLC) {
		return nil
	}

	var serverHost string
	var dnsNames []string

	// If the process is a Cluster Agent, add the external IP and DNS name to the SANs
	if c.isDCA {
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
	} else if c.isCLC {
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
