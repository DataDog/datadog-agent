// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cert

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// createTestCA creates a test CA certificate and private key for testing
func createTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	// Generate CA private key
	caPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Create CA certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	caTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:  []string{"Test CA"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"New York City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create the CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// Parse the CA certificate
	caCert, err := x509.ParseCertificate(caCertDER)
	require.NoError(t, err)

	return caCert, caPrivKey
}

// setupTempConfig creates a temporary directory and mock config for testing
func setupTempConfig(t *testing.T) (model.Config, string) {
	tempDir := t.TempDir()

	config := mock.New(t)
	config.SetWithoutSource("ipc_cert_file_path", filepath.Join(tempDir, "test_cert.pem"))

	return config, tempDir
}

func TestFetchOrCreateIPCCert_WithClusterCA(t *testing.T) {
	config, _ := setupTempConfig(t)

	// Create test CA
	caCert, caPrivKey := createTestCA(t)

	// Generate certificate using WithClusterCA option
	ctx := context.Background()
	certPEM, keyPEM, err := FetchOrCreateIPCCert(ctx, config, WithClusterCA(caCert, caPrivKey))
	require.NoError(t, err)
	require.NotEmpty(t, certPEM)
	require.NotEmpty(t, keyPEM)

	// Parse the generated certificate
	block, _ := pem.Decode(certPEM)
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

	// Additional verification: check that the certificate's issuer matches the CA's subject
	assert.Equal(t, caCert.Subject.String(), cert.Issuer.String())
}

func TestFetchOrCreateIPCCert_WithExternalIPs(t *testing.T) {
	config, _ := setupTempConfig(t)

	// Define test external IPs
	externalIP1 := net.ParseIP("192.168.1.100")
	externalIP2 := net.ParseIP("10.0.0.50")
	externalIP3 := net.ParseIP("2001:db8::1") // IPv6 address

	require.NotNil(t, externalIP1)
	require.NotNil(t, externalIP2)
	require.NotNil(t, externalIP3)

	// Generate certificate using WithExternalIPs option
	ctx := context.Background()
	certPEM, keyPEM, err := FetchOrCreateIPCCert(ctx, config, WithExternalIPs(externalIP1, externalIP2, externalIP3))
	require.NoError(t, err)
	require.NotEmpty(t, certPEM)
	require.NotEmpty(t, keyPEM)

	// Parse the generated certificate
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify that the certificate contains the expected IP addresses in SAN
	expectedIPs := []net.IP{
		net.ParseIP("127.0.0.1"), // Default localhost IPv4
		net.ParseIP("::1"),       // Default localhost IPv6
		externalIP1,              // Our external IPs
		externalIP2,
		externalIP3,
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

	// Verify we have the correct number of IP addresses (no extra ones)
	assert.Len(t, cert.IPAddresses, len(expectedIPs), "Certificate should contain exactly the expected number of IP addresses")

	// Verify that the certificate also contains the default DNS name
	assert.Contains(t, cert.DNSNames, "localhost", "Certificate should contain localhost DNS name")
}

func TestFetchOrCreateIPCCert_WithMultipleOptions(t *testing.T) {
	config, _ := setupTempConfig(t)

	// Create test CA
	caCert, caPrivKey := createTestCA(t)

	// Define test external IP
	externalIP := net.ParseIP("192.168.1.200")
	require.NotNil(t, externalIP)

	// Generate certificate using both options
	ctx := context.Background()
	certPEM, keyPEM, err := FetchOrCreateIPCCert(ctx, config,
		WithClusterCA(caCert, caPrivKey),
		WithExternalIPs(externalIP))
	require.NoError(t, err)
	require.NotEmpty(t, certPEM)
	require.NotEmpty(t, keyPEM)

	// Parse the generated certificate
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify trust chain (WithClusterCA functionality)
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	opts := x509.VerifyOptions{
		Roots: roots,
	}

	chains, err := cert.Verify(opts)
	require.NoError(t, err, "Certificate should be verifiable against the provided CA")
	require.Len(t, chains, 1)

	// Verify external IP is in SAN (WithExternalIPs functionality)
	found := false
	for _, certIP := range cert.IPAddresses {
		if certIP.Equal(externalIP) {
			found = true
			break
		}
	}
	assert.True(t, found, "Certificate should contain the external IP address %s", externalIP.String())

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
}

func TestFetchOrCreateIPCCert_WithoutOptions(t *testing.T) {
	config, _ := setupTempConfig(t)

	// Generate certificate without any options (should create self-signed)
	ctx := context.Background()
	certPEM, keyPEM, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)
	require.NotEmpty(t, certPEM)
	require.NotEmpty(t, keyPEM)

	// Parse the generated certificate
	block, _ := pem.Decode(certPEM)
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
}

// TestFetchOrCreateIPCCert_CertificateReuse tests that if a certificate file already exists,
// it's reused instead of generating a new one
func TestFetchOrCreateIPCCert_CertificateReuse(t *testing.T) {
	config, tempDir := setupTempConfig(t)
	ctx := context.Background()

	// Generate first certificate
	cert1PEM, key1PEM, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)

	// Verify the certificate file was created
	certFilePath := filepath.Join(tempDir, "test_cert.pem")
	_, err = os.Stat(certFilePath)
	require.NoError(t, err, "Certificate file should be created")

	// Generate second certificate (should reuse the existing one)
	cert2PEM, key2PEM, err := FetchOrCreateIPCCert(ctx, config)
	require.NoError(t, err)

	// Verify that the same certificate and key are returned
	assert.Equal(t, cert1PEM, cert2PEM, "Should reuse existing certificate")
	assert.Equal(t, key1PEM, key2PEM, "Should reuse existing key")
}
