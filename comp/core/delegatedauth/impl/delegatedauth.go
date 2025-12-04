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

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	delegatedauthpkg "github.com/DataDog/datadog-agent/pkg/delegatedauth"
	"github.com/DataDog/datadog-agent/pkg/delegatedauth/cloudauth"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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

type delegatedAuthComponent struct {
	config config.Component
	log    log.Component

	mu              sync.RWMutex
	apiKey          *string
	provider        delegatedauthpkg.Provider
	authConfig      *delegatedauthpkg.AuthConfig
	refreshInterval time.Duration

	// Exponential backoff tracking
	consecutiveFailures int
	nextRetryInterval   time.Duration

	// Context and cancellation for background refresh goroutine
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
}

type dependencies struct {
	fx.In
	Config config.Component
	Log    log.Component
	Lc     fx.Lifecycle
}

// NewDelegatedAuth creates a new delegated auth Component based on the current configuration
func NewDelegatedAuth(deps dependencies) option.Option[delegatedauth.Component] {
	if !deps.Config.GetBool("delegated_auth.enabled") {
		deps.Log.Info("Delegated authentication is disabled")
		return option.None[delegatedauth.Component]()
	}

	provider := deps.Config.GetString("delegated_auth.provider")
	orgUUID := deps.Config.GetString("delegated_auth.org_uuid")
	refreshInterval := deps.Config.GetDuration("delegated_auth.refresh_interval_mins") * time.Minute

	if refreshInterval == 0 {
		// Default to 60 minutes if refresh interval was set incorrectly
		refreshInterval = 60 * time.Minute
		deps.Log.Warn("Refresh interval was set to 0 defaulting to 60 minutes")
	}

	if orgUUID == "" {
		deps.Log.Error("delegated_auth.org_uuid is required when delegated_auth.enabled is true")
		return option.None[delegatedauth.Component]()
	}

	var tokenProvider delegatedauthpkg.Provider
	switch provider {
	case cloudauth.ProviderAWS:
		tokenProvider = &cloudauth.AWSAuth{
			AwsRegion: deps.Config.GetString("delegated_auth.aws_region"),
		}
	default:
		deps.Log.Errorf("unsupported delegated auth provider: %s", provider)
		return option.None[delegatedauth.Component]()
	}

	authConfig := &delegatedauthpkg.AuthConfig{
		OrgUUID:      orgUUID,
		Provider:     provider,
		ProviderAuth: tokenProvider,
	}

	comp := &delegatedAuthComponent{
		config:          deps.Config,
		log:             deps.Log,
		provider:        tokenProvider,
		authConfig:      authConfig,
		refreshInterval: refreshInterval,
	}

	// Register lifecycle hooks to ensure API key is fetched early
	deps.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			comp.log.Info("Delegated authentication is enabled, fetching initial API key...")

			// Create a context for the background refresh goroutine
			comp.refreshCtx, comp.refreshCancel = context.WithCancel(context.Background())

			// Fetch the initial API key synchronously during startup
			apiKey, err := comp.GetAPIKey(ctx)
			if err != nil {
				comp.log.Errorf("Failed to get initial delegated API key: %v", err)
				// Track the initial failure for exponential backoff
				comp.mu.Lock()
				comp.consecutiveFailures = 1
				comp.mu.Unlock()
			} else {
				// Update the config with the initial API key
				comp.updateConfigWithAPIKey(*apiKey)
				comp.log.Info("Successfully fetched and set initial delegated API key")
			}

			// Always start the background refresh goroutine, even if initial fetch failed
			// This ensures retries will happen with exponential backoff
			comp.startBackgroundRefresh()

			return nil
		},
		OnStop: func(_ context.Context) error {
			comp.log.Info("Stopping delegated auth background refresh...")

			// Cancel the background refresh context
			if comp.refreshCancel != nil {
				comp.refreshCancel()
			}

			comp.log.Info("Delegated auth background refresh stopped")
			return nil
		},
	})

	return option.New[delegatedauth.Component](comp)
}

// GetAPIKey returns the current API key or fetches one if it has not yet been fetched
func (d *delegatedAuthComponent) GetAPIKey(ctx context.Context) (*string, error) {
	creds, _, err := d.refreshAndGetAPIKey(ctx, false)
	return creds, err
}

// RefreshAPIKey fetches the API key and stores it in the component. It only returns an error if there is an issue
func (d *delegatedAuthComponent) RefreshAPIKey(ctx context.Context) error {
	_, _, err := d.refreshAndGetAPIKey(ctx, true)
	return err
}

// RefreshAndGetAPIKey refreshes the API key and stores it in the component it returns the current API key and if a refresh actually occurred
func (d *delegatedAuthComponent) RefreshAndGetAPIKey(ctx context.Context) (*string, bool, error) {
	return d.refreshAndGetAPIKey(ctx, true)
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

	d.log.Info("Fetching delegated API key")

	// Authenticate with the configured provider
	apiKey, err := d.authenticate()
	if err != nil {
		d.log.Errorf("Failed to generate auth proof: %v", err)
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
				d.log.Debug("Background refresh goroutine exiting due to context cancellation")
				return
			case <-ticker.C:
				// Time to refresh
				lCreds, updated, lErr := d.RefreshAndGetAPIKey(d.refreshCtx)

				d.mu.Lock()
				if lErr != nil {
					// Check if the error is due to context cancellation
					if d.refreshCtx.Err() != nil {
						d.mu.Unlock()
						d.log.Debug("Refresh failed due to context cancellation, exiting")
						return
					}

					// Increment consecutive failures (capped to prevent overflow)
					if d.consecutiveFailures < maxConsecutiveFailures {
						d.consecutiveFailures++
					}
					d.nextRetryInterval = d.calculateNextRetryInterval()
					d.log.Errorf("Failed to refresh delegated API key (attempt %d): %v. Next retry in %v",
						d.consecutiveFailures, lErr, d.nextRetryInterval)
				} else {
					// Success - reset backoff
					if d.consecutiveFailures > 0 {
						d.log.Infof("Successfully refreshed delegated API key after %d failed attempts", d.consecutiveFailures)
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
	d.log.Infof("Updated config with new apiKey")
}
