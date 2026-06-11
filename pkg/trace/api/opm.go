// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// validateResponse is the subset of the /api/v2/validate response we care about.
type validateResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// computeOPM derives the Org Propagation Marker from an org UUID string.
//
// OPM = base64url( Trunc60( SHA-256(orgUUID) ) )
//
// SHA-256 produces a 32-byte digest. Trunc60 keeps the first 60 bits:
// the first 7 bytes plus the high nibble of byte 7 (mask byte 7 with 0xF0).
func computeOPM(orgUUID string) string {
	digest := sha256.Sum256([]byte(orgUUID))
	trunc := make([]byte, 8)
	copy(trunc, digest[:8])
	trunc[7] &= 0xF0
	return base64.RawURLEncoding.EncodeToString(trunc)
}

// fetchOPM performs a single GET to cfg.OPMValidateURL, decodes the org UUID,
// and returns the computed OPM. The request uses cfg.APIKey() for authentication
// and is made through the configured HTTP client (proxy-aware).
func fetchOPM(ctx context.Context, client *config.ResetClient, cfg *config.AgentConfig) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.OPMValidateURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("DD-API-KEY", cfg.APIKey())

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var vr validateResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if vr.Data.ID == "" {
		return "", errors.New("empty org UUID in validate response")
	}
	return computeOPM(vr.Data.ID), nil
}

// setOrgPropMarker stores opm atomically and updates the agentState hash and
// cached response body. The orgPropMarker write is outside the mutex (it is an
// atomic.String so always safe), but the body/hash writes are held under
// computeInfoAndHashMu so they cannot interleave with makeInfoHandler's own
// initialisation. If computeInfoAndHash has not yet been set by makeInfoHandler,
// the update is skipped — makeInfoHandler reads orgPropMarker under the same
// mutex, so it will pick up the already-stored value and initialise both fields
// correctly.
func (r *HTTPReceiver) setOrgPropMarker(opm string) {
	r.orgPropMarker.Store(opm)
	r.computeInfoAndHashMu.Lock()
	if fn := r.computeInfoAndHash; fn != nil {
		body, hash := fn(opm)
		r.cachedInfoResponse.Store(body)
		r.agentState.Store(hash)
	}
	r.computeInfoAndHashMu.Unlock()
}

// OrgPropMarker returns the current Org Propagation Marker, or the empty string
// if it has not yet been fetched.
func (r *HTTPReceiver) OrgPropMarker() string {
	return r.orgPropMarker.Load()
}

// StartOPMFetch begins a background goroutine that fetches /api/v2/validate,
// computes the OPM, and stores it on the receiver. It retries up to maxRetries
// times with simple exponential backoff. retryBaseDelay controls the first
// inter-attempt pause (doubled each retry); pass time.Second in production and
// time.Millisecond in tests.
//
// The goroutine exits immediately if cfg.EnableOPMFetch is false, or when ctx
// is cancelled (e.g. on agent shutdown).
func (r *HTTPReceiver) StartOPMFetch(ctx context.Context, retryBaseDelay time.Duration) {
	if !r.conf.EnableOPMFetch {
		return
	}
	go func() {
		const maxRetries = 3
		client := r.conf.NewHTTPClient()

		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				delay := retryBaseDelay * (1 << (attempt - 1)) // base, 2×, 4×
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return
				}
			}

			opm, err := fetchOPM(ctx, client, r.conf)
			if err != nil {
				log.Warnf("OPM fetch attempt %d/%d failed: %v", attempt+1, maxRetries+1, err)
				continue
			}
			r.setOrgPropMarker(opm)
			log.Debugf("Org Propagation Marker set: %s", opm)
			return
		}
		log.Warnf("OPM fetch failed after %d attempts, org_prop_marker will not be set", maxRetries+1)
	}()
}
