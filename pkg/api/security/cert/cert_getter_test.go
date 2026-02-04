// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cert

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// The following certificate and key are used for testing purposes only.
// They have been generated using the following command:
//
//	openssl req -x509 -newkey ec:<(openssl ecparam -name prime256v1) -keyout key.pem -out cert.pem -days 3650 \
//	  -subj "/O=Datadog, Inc." \
//	  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
//	  -addext "keyUsage=keyCertSign" \
//	  -addext "extendedKeyUsage=serverAuth,clientAuth" \
//	  -addext "basicConstraints=CA:TRUE" \
//	  -nodes
var (
	clusterCAcert = []byte(`-----BEGIN CERTIFICATE-----
MIIBzTCCAXKgAwIBAgIUWTX/Wlc/ovPPsG5bhU5RzAUb7qYwCgYIKoZIzj0EAwIw
GDEWMBQGA1UECgwNRGF0YWRvZywgSW5jLjAeFw0yNTA4MjIwOTAyNDRaFw0zNTA4
MjAwOTAyNDRaMBgxFjAUBgNVBAoMDURhdGFkb2csIEluYy4wWTATBgcqhkjOPQIB
BggqhkjOPQMBBwNCAAQ68kYT6H8kzjyqCiFHzwWolffAejhBmNbFDRNR694b9MAo
ekrdHSAjlfwHAFxC7SBPfyEn723NvJA+9AWjkEpEo4GZMIGWMB0GA1UdDgQWBBTL
OxLYXEuBE9eiNozfCNVkYw6szjAfBgNVHSMEGDAWgBTLOxLYXEuBE9eiNozfCNVk
Yw6szjAaBgNVHREEEzARgglsb2NhbGhvc3SHBH8AAAEwCwYDVR0PBAQDAgEGMB0G
A1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjAMBgNVHRMEBTADAQH/MAoGCCqG
SM49BAMCA0kAMEYCIQDl7HfsTM2NBJp5HGH2rpnxI6ULLG3GAf7PjOF6FJLYSgIh
AO4uOH/M1w5tJcHFMxW9D6vmn4tTgLPkHjt57EUJWDYG
-----END CERTIFICATE-----
`)
	clusterCAkey = []byte(`-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdGXwlnGYZNIAggVO
26xbChsNii0Peja4sNuyRpFSJZihRANCAAQ68kYT6H8kzjyqCiFHzwWolffAejhB
mNbFDRNR694b9MAoekrdHSAjlfwHAFxC7SBPfyEn723NvJA+9AWjkEpE
-----END PRIVATE KEY-----
`)
)

// setupTempConfig creates a temporary directory and mock config for testing
func setupTempConfig(t *testing.T) (model.Config, string) {
	t.Helper()

	tempDir := t.TempDir()

	config := mock.New(t)

	// Explicitly clear/set all relevant config values to prevent global state contamination
	config.SetWithoutSource("ipc_cert_file_path", filepath.Join(tempDir, "test_cert.pem"))
	config.SetWithoutSource("cluster_trust_chain.ca_cert_file_path", "")
	config.SetWithoutSource("cluster_trust_chain.ca_key_file_path", "")
	config.SetWithoutSource("cluster_trust_chain.enable_tls_verification", false)
	config.SetWithoutSource("clc_runner_host", "")
	config.SetWithoutSource("auth_token_file_path", "")

	return config, tempDir
}

// setupTempConfigWithCA creates config with CA files set up
// This function returns:
// - config: the mock config
// - tempDir: the temporary directory
// - caCert: the CA certificate
// - caPrivKey: the CA private key
func setupTempConfigWithCA(t *testing.T) (model.Config, string, *x509.Certificate, any) {
	t.Helper()

	tempDir := t.TempDir()

	// Write CA cert file
	caCertPath := filepath.Join(tempDir, "ca_cert.pem")
	err := os.WriteFile(caCertPath, clusterCAcert, 0644)
	require.NoError(t, err)

	// Write CA key file
	caKeyPath := filepath.Join(tempDir, "ca_key.pem")
	err = os.WriteFile(caKeyPath, clusterCAkey, 0644)
	require.NoError(t, err)

	config := mock.New(t)

	// Explicitly clear/set all relevant config values to prevent global state contamination
	config.SetWithoutSource("ipc_cert_file_path", filepath.Join(tempDir, "test_cert.pem"))
	config.SetWithoutSource("cluster_trust_chain.ca_cert_file_path", caCertPath)
	config.SetWithoutSource("cluster_trust_chain.ca_key_file_path", caKeyPath)
	config.SetWithoutSource("cluster_trust_chain.enable_tls_verification", true)
	config.SetWithoutSource("clc_runner_host", "")
	config.SetWithoutSource("auth_token_file_path", "")

	// Decode the certificate PEM
	block, _ := pem.Decode(clusterCAcert)
	require.NotNil(t, block, "Failed to decode clusterCAcert PEM")
	require.Equal(t, "CERTIFICATE", block.Type, "Expected CERTIFICATE PEM block")
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "Failed to parse clusterCAcert")

	// Decode the private key PEM
	block, _ = pem.Decode(clusterCAkey)
	require.NotNil(t, block, "Failed to decode clusterCAkey PEM")
	// The key is in PKCS8 format
	privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	require.NoError(t, err, "Failed to parse clusterCAkey as PKCS8")

	return config, tempDir, cert, privKey
}

func TestFetchOrCreateIPCCert_WithClusterCA(t *testing.T) {
	// Setup config with CA files
	config, tempDir, caCert, _ := setupTempConfigWithCA(t)

	// Generate certificate using cluster CA from config
	ctx := context.Background()
	clientConfig, serverConfig, clusterClientConfig, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, clientConfig)
	require.NotNil(t, serverConfig)
	require.NotNil(t, clusterClientConfig)

	// Read the generated certificate file to verify it was signed by our CA
	certFilePath := filepath.Join(tempDir, "test_cert.pem")
	certData, err := os.ReadFile(certFilePath)
	require.NoError(t, err)

	// Parse the generated certificate
	block, _ := pem.Decode(certData)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify the certificate was signed by our CA
	// Create a CA certificate pool with our test CA
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	// Verify the certificate chain
	opts := x509.VerifyOptions{
		Roots: roots,
	}

	chains, err := cert.Verify(opts)
	require.NoError(t, err, "Certificate should be verifiable against the provided CA")
	require.Len(t, chains, 1, "Should have exactly one certificate chain")
	require.Len(t, chains[0], 2, "Chain should contain the cert and the CA")

	// Verify that the leaf certificate is our generated cert
	assert.Equal(t, cert, chains[0][0])
	// Verify that the CA certificate is in the chain
	assert.Equal(t, caCert, chains[0][1])

	// TODO improve test for clusterClientConfig
	// Verify that cluster client config has the proper root CAs since TLS verification is enabled
	assert.NotNil(t, clusterClientConfig.RootCAs, "Cluster client config should have root CAs when TLS verification is enabled")
}

func TestFetchOrCreateIPCCert_WithCLCRunnerHost(t *testing.T) {
	config, tempDir := setupTempConfig(t)

	// Set up CLC runner host configuration
	testHostname := "123.456.789.0"
	config.SetWithoutSource("clc_runner_host", testHostname)

	// Generate certificate with CLC runner host
	ctx := context.Background()
	clientConfig, serverConfig, clusterClientConfig, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, clientConfig)
	require.NotNil(t, serverConfig)
	require.NotNil(t, clusterClientConfig)

	// Read the generated certificate file
	certFilePath := filepath.Join(tempDir, "test_cert.pem")
	certData, err := os.ReadFile(certFilePath)
	require.NoError(t, err)

	// Parse the generated certificate
	block, _ := pem.Decode(certData)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify that the certificate contains the expected IP addresses in SAN
	expectedIPs := []net.IP{
		net.ParseIP("127.0.0.1"), // Default localhost IPv4
		net.ParseIP("::1"),       // Default localhost IPv6
	}

	// Check that all expected IPs are present in the certificate
	for _, expectedIP := range expectedIPs {
		found := false
		for _, certIP := range cert.IPAddresses {
			if certIP.Equal(expectedIP) {
				found = true
				break
			}
		}
		assert.True(t, found, "Certificate should contain IP address %s in SAN field", expectedIP.String())
	}

	// Verify that the certificate contains the default DNS name (but NOT the CLC runner hostname, since no cluster CA is configured)
	assert.Contains(t, cert.DNSNames, "localhost", "Certificate should contain localhost DNS name")
	assert.NotContains(t, cert.DNSNames, testHostname, "Certificate should NOT contain CLC runner hostname when no cluster CA is configured")

	// Verify that cluster client config allows insecure connections since TLS verification is not enabled
	assert.True(t, clusterClientConfig.InsecureSkipVerify, "Cluster client config should skip TLS verification when not enabled")
}

func TestFetchOrCreateIPCCert_WithCAAndCLCRunner(t *testing.T) {
	// Setup config with CA files
	config, tempDir, caCert, _ := setupTempConfigWithCA(t)

	// Fake to be a CLC Runner
	config.SetWithoutSource("clc_runner_enabled", true)
	defer config.SetWithoutSource("clc_runner_enabled", false)

	// Also set up CLC runner host configuration
	testHostname := "clc-runner.example.com"
	config.SetWithoutSource("clc_runner_host", testHostname)

	// Generate certificate using both CA and CLC runner host config
	ctx := context.Background()
	clientConfig, serverConfig, clusterClientConfig, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, clientConfig)
	require.NotNil(t, serverConfig)
	require.NotNil(t, clusterClientConfig)

	// Read the generated certificate file
	certFilePath := filepath.Join(tempDir, "test_cert.pem")
	certData, err := os.ReadFile(certFilePath)
	require.NoError(t, err)

	// Parse the generated certificate
	block, _ := pem.Decode(certData)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify trust chain (cluster CA functionality)
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	opts := x509.VerifyOptions{
		Roots: roots,
	}

	chains, err := cert.Verify(opts)
	require.NoError(t, err, "Certificate should be verifiable against the provided CA")
	require.Len(t, chains, 1)

	// Verify that the certificate contains the CLC runner hostname in DNS names
	assert.Contains(t, cert.DNSNames, testHostname, "Certificate should contain CLC runner hostname in DNS names")

	// Verify default IPs are still present
	localhostIPv4 := net.ParseIP("127.0.0.1")
	localhostIPv6 := net.ParseIP("::1")

	foundLocalhost4 := false
	foundLocalhost6 := false
	for _, certIP := range cert.IPAddresses {
		if certIP.Equal(localhostIPv4) {
			foundLocalhost4 = true
		}
		if certIP.Equal(localhostIPv6) {
			foundLocalhost6 = true
		}
	}
	assert.True(t, foundLocalhost4, "Certificate should still contain localhost IPv4")
	assert.True(t, foundLocalhost6, "Certificate should still contain localhost IPv6")

	// Verify that cluster client config has the proper root CAs since TLS verification is enabled
	assert.NotNil(t, clusterClientConfig.RootCAs, "Cluster client config should have root CAs when TLS verification is enabled")
}

func TestFetchOrCreateIPCCert_WithoutOptions(t *testing.T) {
	config, tempDir := setupTempConfig(t)

	// Generate certificate without any special config (should create self-signed)
	ctx := context.Background()
	clientConfig, serverConfig, clusterClientConfig, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, clientConfig)
	require.NotNil(t, serverConfig)
	require.NotNil(t, clusterClientConfig)

	// Read the generated certificate file
	certFilePath := filepath.Join(tempDir, "test_cert.pem")
	certData, err := os.ReadFile(certFilePath)
	require.NoError(t, err)

	// Parse the generated certificate
	block, _ := pem.Decode(certData)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify it's self-signed (issuer == subject)
	assert.Equal(t, cert.Subject.String(), cert.Issuer.String(), "Certificate should be self-signed")

	// Verify default IP addresses are present
	expectedDefaultIPs := []net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("::1"),
	}

	for _, expectedIP := range expectedDefaultIPs {
		found := false
		for _, certIP := range cert.IPAddresses {
			if certIP.Equal(expectedIP) {
				found = true
				break
			}
		}
		assert.True(t, found, "Certificate should contain default IP address %s", expectedIP.String())
	}

	// Verify default DNS name
	assert.Contains(t, cert.DNSNames, "localhost", "Certificate should contain localhost DNS name")

	// Verify that cluster client config allows insecure connections since TLS verification is not enabled
	assert.True(t, clusterClientConfig.InsecureSkipVerify, "Cluster client config should skip TLS verification when not enabled")
}

// TestFetchOrCreateIPCCert_CertificateReuse tests that if a certificate file already exists,
// it's reused instead of generating a new one
func TestFetchOrCreateIPCCert_CertificateReuse(t *testing.T) {
	config, tempDir := setupTempConfig(t)
	ctx := context.Background()

	// Generate first certificate
	clientConfig1, serverConfig1, clusterClientConfig1, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, clientConfig1)
	require.NotNil(t, serverConfig1)
	require.NotNil(t, clusterClientConfig1)

	// Verify the certificate file was created
	certFilePath := filepath.Join(tempDir, "test_cert.pem")
	_, err = os.Stat(certFilePath)
	require.NoError(t, err, "Certificate file should be created")

	// Read the first certificate content
	cert1Data, err := os.ReadFile(certFilePath)
	require.NoError(t, err)

	// Generate second certificate (should reuse the existing one)
	clientConfig2, serverConfig2, clusterClientConfig2, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, clientConfig2)
	require.NotNil(t, serverConfig2)
	require.NotNil(t, clusterClientConfig2)

	// Read the second certificate content
	cert2Data, err := os.ReadFile(certFilePath)
	require.NoError(t, err)

	// Verify that the same certificate file content is reused
	assert.Equal(t, cert1Data, cert2Data, "Should reuse existing certificate file")

	// Verify that TLS configs are equivalent by checking the certificate serial numbers
	cert1SerialNum := clientConfig1.Certificates[0].Leaf.SerialNumber
	cert2SerialNum := clientConfig2.Certificates[0].Leaf.SerialNumber

	if cert1SerialNum != nil && cert2SerialNum != nil {
		assert.Equal(t, cert1SerialNum, cert2SerialNum, "Certificate serial numbers should be the same")
	}
}

// TestFetchIPCCert tests the FetchIPCCert function (load-only, no create)
func TestFetchIPCCert(t *testing.T) {
	// Setup config with CA files
	config, _, _, _ := setupTempConfigWithCA(t)

	// First, create a certificate using FetchOrCreateIPCCert so we have something to fetch
	ctx := context.Background()
	_, _, _, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)

	// Now test FetchIPCCert (should load the existing certificate)
	clientConfig, serverConfig, clusterClientConfig, err := FetchIPCCert(config)
	require.NoError(t, err)
	require.NotNil(t, clientConfig)
	require.NotNil(t, serverConfig)
	require.NotNil(t, clusterClientConfig)

	// Verify cluster client config has proper root CAs since TLS verification is enabled
	assert.NotNil(t, clusterClientConfig.RootCAs, "Cluster client config should have root CAs when TLS verification is enabled")
	assert.False(t, clusterClientConfig.InsecureSkipVerify, "Should not skip TLS verification when enabled")

	// Test without cluster CA (should still work but with insecure config)
	configNoCA, _ := setupTempConfig(t)
	// Create cert file first
	_, _, _, err = FetchOrCreateIPCCert(ctx, configNoCA)
	require.NoError(t, err)

	clientConfig2, serverConfig2, clusterClientConfig2, err := FetchIPCCert(configNoCA)
	require.NoError(t, err)
	require.NotNil(t, clientConfig2)
	require.NotNil(t, serverConfig2)
	require.NotNil(t, clusterClientConfig2)

	// Verify insecure config when no cluster CA
	assert.True(t, clusterClientConfig2.InsecureSkipVerify, "Should skip TLS verification when not configured")
}

// TestBuildClusterClientTLSConfig_ValidationError tests the validation logic in the helper function
func TestBuildClusterClientTLSConfig_ValidationError(t *testing.T) {
	// Test case: TLS verification enabled but no CA certificate available
	caData := &clusterCAData{
		enableTLSVerification: true,
		caCert:                nil, // Missing CA cert
		caPrivKey:             nil, // Missing CA key
	}

	config, err := caData.buildClusterClientTLSConfig()
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "cluster_trust_chain.enable_tls_verification is true but no CA certificate available. Set ca_cert_file_path/ca_key_file_path or ca_cert_secret_name/ca_cert_secret_namespace")

	// Test case: TLS verification disabled - should work fine without CA
	caDataDisabled := &clusterCAData{
		enableTLSVerification: false,
		caCert:                nil,
		caPrivKey:             nil,
	}

	config2, err := caDataDisabled.buildClusterClientTLSConfig()
	assert.NoError(t, err)
	assert.NotNil(t, config2)
	assert.True(t, config2.InsecureSkipVerify)
}

// TestReadClusterCA_ErrorCases tests error handling in ReadClusterCA function
func TestReadClusterCA_ErrorCases(t *testing.T) {
	tempDir := t.TempDir()

	// Test case 1: Missing cert file
	missingCertPath := filepath.Join(tempDir, "missing_cert.pem")
	missingKeyPath := filepath.Join(tempDir, "missing_key.pem")

	_, _, err := readClusterCA(missingCertPath, missingKeyPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to read cluster CA cert file")

	// Test case 2: Invalid cert file content
	invalidCertPath := filepath.Join(tempDir, "invalid_cert.pem")
	validKeyPath := filepath.Join(tempDir, "valid_key.pem")

	// Write invalid cert content
	err = os.WriteFile(invalidCertPath, []byte("invalid cert content"), 0644)
	require.NoError(t, err)

	// Create a valid key file for the test
	err = os.WriteFile(validKeyPath, clusterCAkey, 0644)
	require.NoError(t, err)

	_, _, err = readClusterCA(invalidCertPath, validKeyPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to decode cluster CA cert PEM")

	// Test case 3: Valid cert but missing key file
	validCertPath := filepath.Join(tempDir, "valid_cert.pem")
	err = os.WriteFile(validCertPath, clusterCAcert, 0644)
	require.NoError(t, err)

	_, _, err = readClusterCA(validCertPath, missingKeyPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to read cluster CA key file")
}

// TestFetchOrCreateIPCCert_ClusterAgentFlavor tests behavior when flavor is set to ClusterAgent
func TestFetchOrCreateIPCCert_ClusterAgentFlavor(t *testing.T) {
	// Set flavor to ClusterAgent to test that specific code path
	flavor.SetFlavor(flavor.ClusterAgent)
	defer flavor.SetFlavor(flavor.DefaultAgent)

	// Setup config with CA files
	config, tempDir, caCert, _ := setupTempConfigWithCA(t)

	// Mocking cluster agent URL configuration is required for ClusterAgent flavor
	// This is needed because ClusterAgent flavor tries to get the cluster agent endpoint
	// for adding it to certificate SANs.
	config.SetWithoutSource("cluster_agent.url", "https://127.0.0.1:5005")
	defer config.SetWithoutSource("cluster_agent.url", "")

	// Generate certificate with ClusterAgent flavor
	ctx := context.Background()
	clientConfig, serverConfig, clusterClientConfig, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)
	require.NotNil(t, clientConfig)
	require.NotNil(t, serverConfig)
	require.NotNil(t, clusterClientConfig)

	// Read the generated certificate file
	certFilePath := filepath.Join(tempDir, "test_cert.pem")
	certData, err := os.ReadFile(certFilePath)
	require.NoError(t, err)

	// Parse the generated certificate
	block, _ := pem.Decode(certData)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify it was signed by our CA
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	opts := x509.VerifyOptions{
		Roots: roots,
	}

	chains, err := cert.Verify(opts)
	require.NoError(t, err, "Certificate should be verifiable against the provided CA")
	require.Len(t, chains, 1)
}
