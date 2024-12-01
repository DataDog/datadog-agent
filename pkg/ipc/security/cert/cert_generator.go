// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cert provide useful functions to generate certificates
package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func generateKeyPair(bits int) (*rsa.PrivateKey, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("generating random key: %v", err)
	}

	return privKey, nil
}

func certTemplate() (*x509.Certificate, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %s", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Datadog, Inc."},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
	}

	return &template, nil
}

// GenerateCertKeyPair generates a root certificate
func GenerateCertKeyPair(hosts []string, bits int) ([]byte, []byte, error) {
	// print the caller to identify what is calling this function
	if _, file, line, ok := runtime.Caller(1); ok {
		log.Infof("[%s:%d] Generating root certificate for hosts %v", file, line, strings.Join(hosts, ", "))
	}

	rootCertTmpl, err := certTemplate()
	if err != nil {
		return nil, nil, err
	}

	rootKey, err := generateKeyPair(bits)
	if err != nil {
		return nil, nil, err
	}

	// describe what the certificate will be used for
	rootCertTmpl.IsCA = true
	rootCertTmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign
	rootCertTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			rootCertTmpl.IPAddresses = append(rootCertTmpl.IPAddresses, ip)
		} else {
			rootCertTmpl.DNSNames = append(rootCertTmpl.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, rootCertTmpl, rootCertTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey)})

	return certPEM, keyPEM, nil
}

// FetchAgentIPCCert gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func FetchAgentIPCCert(destDir string) ([]byte, []byte, error) {
	return fetchAgentIPCCert(destDir, false)
}

// CreateOrFetchAgentIPCCert gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func CreateOrFetchAgentIPCCert(destDir string) ([]byte, []byte, error) {
	return fetchAgentIPCCert(destDir, true)
}

func fetchAgentIPCCert(certPath string, certCreationAllowed bool) ([]byte, []byte, error) {
	// Create a new token if it doesn't exist and if permitted by calling func
	if _, e := os.Stat(certPath + ".cert"); os.IsNotExist(e) && certCreationAllowed {
		// print the caller to identify what is calling this function
		if _, file, line, ok := runtime.Caller(2); ok {
			log.Infof("[%s:%d] Creating a new IPC certificate", file, line)
		}

		hosts := []string{"127.0.0.1", "localhost", "::1"}
		// hosts = append(hosts, additionalHostIdentities...)
		cert, key, err := GenerateCertKeyPair(hosts, 2048)

		if err != nil {
			return nil, nil, err
		}

		// Write the auth token to the auth token file (platform-specific)
		e = saveIPCCertKey(cert, key, certPath)
		if e != nil {
			return nil, nil, fmt.Errorf("error writing authentication token file on fs: %s", e)
		}
		log.Infof("Saved a new  IPC certificate/key pair to %s", certPath)

		return cert, key, nil
	}

	// Read the token
	cert, e := os.ReadFile(certPath + ".cert")
	if e != nil {
		return nil, nil, fmt.Errorf("unable to read authentication token file: %s", e.Error())
	}
	key, e := os.ReadFile(certPath + ".key")
	if e != nil {
		return nil, nil, fmt.Errorf("unable to read authentication token file: %s", e.Error())
	}

	return cert, key, nil
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveIPCCertKey(cert, key []byte, dest string) error {
	log.Infof("Saving a new IPC certificate/key pair in %s", dest)
	if err := os.WriteFile(dest+".cert", cert, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(dest+".key", key, 0o600); err != nil {
		return err
	}

	perms, err := filesystem.NewPermission()
	if err != nil {
		return err
	}

	if err := perms.RestrictAccessToUser(dest + ".cert"); err != nil {
		log.Errorf("Failed to write auth token acl %s", err)
		return err
	}

	if err := perms.RestrictAccessToUser(dest + ".key"); err != nil {
		log.Errorf("Failed to write auth token acl %s", err)
		return err
	}

	log.Infof("Wrote IPC certificate/key pair in %s", dest)
	return nil
}
