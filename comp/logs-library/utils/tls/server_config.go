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

	"github.com/DataDog/datadog-agent/comp/logs-library/utils/tls/certreloader"
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
// for use with tls.NewListener. A CertReloader is created to support automatic
// certificate rotation without process restarts.
//
// When a CA file is configured, a CAReloader is used so that CA certificate
// rotation does not require a restart. Because tls.Config.ClientCAs cannot be
// safely mutated after use, we set ClientAuth to its non-verifying equivalent
// and perform CA verification in VerifyConnection against the
// dynamically-reloaded pool. This follows the pattern recommended by the Go
// crypto team: https://go.dev/issue/64796
func (c *ServerConfig) BuildTLSConfig(ctx context.Context) (*tls.Config, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	reloader := certreloader.New(ctx, c.CertFile, c.KeyFile, certreloader.RealClock())
	if _, err := reloader.GetCertificate(nil); err != nil {
		return nil, fmt.Errorf("failed to load TLS cert/key: %w", err)
	}

	minVersion := c.MinVersion
	if minVersion == 0 {
		minVersion = tls.VersionTLS12
	}

	tlsCfg := &tls.Config{
		GetCertificate: reloader.GetCertificate,
		MinVersion:     minVersion,
		ClientAuth:     c.ClientAuth,
	}

	if c.CAFile != "" {
		caReloader := certreloader.NewCAReloader(ctx, c.CAFile, certreloader.RealClock())
		if _, err := caReloader.GetPool(); err != nil {
			return nil, fmt.Errorf("failed to load TLS CA: %w", err)
		}
		tlsCfg.ClientAuth = clientAuthNoVerify(c.ClientAuth)
		tlsCfg.VerifyConnection = buildCAVerifier(caReloader)
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

// clientAuthNoVerify maps a verifying ClientAuthType to its non-verifying
// equivalent so that CA verification can be performed via VerifyConnection
// against a dynamically-reloaded pool.
func clientAuthNoVerify(auth tls.ClientAuthType) tls.ClientAuthType {
	switch auth {
	case tls.VerifyClientCertIfGiven:
		return tls.RequestClientCert
	case tls.RequireAndVerifyClientCert:
		return tls.RequireAnyClientCert
	default:
		return auth
	}
}

// buildCAVerifier returns a VerifyConnection callback that verifies client
// certificates against the CAReloader's current pool.
func buildCAVerifier(caReloader *certreloader.CAReloader) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return nil
		}
		pool, err := caReloader.GetPool()
		if err != nil {
			return fmt.Errorf("CA pool unavailable: %w", err)
		}

		intermediates := x509.NewCertPool()
		for _, cert := range cs.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}

		_, err = cs.PeerCertificates[0].Verify(x509.VerifyOptions{
			Roots:         pool,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		})
		return err
	}
}
