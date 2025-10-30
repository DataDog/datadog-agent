// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package external

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"k8s.io/utils/clock"
)

// TLSConfig represents the TLS configuration used for external recommender calls.
type TLSConfig struct {
	CAFile   string
	CertFile string
	KeyFile  string
}

func (c *TLSConfig) requiresClientCertificate() bool {
	return c != nil && (c.CertFile != "" || c.KeyFile != "")
}

const (
	certificateCacheTimeout      = 10 * time.Minute
	certificateErrorCacheTimeout = 1 * time.Minute
	certificateLoadRetryAttempts = 5
	certificateLoadRetrySleep    = 50 * time.Millisecond
	minTLSVersion                = tls.VersionTLS12
)

// configureTransportTLS updates the provided transport with a TLS configuration based on the given settings.
func configureTransportTLS(transport *http.Transport, config *TLSConfig, cache *tlsCertificateManager) error {
	if config == nil {
		return nil
	}

	tlsConfig, err := buildTLSConfig(config)
	if err != nil {
		return fmt.Errorf("failed to build TLS config: %w", err)
	}

	if config.requiresClientCertificate() {
		if cache == nil {
			return errors.New("tls certificate manager is required when using client certificates")
		}
		if config.CertFile == "" || config.KeyFile == "" {
			return errors.New("both cert file and key file must be provided for client TLS configuration")
		}
		tlsConfig.GetClientCertificate = cache.GetClientCertificateReloadingFunc()
	}

	transport.TLSClientConfig = tlsConfig
	return nil
}

func buildTLSConfig(config *TLSConfig) (*tls.Config, error) {
	var rootCA *x509.CertPool
	if config.CAFile != "" {
		caPEM, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		rootCA = x509.NewCertPool()
		if !rootCA.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to append root CA to pool")
		}
	}

	return &tls.Config{
		MinVersion: minTLSVersion,
		RootCAs:    rootCA,
	}, nil
}

// tlsCertificateManager manages a single certificate/key pair with automatic reloading.
type tlsCertificateManager struct {
	mu          sync.RWMutex
	certFile    string
	keyFile     string
	certificate *tls.Certificate
	err         error
	lastUpdate  time.Time
	clock       clock.Clock
}

func newTLSCertificateManager(clk clock.Clock, certFile, keyFile string) *tlsCertificateManager {
	manager := &tlsCertificateManager{
		certFile: certFile,
		keyFile:  keyFile,
		clock:    clk,
	}
	// Load certificate immediately
	manager.reloadCertificate()
	// Start background reloader
	go manager.run()
	return manager
}

func (c *tlsCertificateManager) GetClientCertificateReloadingFunc() func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	return func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.certificate, c.err
	}
}

func (c *tlsCertificateManager) run() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if c.shouldReload() {
			c.reloadCertificate()
		}
	}
}

func (c *tlsCertificateManager) shouldReload() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := c.clock.Now()

	// Reload if there was an error and enough time has passed
	if c.err != nil {
		return now.After(c.lastUpdate.Add(certificateErrorCacheTimeout))
	}

	// Reload if certificate is expired
	if c.certificate != nil && c.certificate.Leaf != nil && now.After(c.certificate.Leaf.NotAfter) {
		return true
	}

	// Reload periodically
	return now.After(c.lastUpdate.Add(certificateCacheTimeout))
}

func (c *tlsCertificateManager) reloadCertificate() {
	certificate, err := retryLoadingX509Keypair(
		certificateLoadRetryAttempts,
		certificateLoadRetrySleep,
		c.certFile,
		c.keyFile,
	)

	c.mu.Lock()
	c.certificate = certificate
	c.err = err
	c.lastUpdate = c.clock.Now()
	c.mu.Unlock()
}

func retryLoadingX509Keypair(attempts int, sleep time.Duration, certFile, keyFile string) (*tls.Certificate, error) {
	var err error
	for range attempts {
		certificate, loadErr := tls.LoadX509KeyPair(certFile, keyFile)
		if loadErr == nil {
			if len(certificate.Certificate) > 0 {
				if leaf, parseErr := x509.ParseCertificate(certificate.Certificate[0]); parseErr == nil {
					certificate.Leaf = leaf
				} else {
					return nil, fmt.Errorf("failed to parse certificate leaf: %w", parseErr)
				}
			}
			return &certificate, nil
		}

		err = loadErr

		if loadErr.Error() != "tls: private key does not match public key" {
			return nil, loadErr
		}

		time.Sleep(sleep)
	}
	return nil, fmt.Errorf("unable to load a matching certificate and key after %d attempts, last error: %w", attempts, err)
}
