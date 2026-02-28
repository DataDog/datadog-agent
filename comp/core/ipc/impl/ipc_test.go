// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ipcimpl

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pkgapiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestSuccessfulCreateAndSetAuthToken(t *testing.T) {
	// Create a new config
	mockConfig := configmock.New(t)
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an auth_token file
	authTokenDir := path.Join(tmpDir, "auth_token_dir")
	err = os.Mkdir(authTokenDir, 0700)
	require.NoError(t, err)
	authTokenLocation := path.Join(authTokenDir, "auth_token")
	mockConfig.SetWithoutSource("auth_token_file_path", authTokenLocation)

	// Create an ipc_cert_file
	ipcCertFileLocation := path.Join(tmpDir, "ipc_cert_file")
	mockConfig.SetWithoutSource("ipc_cert_file_path", ipcCertFileLocation)

	// Check that CreateAndSetAuthToken returns no error
	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}
	comp, err := NewReadWriteComponent(reqs)
	assert.NoError(t, err)

	// Check that the auth_token content is the same as the one in the file
	authTokenFileContent, err := os.ReadFile(authTokenLocation)
	require.NoError(t, err)
	assert.Equal(t, comp.Comp.GetAuthToken(), string(authTokenFileContent))

	// Check that the IPC cert and key have been initialized with the correct values
	assert.NotNil(t, comp.Comp.GetTLSClientConfig().RootCAs)
	assert.NotNil(t, comp.Comp.GetTLSServerConfig().Certificates)
}

func TestSuccessfulLoadAuthToken(t *testing.T) {
	// Create a new config
	mockConfig := configmock.New(t)
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an auth_token file
	authTokenLocation := path.Join(tmpDir, "auth_token")
	mockConfig.SetWithoutSource("auth_token_file_path", authTokenLocation)

	// Check that CreateAndSetAuthToken returns no error
	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}
	RWComp, err := NewReadWriteComponent(reqs)
	assert.NoError(t, err)

	// Check that SetAuthToken returns no error
	ROComp, err := NewReadOnlyComponent(reqs)
	assert.NoError(t, err)

	// Check that the auth_token content is the same as the old one
	assert.Equal(t, RWComp.Comp.GetAuthToken(), ROComp.Comp.GetAuthToken())
	assert.True(t, RWComp.Comp.GetTLSClientConfig().RootCAs.Equal(ROComp.Comp.GetTLSClientConfig().RootCAs))
	assert.EqualValues(t, RWComp.Comp.GetTLSServerConfig().Certificates, ROComp.Comp.GetTLSServerConfig().Certificates)
}

// This test check that if CreateAndSetAuthToken blocks, the function timeout
func TestDeadline(t *testing.T) {
	// Create a new config
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("auth_init_timeout", 1*time.Second)

	// Create a lock file to simulate contention on ipc_cert_file
	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	ipcCertFileLocation := path.Join(tmpDir, "ipc_cert_file")
	mockConfig.SetWithoutSource("ipc_cert_file_path", ipcCertFileLocation)
	lockFile := flock.New(ipcCertFileLocation + ".lock")
	err = lockFile.Lock()
	require.NoError(t, err)
	defer lockFile.Unlock()
	defer os.Remove(ipcCertFileLocation + ".lock")

	// Check that CreateAndSetAuthToken times out when the auth_token file is locked
	start := time.Now()
	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}
	_, err = NewReadWriteComponent(reqs)
	duration := time.Since(start)
	assert.Error(t, err)
	assert.LessOrEqual(t, duration, mockConfig.GetDuration("auth_init_timeout")+time.Second)
}

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

// createTestCA creates a test CA certificate and private key files for testing cluster trust chain
func createTestCA(t *testing.T) (string, string, *x509.Certificate) {
	t.Helper()

	// Parse the cluster CA cert
	block, _ := pem.Decode(clusterCAcert)
	require.NotNil(t, block, "Failed to decode cluster CA cert PEM")

	caCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "Failed to parse cluster CA cert file")

	// Write files to temporary directory
	tmpDir := t.TempDir()
	caCertPath := path.Join(tmpDir, "ca.crt")
	caKeyPath := path.Join(tmpDir, "ca.key")

	err = os.WriteFile(caCertPath, clusterCAcert, 0600)
	require.NoError(t, err)

	err = os.WriteFile(caKeyPath, clusterCAkey, 0600)
	require.NoError(t, err)

	return caCertPath, caKeyPath, caCert
}

// setupBasicIPCConfig creates a basic IPC configuration for testing
func setupBasicIPCConfig(t *testing.T) model.Config {
	t.Helper()

	mockConfig := configmock.New(t)
	tmpDir := t.TempDir()

	// Set up auth token path
	authTokenLocation := path.Join(tmpDir, "auth_token")
	mockConfig.SetWithoutSource("auth_token_file_path", authTokenLocation)

	// Set up IPC cert path
	ipcCertFileLocation := path.Join(tmpDir, "ipc_cert_file")
	mockConfig.SetWithoutSource("ipc_cert_file_path", ipcCertFileLocation)

	return mockConfig
}

// assertTrustChain verifies that the certificate in serverTLSConfig is signed by the provided cluster CA
func assertTrustChain(t *testing.T, serverTLSConfig *tls.Config, caCert *x509.Certificate) {
	t.Helper()

	require.NotNil(t, serverTLSConfig, "Server TLS config should not be nil")
	require.Len(t, serverTLSConfig.Certificates, 1, "Server should have exactly one certificate")

	// Parse the server certificate
	serverCert, err := x509.ParseCertificate(serverTLSConfig.Certificates[0].Certificate[0])
	require.NoError(t, err, "Should be able to parse server certificate")

	// Verify the certificate was signed by our CA
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	opts := x509.VerifyOptions{
		Roots: roots,
	}

	chains, err := serverCert.Verify(opts)
	require.NoError(t, err, "Server certificate should be verifiable against the provided CA")
	require.Len(t, chains, 1, "Should have exactly one certificate chain")
	assert.Equal(t, caCert.Subject.String(), serverCert.Issuer.String(), "Server certificate should be signed by the cluster CA")
}

// assertCrossNodeTLSSkipVerification verifies that CrossNodeClientTLSConfig is configured to skip verification
func assertCrossNodeTLSSkipVerification(t *testing.T) {
	t.Helper()

	crossNodeTLSConfig, err := pkgapiutil.GetCrossNodeClientTLSConfig()
	require.NoError(t, err, "CrossNodeClientTLSConfig should be set by initClusterTLSConfig")
	require.NotNil(t, crossNodeTLSConfig, "CrossNodeClientTLSConfig should not be nil")
	assert.True(t, crossNodeTLSConfig.InsecureSkipVerify, "CrossNodeClientTLSConfig should have InsecureSkipVerify set to true")
}

// TestClusterTrustChain_NoCA_SkipVerification tests case 1:
// No ClusterCA access and configured to skip server certificate verification
func TestClusterTrustChain_NoCA_SkipVerification(t *testing.T) {
	defer pkgapiutil.TestOnlyResetCrossNodeClientTLSConfig()

	mockConfig := setupBasicIPCConfig(t)
	mockConfig.SetWithoutSource("cluster_trust_chain.enable_tls_verification", false)
	// cluster_trust_chain.ca_cert_file_path and ca_key_file_path are empty by default

	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}

	comp, err := NewReadWriteComponent(reqs)
	require.NoError(t, err)
	require.NotNil(t, comp.Comp)

	// Check that CrossNodeClientTLSConfig is set to InsecureSkipVerify
	assertCrossNodeTLSSkipVerification(t)
}

// TestClusterTrustChain_NoCA_RequireVerification tests case 2:
// No ClusterCA access and configured to require server certificate verification (should fail)
func TestClusterTrustChain_NoCA_RequireVerification(t *testing.T) {
	defer pkgapiutil.TestOnlyResetCrossNodeClientTLSConfig()

	mockConfig := setupBasicIPCConfig(t)
	mockConfig.SetWithoutSource("cluster_trust_chain.enable_tls_verification", true)
	// cluster_trust_chain.ca_cert_file_path and ca_key_file_path are empty by default

	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}

	_, err := NewReadWriteComponent(reqs)
	require.Error(t, err, "IPC component creation should fail when TLS verification is enabled without CA")
	assert.Contains(t, err.Error(), "cluster_trust_chain.enable_tls_verification cannot be true if cluster_trust_chain.ca_cert_file_path is not set")
}

// TestClusterTrustChain_WithCA_SkipVerification_ClusterAgent tests case 3:
// Has ClusterCA access and configured to skip verification (ClusterAgent flavor)
func TestClusterTrustChain_WithCA_SkipVerification_ClusterAgent(t *testing.T) {
	defer pkgapiutil.TestOnlyResetCrossNodeClientTLSConfig()

	// Set flavor to ClusterAgent
	originalFlavor := flavor.GetFlavor()
	flavor.SetFlavor(flavor.ClusterAgent)
	defer flavor.SetFlavor(originalFlavor)

	mockConfig := setupBasicIPCConfig(t)
	caCertPath, caKeyPath, caCert := createTestCA(t)

	mockConfig.SetWithoutSource("cluster_trust_chain.ca_cert_file_path", caCertPath)
	mockConfig.SetWithoutSource("cluster_trust_chain.ca_key_file_path", caKeyPath)
	mockConfig.SetWithoutSource("cluster_trust_chain.enable_tls_verification", false)

	// Set cluster agent URL so retrieveExternalIPs works
	mockConfig.SetWithoutSource("cluster_agent.url", "https://10.0.0.1:5005")

	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}

	comp, err := NewReadWriteComponent(reqs)
	require.NoError(t, err)

	// Check that CrossNodeClientTLSConfig is set to InsecureSkipVerify
	assertCrossNodeTLSSkipVerification(t)

	// Check that the server certificate is signed by the cluster CA
	serverTLSConfig := comp.Comp.GetTLSServerConfig()
	assertTrustChain(t, serverTLSConfig, caCert)

	// Verify external IP is in the certificate SAN
	serverCert, err := x509.ParseCertificate(serverTLSConfig.Certificates[0].Certificate[0])
	require.NoError(t, err)

	expectedIPStr := "10.0.0.1"
	found := false
	for _, certIP := range serverCert.IPAddresses {
		if certIP.String() == expectedIPStr {
			found = true
			break
		}
	}
	assert.True(t, found, "Server certificate should contain the external IP %s, but got: %v", expectedIPStr, serverCert.IPAddresses)
}

// TestClusterTrustChain_WithCA_RequireVerification_ClusterAgent tests case 4:
// Has ClusterCA access and configured to require verification (ClusterAgent flavor)
func TestClusterTrustChain_WithCA_RequireVerification_ClusterAgent(t *testing.T) {
	defer pkgapiutil.TestOnlyResetCrossNodeClientTLSConfig()

	// Set flavor to ClusterAgent
	originalFlavor := flavor.GetFlavor()
	flavor.SetFlavor(flavor.ClusterAgent)
	defer flavor.SetFlavor(originalFlavor)

	mockConfig := setupBasicIPCConfig(t)
	caCertPath, caKeyPath, caCert := createTestCA(t)

	mockConfig.SetWithoutSource("cluster_trust_chain.ca_cert_file_path", caCertPath)
	mockConfig.SetWithoutSource("cluster_trust_chain.ca_key_file_path", caKeyPath)
	mockConfig.SetWithoutSource("cluster_trust_chain.enable_tls_verification", true)

	// Set cluster agent URL so retrieveExternalIPs works
	mockConfig.SetWithoutSource("cluster_agent.url", "https://10.0.0.1:5005")

	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}

	comp, err := NewReadWriteComponent(reqs)
	require.NoError(t, err)
	require.NotNil(t, comp.Comp)

	// Check that CrossNodeClientTLSConfig trusts the cluster CA
	crossNodeTLSConfig, err := pkgapiutil.GetCrossNodeClientTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, crossNodeTLSConfig)
	assert.False(t, crossNodeTLSConfig.InsecureSkipVerify, "CrossNodeClientTLSConfig should not skip verification")
	require.NotNil(t, crossNodeTLSConfig.RootCAs, "CrossNodeClientTLSConfig should have RootCAs configured")

	// Verify that the CA is in the RootCAs (should not be empty)
	emptyPool := x509.NewCertPool()
	assert.False(t, crossNodeTLSConfig.RootCAs.Equal(emptyPool), "RootCAs should not be empty")

	// Check that the server certificate is signed by the cluster CA (same as case 3)
	serverTLSConfig := comp.Comp.GetTLSServerConfig()
	assertTrustChain(t, serverTLSConfig, caCert)
}

// TestClusterTrustChain_WithCA_CLCRunner tests that CLCRunner flavor also supports certificate generation
func TestClusterTrustChain_WithCA_CLCRunner(t *testing.T) {
	defer pkgapiutil.TestOnlyResetCrossNodeClientTLSConfig()

	// Set flavor to DefaultAgent (CLCRunner check is via config, not flavor)
	originalFlavor := flavor.GetFlavor()
	flavor.SetFlavor(flavor.DefaultAgent)
	defer flavor.SetFlavor(originalFlavor)

	mockConfig := setupBasicIPCConfig(t)
	caCertPath, caKeyPath, caCert := createTestCA(t)

	mockConfig.SetWithoutSource("cluster_trust_chain.ca_cert_file_path", caCertPath)
	mockConfig.SetWithoutSource("cluster_trust_chain.ca_key_file_path", caKeyPath)
	mockConfig.SetWithoutSource("cluster_trust_chain.enable_tls_verification", false)

	// Set CLC runner configuration - this requires specific setup to enable CLCRunner mode
	mockConfig.SetWithoutSource("clc_runner_enabled", true)
	mockConfig.SetWithoutSource("clc_runner_host", "10.0.0.2")
	// Set configuration providers to enable CLCRunner mode (this is checked by IsCLCRunner)
	mockConfig.SetWithoutSource("config_providers", []map[string]interface{}{
		{"name": "clusterchecks"},
	})

	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}

	comp, err := NewReadWriteComponent(reqs)
	require.NoError(t, err)
	require.NotNil(t, comp.Comp)

	// Check that the server certificate is signed by the cluster CA
	serverTLSConfig := comp.Comp.GetTLSServerConfig()
	assertTrustChain(t, serverTLSConfig, caCert)

	// Verify CLC runner IP is in the certificate SAN
	serverCert, err := x509.ParseCertificate(serverTLSConfig.Certificates[0].Certificate[0])
	require.NoError(t, err)

	expectedIPStr := "10.0.0.2"
	found := false
	for _, certIP := range serverCert.IPAddresses {
		if certIP.String() == expectedIPStr {
			found = true
			break
		}
	}
	assert.True(t, found, "Server certificate should contain the CLC runner host IP %s, but got: %v", expectedIPStr, serverCert.IPAddresses)
}

// TestClusterTrustChain_WithCA_NodeAgent tests that NodeAgent flavor generates CA-signed certificates when cluster CA is configured
func TestClusterTrustChain_WithCA_NodeAgent(t *testing.T) {
	defer pkgapiutil.TestOnlyResetCrossNodeClientTLSConfig()

	// Set flavor to DefaultAgent (NodeAgent is the default flavor)
	originalFlavor := flavor.GetFlavor()
	flavor.SetFlavor(flavor.DefaultAgent)
	defer flavor.SetFlavor(originalFlavor)

	mockConfig := setupBasicIPCConfig(t)
	caCertPath, caKeyPath, caCert := createTestCA(t)

	mockConfig.SetWithoutSource("cluster_trust_chain.ca_cert_file_path", caCertPath)
	mockConfig.SetWithoutSource("cluster_trust_chain.ca_key_file_path", caKeyPath)
	mockConfig.SetWithoutSource("cluster_trust_chain.enable_tls_verification", false)

	reqs := Requires{
		Log:  logmock.New(t),
		Conf: mockConfig,
	}

	comp, err := NewReadWriteComponent(reqs)
	require.NoError(t, err)
	require.NotNil(t, comp.Comp)

	// Check that CrossNodeClientTLSConfig is set to InsecureSkipVerify
	assertCrossNodeTLSSkipVerification(t)

	// Check that the server certificate is signed by the cluster CA
	serverTLSConfig := comp.Comp.GetTLSServerConfig()
	assertTrustChain(t, serverTLSConfig, caCert)

	// Verify it only contains localhost IPs (no external IPs for NodeAgent)
	serverCert, err := x509.ParseCertificate(serverTLSConfig.Certificates[0].Certificate[0])
	require.NoError(t, err)

	expectedIPs := []string{"127.0.0.1", "::1"}

	for _, expectedIPStr := range expectedIPs {
		found := false
		for _, certIP := range serverCert.IPAddresses {
			if certIP.String() == expectedIPStr {
				found = true
				break
			}
		}
		assert.True(t, found, "Certificate should contain localhost IP %s, but got: %v", expectedIPStr, serverCert.IPAddresses)
	}

	// Should not contain any other IPs (NodeAgent doesn't add external IPs like ClusterAgent does)
	assert.Len(t, serverCert.IPAddresses, 2, "NodeAgent certificate should only contain localhost IPs")
}
