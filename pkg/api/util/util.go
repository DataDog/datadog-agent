// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package util implements helper functions for the api
package util

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type source int

const (
	uninitialized source = iota
	setAuthToken
	createAndSetAuthToken
)

var (
	tokenLock sync.RWMutex
	token     string
	dcaToken  string
	// The clientTLSConfig is set by default with `InsecureSkipVerify: true`.
	// This is intentionally done to allow the Agent to local Agent APIs when the clientTLSConfig is not yet initialized.
	// However, this default value should be removed in the future.
	// TODO: Monitor and fix the logs printed by GetTLSClientConfig and GetTLSServerConfig.
	clientTLSConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	serverTLSConfig = &tls.Config{}
	initSource      source
)

// SetAuthToken sets the session token and IPC certificate
// Requires that the config has been set up before calling
func SetAuthToken(config model.Reader) error {
	tokenLock.Lock()
	defer tokenLock.Unlock()

	// Noop if token is already set
	if initSource != uninitialized {
		return nil
	}

	var err error
	token, err = pkgtoken.FetchAuthToken(config)
	if err != nil {
		return err
	}
	ipccert, ipckey, err := cert.FetchIPCCert(config)
	if err != nil {
		return err
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ipccert); !ok {
		return fmt.Errorf("unable to use cert for creating CertPool")
	}

	clientTLSConfig = &tls.Config{
		RootCAs: certPool,
	}

	tlsCert, err := tls.X509KeyPair(ipccert, ipckey)
	if err != nil {
		return err
	}
	serverTLSConfig = &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}

	// printing the fingerprint of the loaded auth stack is useful to troubleshoot IPC issues
	printAuthSignature(token, ipccert, ipckey)
	initSource = setAuthToken

	return nil
}

// CreateAndSetAuthToken creates and sets the authorization token and IPC certificate
// Requires that the config has been set up before calling
func CreateAndSetAuthToken(config model.Reader) error {
	tokenLock.Lock()
	defer tokenLock.Unlock()

	// Noop if token is already set
	switch initSource {
	case setAuthToken:
		log.Infof("function CreateAndSetAuthToken was called after SetAuthToken was called")
		return nil
	case createAndSetAuthToken:
		return nil
	}

	authTimeout := config.GetDuration("auth_init_timeout")
	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()
	log.Infof("starting to load the IPC auth primitives (timeout: %v)", authTimeout)

	var err error
	token, err = pkgtoken.FetchOrCreateAuthToken(ctx, config)
	if err != nil {
		return fmt.Errorf("error while creating or fetching auth token: %w", err)
	}
	ipccert, ipckey, err := cert.FetchOrCreateIPCCert(ctx, config)
	if err != nil {
		return fmt.Errorf("error while creating or fetching IPC cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ipccert); !ok {
		return fmt.Errorf("Unable to generate certPool from PERM IPC cert")
	}

	clientTLSConfig = &tls.Config{
		RootCAs: certPool,
	}

	tlsCert, err := tls.X509KeyPair(ipccert, ipckey)
	if err != nil {
		return fmt.Errorf("Unable to generate x509 cert from PERM IPC cert and key")
	}
	serverTLSConfig = &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}

	// printing the fingerprint of the loaded auth stack is useful to troubleshoot IPC issues
	printAuthSignature(token, ipccert, ipckey)
	initSource = createAndSetAuthToken

	return nil
}

// IsInitialized return true if the auth_token and IPC cert/key pair have been initialized with SetAuthToken or CreateAndSetAuthToken functions
func IsInitialized() bool {
	tokenLock.RLock()
	defer tokenLock.RUnlock()
	return initSource != uninitialized
}

// GetAuthToken gets the session token
func GetAuthToken() string {
	tokenLock.RLock()
	defer tokenLock.RUnlock()
	return token
}

// GetTLSClientConfig gets the certificate and key used for IPC
func GetTLSClientConfig() *tls.Config {
	tokenLock.RLock()
	defer tokenLock.RUnlock()
	if initSource == uninitialized {
		log.Warn("GetTLSClientConfig was called before being initialized (through SetAuthToken or CreateAndSetAuthToken function)")
	}
	return clientTLSConfig.Clone()
}

// GetTLSServerConfig gets the certificate and key used for IPC
func GetTLSServerConfig() *tls.Config {
	tokenLock.RLock()
	defer tokenLock.RUnlock()
	if initSource == uninitialized {
		log.Warn("GetTLSServerConfig was called before being initialized (through SetAuthToken or CreateAndSetAuthToken function), generating a self-signed certificate")
		config, err := generateSelfSignedCert()
		if err != nil {
			log.Error(err.Error())
		}
		serverTLSConfig = &config
	}
	return serverTLSConfig.Clone()
}

// InitDCAAuthToken initialize the session token for the Cluster Agent based on config options
// Requires that the config has been set up before calling
func InitDCAAuthToken(config model.Reader) error {
	tokenLock.Lock()
	defer tokenLock.Unlock()

	// Noop if dcaToken is already set
	if dcaToken != "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.GetDuration("auth_init_timeout"))
	defer cancel()

	var err error
	dcaToken, err = pkgtoken.CreateOrGetClusterAgentAuthToken(ctx, config)
	return err
}

// GetDCAAuthToken gets the session token
func GetDCAAuthToken() string {
	tokenLock.RLock()
	defer tokenLock.RUnlock()
	return dcaToken
}

// Validate validates an http request
func Validate(w http.ResponseWriter, r *http.Request) error {
	var err error
	auth := r.Header.Get("Authorization")
	if auth == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
		err = fmt.Errorf("no session token provided")
		http.Error(w, err.Error(), 401)
		return err
	}

	tok := strings.Split(auth, " ")
	if tok[0] != "Bearer" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
		err = fmt.Errorf("unsupported authorization scheme: %s", tok[0])
		http.Error(w, err.Error(), 401)
		return err
	}

	// The following comparison must be evaluated in constant time
	if len(tok) < 2 || !constantCompareStrings(tok[1], GetAuthToken()) {
		err = fmt.Errorf("invalid session token")
		http.Error(w, err.Error(), 403)
	}

	return err
}

// ValidateDCARequest is used for the exposed endpoints of the DCA.
// It is different from Validate as we want to have different validations.
func ValidateDCARequest(w http.ResponseWriter, r *http.Request) error {
	var err error
	auth := r.Header.Get("Authorization")
	if auth == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
		err = fmt.Errorf("no session token provided")
		http.Error(w, err.Error(), 401)
		return err
	}

	tok := strings.Split(auth, " ")
	if tok[0] != "Bearer" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="Datadog Agent"`)
		err = fmt.Errorf("unsupported authorization scheme: %s", tok[0])
		http.Error(w, err.Error(), 401)
		return err
	}

	// The following comparison must be evaluated in constant time
	if len(tok) != 2 || !constantCompareStrings(tok[1], GetDCAAuthToken()) {
		err = fmt.Errorf("invalid session token")
		http.Error(w, err.Error(), 403)
	}

	return err
}

// constantCompareStrings compares two strings in constant time.
// It uses the subtle.ConstantTimeCompare function from the crypto/subtle package
// to compare the byte slices of the input strings.
// Returns true if the strings are equal, false otherwise.
func constantCompareStrings(src, tgt string) bool {
	return subtle.ConstantTimeCompare([]byte(src), []byte(tgt)) == 1
}

// IsForbidden returns whether the cluster check runner server is allowed to listen on a given ip
// The function is a non-secure helper to help avoiding setting an IP that's too permissive.
// The function doesn't guarantee any security feature
func IsForbidden(ip string) bool {
	forbidden := map[string]bool{
		"":                true,
		"0.0.0.0":         true,
		"::":              true,
		"0:0:0:0:0:0:0:0": true,
	}
	return forbidden[ip]
}

// IsIPv6 is used to differentiate between ipv4 and ipv6 addresses.
func IsIPv6(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() == nil
}

func generateSelfSignedCert() (tls.Config, error) {
	// create cert
	hosts := []string{"127.0.0.1", "localhost", "::1"}
	_, rootCertPEM, rootKey, err := pkgtoken.GenerateRootCert(hosts, 2048)
	if err != nil {
		return tls.Config{}, fmt.Errorf("unable to generate a self-signed certificate: %v", err)
	}

	// PEM encode the private key
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})

	// Create a TLS cert using the private key and certificate
	rootTLSCert, err := tls.X509KeyPair(rootCertPEM, rootKeyPEM)
	if err != nil {
		return tls.Config{}, fmt.Errorf("unable to generate a self-signed certificate: %v", err)

	}

	return tls.Config{
		Certificates: []tls.Certificate{rootTLSCert},
	}, nil
}

// printAuthSignature computes and logs the authentication signature for the given token and IPC certificate/key.
// It uses SHA-256 to hash the concatenation of the token, IPC certificate, and IPC key.
func printAuthSignature(token string, ipccert, ipckey []byte) {
	h := sha256.New()

	_, err := h.Write(bytes.Join([][]byte{[]byte(token), ipccert, ipckey}, []byte{}))
	if err != nil {
		log.Warnf("error while computing auth signature: %v", err)
	}

	sign := h.Sum(nil)
	log.Infof("successfully loaded the IPC auth primitives (fingerprint: %.8x)", sign)
}
