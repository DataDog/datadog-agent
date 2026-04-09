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

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   minVersion,
	}, nil
}

// Validate checks that the ServerConfig fields are internally consistent.
func (c *ServerConfig) Validate() error {
	if c.CertFile == "" || c.KeyFile == "" {
		return errors.New("tls requires both cert_file and key_file")
	}
	if c.MinVersion != 0 && c.MinVersion != tls.VersionTLS12 && c.MinVersion != tls.VersionTLS13 {
		return fmt.Errorf("unsupported TLS minimum version: %#x", c.MinVersion)
	}
	WarnKeyFilePermissions(c.KeyFile)
	return nil
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
