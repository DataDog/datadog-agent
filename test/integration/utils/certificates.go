// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils provides utility functions
package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/errors"
)

// CertificatesConfig holds certificate configuration
type CertificatesConfig struct {
	Hosts        string
	ValidFor     time.Duration
	RsaBits      int
	EcdsaCurve   string
	CertFilePath string
	KeyFilePath  string
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func pemBlockForKey(privateKey interface{}) (*pem.Block, error) {
	switch k := privateKey.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}, nil
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal ECDSA private key: %v", err)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}, nil
	default:
		return nil, fmt.Errorf("unrecognized format for privateKey")
	}
}

// GenerateCertificates create a self-signed certificate with a private key according the config in parameter
func GenerateCertificates(config *CertificatesConfig) error {
	var privateKey interface{}
	var err error

	switch config.EcdsaCurve {
	case "":
		privateKey, err = rsa.GenerateKey(rand.Reader, config.RsaBits)
	case "P224":
		privateKey, err = ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	case "P256":
		privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "P384":
		privateKey, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "P521":
		privateKey, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	default:
		return fmt.Errorf("unrecognized elliptic curve: %q", config.EcdsaCurve)
	}

	if err != nil {
		return fmt.Errorf("failed to generate private key: %s", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(config.ValidFor)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"datadog"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	hosts := strings.Split(config.Hosts, ",")
	for _, h := range hosts {
		ip := net.ParseIP(h)
		if ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, h)
	}

	template.IsCA = true
	template.KeyUsage = x509.KeyUsageCertSign

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(privateKey), privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %s", err)
	}

	certOut, err := os.Create(config.CertFilePath)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %s", config.CertFilePath, err)
	}
	defer certOut.Close()
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		return err
	}

	keyOut, err := os.OpenFile(config.KeyFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %s", config.KeyFilePath, err)
	}
	defer keyOut.Close()

	p, err := pemBlockForKey(privateKey)
	if err != nil {
		return err
	}
	err = pem.Encode(keyOut, p)
	if err != nil {
		return err
	}

	err = errors.NewAggregate([]error{
		certOut.Close(),
		keyOut.Close(),
	})

	return err
}
