// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package certreloader provides automatic TLS certificate reloading from disk.
// It periodically checks and reloads cert/key pairs so that certificate
// rotation does not require a process restart.
package certreloader

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheTimeout      = 10 * time.Minute
	errorCacheTimeout = 1 * time.Minute
	tickInterval      = 1 * time.Minute
	loadRetryAttempts = 5
	loadRetrySleep    = 50 * time.Millisecond
)

// Clock provides the current time. It is satisfied by k8s.io/utils/clock.Clock
// and clocktesting.FakeClock without introducing that dependency.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RealClock returns a Clock backed by time.Now.
func RealClock() Clock { return realClock{} }

// CertReloader manages a single certificate/key pair with automatic periodic
// reloading. It is safe for concurrent use.
//
// On reload failure, the last successfully loaded certificate is preserved
// and continues to be served. This follows the same pattern used by gRPC-Go's
// advancedtls pemfile watcher, nginx, and Envoy: a transient disk error should
// not take down TLS serving when a valid certificate is already in memory.
type CertReloader struct {
	mu          sync.RWMutex
	clock       Clock
	certFile    string
	keyFile     string
	certificate *tls.Certificate
	err         error
	loadErr     error
	lastUpdate  time.Time
}

// New creates a CertReloader that immediately loads the cert/key pair from
// disk and starts a background goroutine to periodically reload it. The
// background goroutine exits when ctx is cancelled.
func New(ctx context.Context, certFile, keyFile string, clock Clock) *CertReloader {
	r := &CertReloader{
		certFile: certFile,
		keyFile:  keyFile,
		clock:    clock,
	}
	r.reloadCertificate()
	go r.run(ctx)
	return r
}

// GetCertificate returns the current certificate for use as a
// tls.Config.GetCertificate callback (server-side).
func (r *CertReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.certificate, r.err
}

// GetClientCertificate returns the current certificate for use as a
// tls.Config.GetClientCertificate callback (client-side).
func (r *CertReloader) GetClientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.certificate, r.err
}

func (r *CertReloader) run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.shouldReload() {
				r.reloadCertificate()
			}
		}
	}
}

func (r *CertReloader) shouldReload() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := r.clock.Now()

	if r.loadErr != nil {
		return now.After(r.lastUpdate.Add(errorCacheTimeout))
	}

	if r.certificate != nil && r.certificate.Leaf != nil && now.After(r.certificate.Leaf.NotAfter) {
		return true
	}

	return now.After(r.lastUpdate.Add(cacheTimeout))
}

func (r *CertReloader) reloadCertificate() {
	certificate, err := retryLoadX509KeyPair(loadRetryAttempts, loadRetrySleep, r.certFile, r.keyFile)

	r.mu.Lock()
	r.loadErr = err
	if err == nil {
		r.certificate = certificate
		r.err = nil
	} else if r.certificate != nil {
		log.Warnf("Failed to reload TLS certificate from %s / %s, continuing with previously loaded certificate: %v", r.certFile, r.keyFile, err)
	} else {
		r.err = err
	}
	r.lastUpdate = r.clock.Now()
	r.mu.Unlock()
}

// retryLoadX509KeyPair attempts to load the cert/key pair, retrying on
// key mismatch errors that can occur when cert and key files are written
// non-atomically during rotation.
func retryLoadX509KeyPair(attempts int, sleep time.Duration, certFile, keyFile string) (*tls.Certificate, error) {
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
