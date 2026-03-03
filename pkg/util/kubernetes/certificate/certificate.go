// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package certificate provides helpers to work with Kubernetes secrets.
package certificate

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"

	corelisters "k8s.io/client-go/listers/core/v1"
)

const (
	certKey             = "cert.pem"
	keyKey              = "key.pem"
	certCacheKey        = "CertFrom-%s/%s"
	certCacheExpiration = 5 * time.Minute
)

// GenerateSecretData generates the content of Secret.Data
// of the Secret object containing the certificate.
func GenerateSecretData(notBefore, notAfter time.Time, hosts []string) (map[string][]byte, error) {
	certPEM, keyPEM, err := generateCertificate(
		hosts,
		notBefore,
		notAfter)
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate: %v", err)
	}
	data := map[string][]byte{
		certKey: certPEM,
		keyKey:  keyPEM,
	}
	return data, nil
}

// GetCertFromSecret returns the x509.Certificate from Secret.Data.
func GetCertFromSecret(data map[string][]byte) (*x509.Certificate, error) {
	certPEM, ok := data[certKey]
	if !ok {
		return nil, fmt.Errorf("the Secret data doesn't contain an entry for %q", certKey)
	}

	certAsn1, _ := pem.Decode(certPEM)
	if certAsn1 == nil {
		return nil, errors.New("failed to parse certificate PEM")
	}

	return x509.ParseCertificate(certAsn1.Bytes)
}

// GetDurationBeforeExpiration returns the time.Duration before the TLS certificate expires.
func GetDurationBeforeExpiration(cert *x509.Certificate) time.Duration {
	return -time.Since(cert.NotAfter)
}

// GetDNSNames returns the configured DNS names from the certificate.
func GetDNSNames(cert *x509.Certificate) []string {
	return cert.DNSNames
}

// ParseSecretData return the tls.Certificate contained in the provided Secret.Data.
func ParseSecretData(data map[string][]byte) (tls.Certificate, error) {
	return tls.X509KeyPair(data[certKey], data[keyKey])
}

// GetCABundle returns the CA certificate contained in the provided Secret.Data.
func GetCABundle(data map[string][]byte) []byte {
	return data[certKey]
}

func generateCertificate(hosts []string, notBefore, notAfter time.Time) ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Datadog"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the certificate: %v", err)
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, nil, fmt.Errorf("failed to encode the certificate: %v", err)
	}

	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return nil, nil, fmt.Errorf("failed to encode the private key: %v", err)
	}

	return certBuf.Bytes(), keyBuf.Bytes(), nil
}

// GetCertificateFromLister returns the certificate from the informer cache.
func GetCertificateFromLister(lister corelisters.SecretNamespaceLister, secretName string) (*tls.Certificate, error) {
	secret, err := lister.Get(secretName)
	if err != nil {
		return nil, err
	}

	cert, err := ParseSecretData(secret.Data)
	if err != nil {
		return nil, err
	}

	return &cert, nil
}
