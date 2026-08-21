// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package certreloader

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// crlPEMTypes are the PEM block types that hold a certificate revocation list.
// "X509 CRL" is what RFC 7468 and OpenSSL emit; "CRL" is accepted because some
// PKI tooling writes the shorter label.
var crlPEMTypes = map[string]struct{}{
	"X509 CRL": {},
	"CRL":      {},
}

// CRLReloader manages a set of certificate revocation lists with automatic
// periodic reloading from disk. It is safe for concurrent use.
//
// On reload failure, the last successfully loaded lists are preserved and
// continue to be used. A revocation list that cannot be re-read is more useful
// than no revocation list at all: dropping it would silently re-admit every
// revoked certificate.
type CRLReloader struct {
	mu         sync.RWMutex
	clock      Clock
	crlFile    string
	crls       []*x509.RevocationList
	err        error
	loadErr    error
	lastUpdate time.Time
	crlFileMod time.Time
}

// NewCRLReloader creates a CRLReloader that immediately loads the revocation
// lists from disk and starts a background goroutine to periodically reload
// them. The background goroutine exits when ctx is cancelled.
func NewCRLReloader(ctx context.Context, crlFile string, clock Clock) *CRLReloader {
	r := &CRLReloader{
		crlFile: crlFile,
		clock:   clock,
	}
	r.reloadCRLs()
	go r.run(ctx)
	return r
}

// GetCRLs returns the currently loaded revocation lists.
func (r *CRLReloader) GetCRLs() ([]*x509.RevocationList, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.crls, r.err
}

func (r *CRLReloader) run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.shouldReload() {
				r.reloadCRLs()
			}
		}
	}
}

func (r *CRLReloader) shouldReload() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := r.clock.Now()
	if r.loadErr != nil {
		return now.After(r.lastUpdate.Add(errorCacheTimeout))
	}
	if !now.After(r.lastUpdate.Add(cacheTimeout)) {
		return false
	}
	return fileModified(r.crlFile, r.crlFileMod)
}

func (r *CRLReloader) reloadCRLs() {
	crls, err := LoadCRLs(r.crlFile)

	r.mu.Lock()
	r.loadErr = err
	if err == nil {
		r.crls = crls
		r.err = nil
		r.crlFileMod = fileMtime(r.crlFile)
		warnExpiredCRLs(r.crlFile, crls, r.clock.Now())
	} else if r.crls != nil {
		log.Warnf("Failed to reload CRLs from %s, continuing with previously loaded revocation lists: %v", r.crlFile, err)
	} else {
		r.err = err
	}
	r.lastUpdate = r.clock.Now()
	r.mu.Unlock()
}

// LoadCRLs reads a file containing one or more certificate revocation lists.
// Both PEM (possibly concatenated) and raw DER encodings are accepted, since
// CAs and PKI tooling emit either.
func LoadCRLs(crlFile string) ([]*x509.RevocationList, error) {
	data, err := os.ReadFile(crlFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CRL file: %w", err)
	}

	var crls []*x509.RevocationList
	for rest := data; len(rest) > 0; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if _, ok := crlPEMTypes[block.Type]; !ok {
			continue
		}
		crl, parseErr := x509.ParseRevocationList(block.Bytes)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse CRL in %q: %w", crlFile, parseErr)
		}
		crls = append(crls, crl)
	}

	if len(crls) == 0 {
		crl, derErr := x509.ParseRevocationList(data)
		if derErr != nil {
			return nil, fmt.Errorf("CRL file %q contains no valid PEM or DER revocation list: %w", crlFile, derErr)
		}
		crls = append(crls, crl)
	}

	return crls, nil
}

// warnExpiredCRLs flags revocation lists whose nextUpdate has passed. An
// expired list is still enforced — refusing to enforce it would re-admit
// revoked certificates — but it may be missing recent revocations, so the
// operator needs to know their CRL distribution has stalled.
func warnExpiredCRLs(crlFile string, crls []*x509.RevocationList, now time.Time) {
	for _, crl := range crls {
		if !crl.NextUpdate.IsZero() && now.After(crl.NextUpdate) {
			log.Warnf("CRL for issuer %s in %s expired at %s; its entries are still enforced but the list may be missing recent revocations",
				crl.Issuer, crlFile, crl.NextUpdate.UTC().Format(time.RFC3339))
		}
	}
}
