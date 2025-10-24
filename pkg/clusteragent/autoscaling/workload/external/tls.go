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
)

// TLSConfig represents the TLS configuration used for external recommender calls.
type TLSConfig struct {
	CAFile             string
	CertFile           string
	KeyFile            string
	ServerName         string
	MinVersion         string
	MaxVersion         string
	InsecureSkipVerify bool
}

func (c *TLSConfig) requiresClientCertificate() bool {
	return c != nil && (c.CertFile != "" || c.KeyFile != "")
}

const (
	certificateCacheTimeout           = 10 * time.Minute
	certificateCacheExpirationTimeout = 10 * time.Minute
	certificateErrorCacheTimeout      = 1 * time.Minute
)

var tlsVersions = map[string]uint16{
	"TLS13": tls.VersionTLS13,
	"TLS12": tls.VersionTLS12,
	"TLS11": tls.VersionTLS11,
	"TLS10": tls.VersionTLS10,
}

// configureTransportTLS updates the provided transport with a TLS configuration based on the given settings.
func configureTransportTLS(transport *http.Transport, config *TLSConfig, cache *tlsCertificateCache) error {
	if config == nil {
		return nil
	}

	tlsConfig, err := buildTLSConfig(config)
	if err != nil {
		return fmt.Errorf("failed to build TLS config: %w", err)
	}

	if config.requiresClientCertificate() {
		if cache == nil {
			return errors.New("tls certificate cache is required when using client certificates")
		}
		if config.CertFile == "" || config.KeyFile == "" {
			return errors.New("both cert file and key file must be provided for client TLS configuration")
		}
		tlsConfig.GetClientCertificate = cache.GetClientCertificateReloadingFunc(config.CertFile, config.KeyFile)
	}

	transport.TLSClientConfig = tlsConfig
	return nil
}

func buildTLSConfig(config *TLSConfig) (*tls.Config, error) {
	var minVersion uint16
	if config.MinVersion != "" {
		var ok bool
		minVersion, ok = tlsVersions[config.MinVersion]
		if !ok {
			return nil, fmt.Errorf("unknown minimum TLS version: %s", config.MinVersion)
		}
	}

	var maxVersion uint16
	if config.MaxVersion != "" {
		var ok bool
		maxVersion, ok = tlsVersions[config.MaxVersion]
		if !ok {
			return nil, fmt.Errorf("unknown maximum TLS version: %s", config.MaxVersion)
		}
	}

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
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
		ServerName:         config.ServerName,
		RootCAs:            rootCA,
		InsecureSkipVerify: config.InsecureSkipVerify,
	}, nil
}

// tlsCertificateCache is an expiring cache to store certificates in memory used for TLS connections.
type tlsCertificateCache struct {
	mu    sync.RWMutex
	cache map[string]*tlsCacheEntry
}

func newTLSCertificateCache() *tlsCertificateCache {
	cache := &tlsCertificateCache{
		cache: make(map[string]*tlsCacheEntry),
	}
	go cache.run()
	return cache
}

type tlsCacheEntry struct {
	certificate *tls.Certificate
	err         error
	lastUpdate  time.Time
	lastAccess  time.Time
}

func (c *tlsCacheEntry) shouldReload(now time.Time) bool {
	if c.err != nil {
		return now.After(c.lastUpdate.Add(certificateErrorCacheTimeout))
	}

	if c.certificate != nil && c.isCertificateExpired(now) {
		return true
	}

	return c.isExpired(now, certificateCacheTimeout)
}

func (c *tlsCacheEntry) isExpired(now time.Time, duration time.Duration) bool {
	return now.After(c.lastUpdate.Add(duration))
}

func (c *tlsCacheEntry) isCertificateExpired(now time.Time) bool {
	return c.certificate != nil && c.certificate.Leaf != nil && now.After(c.certificate.Leaf.NotAfter)
}

func (c *tlsCertificateCache) GetClientCertificateReloadingFunc(certFile, keyFile string) func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	return func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		now := time.Now()

		c.mu.RLock()
		entry, ok := c.cache[certFile]
		c.mu.RUnlock()

		if !ok || entry.shouldReload(now) {
			certificate, err := retryLoadingX509Keypair(5, 50*time.Millisecond, certFile, keyFile)

			c.mu.Lock()
			entry = &tlsCacheEntry{
				certificate: certificate,
				err:         err,
				lastUpdate:  now,
				lastAccess:  now,
			}
			c.cache[certFile] = entry
			c.mu.Unlock()
		} else {
			c.mu.Lock()
			entry.lastAccess = now
			c.mu.Unlock()
		}

		return entry.certificate, entry.err
	}
}

func (c *tlsCertificateCache) run() {
	ticker := time.NewTicker(certificateCacheExpirationTimeout)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.cache {
			if now.After(entry.lastAccess.Add(certificateCacheExpirationTimeout)) {
				delete(c.cache, key)
			}
		}
		c.mu.Unlock()
	}
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
