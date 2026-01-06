// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauthimpl implements the delegatedauth component interface
package delegatedauthimpl

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	delegatedauthpkg "github.com/DataDog/datadog-agent/pkg/delegatedauth"
	"github.com/DataDog/datadog-agent/pkg/delegatedauth/cloudauth"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// maxBackoffInterval is the maximum time to wait between retries (1 hour)
	maxBackoffInterval = time.Hour
	// maxConsecutiveFailures is the maximum number of failures we'll track to prevent overflow
	// Once we hit maxBackoffInterval, there's no point incrementing further
	// With a minimum reasonable refresh_interval of 1 minute: 1 * 2^(10-1) = 512 minutes > 60 minutes
	// So capping at 10 gives us plenty of headroom for any configuration
	maxConsecutiveFailures = 10
)

// delegatedAuthComponent implements the delegatedauth.Component interface.
//
// Thread-safety: This struct uses sync.RWMutex (mu) to protect concurrent access to all
// fields except config, which is immutable after construction.
type delegatedAuthComponent struct {
	// Immutable fields (safe for concurrent access without locking)
	config config.Component

	// Mutable fields (protected by mu)
	mu              sync.RWMutex
	apiKey          *string
	provider        delegatedauthpkg.Provider
	authConfig      *delegatedauthpkg.AuthConfig
	refreshInterval time.Duration

	// Exponential backoff tracking (protected by mu)
	consecutiveFailures int
	nextRetryInterval   time.Duration

	// Context and cancellation for background refresh goroutine (protected by mu for initialization)
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
}

// Provides list the provided interfaces from the delegatedauth Component
type Provides struct {
	Comp delegatedauth.Component
}

// NewComponent creates a new delegated auth Component
func NewComponent() Provides {
	comp := &delegatedAuthComponent{}

	return Provides{
		Comp: comp,
	}
}

// Configure initializes the delegated auth component with the provided configuration
func (d *delegatedAuthComponent) Configure(params delegatedauth.ConfigParams) {
	// Store the config for later use
	if params.Config != nil {
		d.config = params.Config.(config.Component)
	}

	if !params.Enabled {
		log.Info("Delegated authentication is disabled")
		return
	}

	refreshInterval := time.Duration(params.RefreshInterval) * time.Minute
	if refreshInterval == 0 {
		// Default to 60 minutes if refresh interval was set incorrectly
		refreshInterval = 60 * time.Minute
		log.Warn("Refresh interval was set to 0 defaulting to 60 minutes")
	}

	if params.OrgUUID == "" {
		log.Error("delegated_auth.org_uuid is required when delegated_auth.enabled is true")
		return
	}

	var tokenProvider delegatedauthpkg.Provider
	switch params.Provider {
	case cloudauth.ProviderAWS:
		tokenProvider = &cloudauth.AWSAuth{
			AwsRegion: params.AWSRegion,
		}
	default:
		log.Errorf("unsupported delegated auth provider: %s", params.Provider)
		return
	}

	authConfig := &delegatedauthpkg.AuthConfig{
		OrgUUID:      params.OrgUUID,
		Provider:     params.Provider,
		ProviderAuth: tokenProvider,
	}

	d.mu.Lock()
	d.provider = tokenProvider
	d.authConfig = authConfig
	d.refreshInterval = refreshInterval
	d.mu.Unlock()

	log.Info("Delegated authentication is enabled, fetching initial API key...")

	// Create a context for the background refresh goroutine
	d.refreshCtx, d.refreshCancel = context.WithCancel(context.Background())

	// Fetch the initial API key synchronously
	apiKey, _, err := d.refreshAndGetAPIKey(context.Background(), false)
	if err != nil {
		log.Errorf("Failed to get initial delegated API key: %v", err)
		// Track the initial failure for exponential backoff
		d.mu.Lock()
		d.consecutiveFailures = 1
		d.mu.Unlock()
	} else {
		// Update the config with the initial API key
		d.updateConfigWithAPIKey(*apiKey)
		log.Info("Successfully fetched and set initial delegated API key")
	}

	// Always start the background refresh goroutine, even if initial fetch failed
	// This ensures retries will happen with exponential backoff
	d.startBackgroundRefresh()
}

// refreshAndGetAPIKey is the internal implementation that can optionally force a refresh
func (d *delegatedAuthComponent) refreshAndGetAPIKey(_ context.Context, forceRefresh bool) (*string, bool, error) {
	// If not forcing refresh, check if we already have a cached key
	if !forceRefresh {
		d.mu.RLock()
		apiKey := d.apiKey
		d.mu.RUnlock()

		if apiKey != nil {
			return apiKey, false, nil
		}
	}

	// Need to fetch a new key - acquire write lock
	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check pattern - another goroutine might have refreshed while we were waiting for the write lock
	if !forceRefresh && d.apiKey != nil {
		return d.apiKey, false, nil
	}

	log.Info("Fetching delegated API key")

	// Authenticate with the configured provider
	apiKey, err := d.authenticate()
	if err != nil {
		log.Errorf("Failed to generate auth proof: %v", err)
		return nil, false, err
	}

	d.apiKey = apiKey

	return apiKey, true, nil
}

// calculateNextRetryInterval calculates the next retry interval using exponential backoff
// First retry after failure is at the base interval, then doubles on each subsequent failure, capped at 1 hour
func (d *delegatedAuthComponent) calculateNextRetryInterval() time.Duration {
	// Base interval is the configured refresh interval
	baseInterval := d.refreshInterval

	// Calculate exponential backoff: baseInterval * 2^max(0, consecutiveFailures-1)
	// This ensures the first retry is at the base interval, not doubled
	// Using math.Pow for clarity, though bit shifting could also be used
	exponent := float64(d.consecutiveFailures - 1)
	if exponent < 0 {
		exponent = 0
	}
	backoffMultiplier := math.Pow(2, exponent)
	backoffInterval := time.Duration(float64(baseInterval) * backoffMultiplier)

	// Cap at maximum backoff interval (1 hour)
	if backoffInterval > maxBackoffInterval {
		backoffInterval = maxBackoffInterval
	}

	return backoffInterval
}

// startBackgroundRefresh starts the background goroutine that periodically refreshes the API key
// with exponential backoff on failures
func (d *delegatedAuthComponent) startBackgroundRefresh() {
	// Start background refresh
	go func() {
		// Initialize with the configured refresh interval
		d.mu.Lock()
		d.nextRetryInterval = d.refreshInterval
		d.mu.Unlock()

		ticker := time.NewTicker(d.nextRetryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-d.refreshCtx.Done():
				// Context was canceled, exit the goroutine
				log.Debug("Background refresh goroutine exiting due to context cancellation")
				return
			case <-ticker.C:
				// Time to refresh
				lCreds, updated, lErr := d.refreshAndGetAPIKey(d.refreshCtx, true)

				d.mu.Lock()
				if lErr != nil {
					// Check if the error is due to context cancellation
					if d.refreshCtx.Err() != nil {
						d.mu.Unlock()
						log.Debug("Refresh failed due to context cancellation, exiting")
						return
					}

					// Increment consecutive failures (capped to prevent overflow)
					if d.consecutiveFailures < maxConsecutiveFailures {
						d.consecutiveFailures++
					}
					d.nextRetryInterval = d.calculateNextRetryInterval()
					log.Errorf("Failed to refresh delegated API key (attempt %d): %v. Next retry in %v",
						d.consecutiveFailures, lErr, d.nextRetryInterval)
				} else {
					// Success - reset backoff
					if d.consecutiveFailures > 0 {
						log.Infof("Successfully refreshed delegated API key after %d failed attempts", d.consecutiveFailures)
					}
					d.consecutiveFailures = 0
					d.nextRetryInterval = d.refreshInterval

					// Update the config with the new API key
					if updated {
						d.updateConfigWithAPIKey(*lCreds)
					}
				}

				// Reset the ticker with the new interval
				ticker.Reset(d.nextRetryInterval)
				d.mu.Unlock()
			}
		}
	}()
}

// authenticate uses the configured provider to get creds
func (d *delegatedAuthComponent) authenticate() (*string, error) {
	creds, err := d.authConfig.ProviderAuth.GetAPIKey(d.config, d.authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with AWS: %w", err)
	}
	return creds, nil
}

// Update the updateConfigWithAPIKey method to use the correct Set method
func (d *delegatedAuthComponent) updateConfigWithAPIKey(apiKey string) {
	// Update the api_key config value using the Writer interface
	// This will trigger OnUpdate callbacks for any components listening to this config
	d.config.Set("api_key", apiKey, pkgconfigmodel.SourceAgentRuntime)
	log.Infof("Updated config with new apiKey")
}
