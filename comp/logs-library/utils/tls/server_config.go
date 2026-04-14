// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tlsutil provides shared TLS configuration types and helpers for
// any agent component that needs a server-side TLS listener.
package tlsutil

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServerConfig holds typed TLS settings for a server-side TLS listener. All
// fields use concrete Go crypto types rather than user-facing strings; the
// calling config layer is responsible for parsing and validating raw input
// before constructing a ServerConfig.
type ServerConfig struct {
	CertFile   string
	KeyFile    string
	CAFile     string
	ClientAuth tls.ClientAuthType
	MinVersion uint16
}

// BuildTLSConfig loads certificates from disk and returns a *tls.Config ready
// for use with tls.NewListener.
func (c *ServerConfig) BuildTLSConfig(_ context.Context) (*tls.Config, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS cert/key: %w", err)
	}

	minVersion := c.MinVersion
	if minVersion == 0 {
		minVersion = tls.VersionTLS12
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   minVersion,
		ClientAuth:   c.ClientAuth,
	}

	if c.CAFile != "" {
		pool, err := loadCACertPool(c.CAFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.ClientCAs = pool
	}

	return tlsCfg, nil
}

// Validate checks that the ServerConfig fields are internally consistent.
func (c *ServerConfig) Validate() error {
	if c.CertFile == "" || c.KeyFile == "" {
		return errors.New("tls requires both cert_file and key_file")
	}
	if c.MinVersion != 0 && c.MinVersion != tls.VersionTLS12 && c.MinVersion != tls.VersionTLS13 {
		return fmt.Errorf("unsupported TLS minimum version: %#x", c.MinVersion)
	}
	switch c.ClientAuth {
	case tls.NoClientCert, tls.RequestClientCert, tls.RequireAnyClientCert,
		tls.VerifyClientCertIfGiven, tls.RequireAndVerifyClientCert:
	default:
		return fmt.Errorf("unsupported TLS client auth type: %d", c.ClientAuth)
	}
	if ClientAuthRequiresVerification(c.ClientAuth) && c.CAFile == "" {
		return errors.New("tls client_auth requires ca_file to be set")
	}
	WarnKeyFilePermissions(c.KeyFile)
	return nil
}

// ClientAuthRequiresVerification returns true if the given client auth type
// requires a CA certificate for client verification.
func ClientAuthRequiresVerification(auth tls.ClientAuthType) bool {
	switch auth {
	case tls.VerifyClientCertIfGiven, tls.RequireAndVerifyClientCert:
		return true
	default:
		return false
	}
}

// WarnKeyFilePermissions checks if the TLS private key file is readable by
// group or others and emits a warning if so.
func WarnKeyFilePermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		log.Warnf("TLS key file %q has permissions %04o; recommended permissions are 0600", path, mode)
	}
}

func loadCACertPool(path string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("CA file %q contains no valid certificates", path)
	}
	return pool, nil
}
