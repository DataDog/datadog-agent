// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	stdLog "log"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/api/agent"
	"github.com/DataDog/datadog-agent/cmd/agent/api/check"
	"github.com/DataDog/datadog-agent/cmd/agent/api/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/gorilla/mux"
)

var (
	listener net.Listener
)

// create a public/private keypair
func GenerateKeyPair(bits int) (*rsa.PrivateKey, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("generating random key: %v", err)
	}

	return privKey, nil
}

func CertTemplate() (*x509.Certificate, error) {
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
			Organization: []string{"Datadoc, Inc."},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
	}

	return &template, nil
}

func GenerateRootCert(hosts []string, bits int) (
	cert *x509.Certificate, certPEM []byte, rootKey *rsa.PrivateKey, err error) {

	rootCertTmpl, err := CertTemplate()
	if err != nil {
		return
	}

	rootKey, err = GenerateKeyPair(bits)
	if err != nil {
		return
	}

	// describe what the certificate will be used for
	rootCertTmpl.IsCA = true
	rootCertTmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
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
		return
	}
	// parse the resulting certificate so we can use it again
	cert, err = x509.ParseCertificate(certDER)
	if err != nil {
		return
	}
	// PEM encode the certificate (this is a standard TLS encoding)
	b := pem.Block{Type: "CERTIFICATE", Bytes: certDER}
	certPEM = pem.EncodeToMemory(&b)
	return
}

// StartServer creates the router and starts the HTTP server
func StartServer() {
	// create the root HTTP router
	r := mux.NewRouter()

	// IPC REST API server
	agent.SetupHandlers(r.PathPrefix("/agent").Subrouter())
	check.SetupHandlers(r.PathPrefix("/check").Subrouter())

	// get the transport we're going to use under HTTP
	var err error
	listener, err = getListener()
	if err != nil {
		// we use the listener to handle commands for the Agent, there's
		// no way we can recover from this error
		panic(fmt.Sprintf("Unable to create the api server: %v", err))
	}
	common.SetSessionToken()

	//
	hosts := []string{"127.0.0.1", "localhost"}
	_, rootCertPEM, rootKey, err := GenerateRootCert(hosts, 2048)
	if err != nil {
		stdLog.Fatalf("unable to start TLS server")
		return
	}

	// PEM encode the private key
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})

	// Create a TLS cert using the private key and certificate
	rootTLSCert, err := tls.X509KeyPair(rootCertPEM, rootKeyPEM)
	if err != nil {
		stdLog.Fatalf("invalid key pair: %v", err)
		return
	}

	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{rootTLSCert},
	}

	srv := &http.Server{
		Handler:   r,
		ErrorLog:  stdLog.New(&config.ErrorLogWriter{}, "", 0), // log errors to seelog
		TLSConfig: &tlsConfig,
	}
	tlsListener := tls.NewListener(listener, &tlsConfig)

	go srv.Serve(tlsListener)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	listener.Close()
}
