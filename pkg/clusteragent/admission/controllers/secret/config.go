// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package secret

import "time"

// CertConfig contains config parameters needed by
// the secret controller for certificate management
type CertConfig struct {
	expirationThreshold time.Duration
	validityBound       time.Duration
}

// NewCertConfig creates a certificate configuration
func NewCertConfig(expirationThreshold, validityBound time.Duration) CertConfig {
	return CertConfig{
		expirationThreshold: expirationThreshold,
		validityBound:       validityBound,
	}
}

// Config contains config parameters
// of the secret controller
type Config struct {
	ns   string
	name string
	svc  string
	cert CertConfig
}

// NewConfig creates a secret controller configuration
func NewConfig(ns, name, svc string, cert CertConfig) Config {
	return Config{
		ns:   ns,
		name: name,
		svc:  svc,
		cert: cert,
	}
}

// GetName returns the secret object name
func (s *Config) GetName() string { return s.name }

// GetNs returns secret object namespace
func (s *Config) GetNs() string { return s.ns }

// GetSvc returns the name of the targeted service
func (s *Config) GetSvc() string { return s.svc }

// GetCertExpiration returns the certificate's expiration threshold
// (how long before its expiration a certificate should be refreshed)
func (s *Config) GetCertExpiration() time.Duration { return s.cert.expirationThreshold }

// GetCertValidityBound returns the validity bound of the certificate
func (s *Config) GetCertValidityBound() time.Duration { return s.cert.validityBound }
