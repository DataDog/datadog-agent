// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package certreloader

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// CAReloader manages a CA certificate pool with automatic periodic reloading
// from disk. It is safe for concurrent use.
type CAReloader struct {
	mu         sync.RWMutex
	clock      Clock
	caFile     string
	pool       *x509.CertPool
	err        error
	lastUpdate time.Time
}

// NewCAReloader creates a CAReloader that immediately loads the CA certificates
// from disk and starts a background goroutine to periodically reload them. The
// background goroutine exits when ctx is cancelled.
func NewCAReloader(ctx context.Context, caFile string, clock Clock) *CAReloader {
	r := &CAReloader{
		caFile: caFile,
		clock:  clock,
	}
	r.reloadCA()
	go r.run(ctx)
	return r
}

// GetPool returns the current CA certificate pool.
func (r *CAReloader) GetPool() (*x509.CertPool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pool, r.err
}

func (r *CAReloader) run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.shouldReload() {
				r.reloadCA()
			}
		}
	}
}

func (r *CAReloader) shouldReload() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := r.clock.Now()
	if r.err != nil {
		return now.After(r.lastUpdate.Add(errorCacheTimeout))
	}
	return now.After(r.lastUpdate.Add(cacheTimeout))
}

func (r *CAReloader) reloadCA() {
	pool, err := loadCACertPool(r.caFile)

	r.mu.Lock()
	r.pool = pool
	r.err = err
	r.lastUpdate = r.clock.Now()
	r.mu.Unlock()
}

func loadCACertPool(caFile string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to parse CA certificates")
	}
	return pool, nil
}
