// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cert provide useful functions to generate certificates
package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

func certTemplate(additionalIPs []net.IP, additionalDNSNames []string) (*x509.Certificate, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %s", err)
	}

	notBefore := time.Now()
	// 50 years duration
	notAfter := notBefore.Add(50 * 365 * 24 * time.Hour)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Datadog, Inc."},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		IPAddresses:           append([]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, additionalIPs...),
		DNSNames:              append([]string{"localhost"}, additionalDNSNames...),
	}

	return &template, nil
}

// Certificate contains certificate and key pair (in PEM format) used to communicate between Agent processes
type Certificate struct {
	cert []byte
	key  []byte
}

// generateCertKeyPair generates a certificate and key pair.
// If signerCert and signerKey are not provided, the root certificate template is used as the parent.
func generateCertKeyPair(signerCert *x509.Certificate, signerKey any, additionalIPs []net.IP, additionalDNSNames []string) (Certificate, error) {
	certTmpl, err := certTemplate(additionalIPs, additionalDNSNames)
	if err != nil {
		return Certificate{}, err
	}

	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Certificate{}, fmt.Errorf("Unable to generate IPC private key: %v", err)
	}

	// If signer is not provided, use the root certificate template as the parent
	if signerCert == nil || signerKey == nil {
		signerCert = certTmpl
		signerKey = certKey
	}

	certDER, err := x509.CreateCertificate(rand.Reader, certTmpl, signerCert, &certKey.PublicKey, signerKey)
	if err != nil {
		return Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	rawKey, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		return Certificate{}, fmt.Errorf("Unable to marshall private key: %v", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: rawKey})

	return Certificate{certPEM, keyPEM}, nil
}
