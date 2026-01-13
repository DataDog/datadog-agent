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
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
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
	// jitterPercent is the percentage of jitter to add to refresh intervals (10%)
	// This prevents all agents from hitting the intake-key API at the same time
	jitterPercent = 0.10
)

// authInstance holds the state for a single delegated auth configuration (one API key target).
type authInstance struct {
	apiKey          *string
	provider        common.Provider
	authConfig      *common.AuthConfig
	refreshInterval time.Duration
	apiKeyConfigKey string // Configuration key where the API key should be written

	// Exponential backoff tracking
	consecutiveFailures int
	nextRetryInterval   time.Duration

	// Context and cancellation for background refresh goroutine
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
}

// delegatedAuthComponent implements the delegatedauth.Component interface.
//
// Thread-safety: This struct uses sync.RWMutex (mu) to protect concurrent access to all
// fields except config, which is immutable after construction.
type delegatedAuthComponent struct {
	// Immutable fields (safe for concurrent access without locking)
	config config.Component

	// Mutable fields (protected by mu)
	// Map of APIKeyConfigKey -> authInstance
	mu        sync.RWMutex
	instances map[string]*authInstance
}

// Provides list the provided interfaces from the delegatedauth Component
type Provides struct {
	Comp delegatedauth.Component
}

// NewComponent creates a new delegated auth Component
func NewComponent() Provides {
	comp := &delegatedAuthComponent{
		instances: make(map[string]*authInstance),
	}

	return Provides{
		Comp: comp,
	}
}

// addJitter adds random jitter to a duration to prevent thundering herd
// Returns a duration in the range [duration * (1 - jitterPercent), duration * (1 + jitterPercent)]
// For example, with jitterPercent=0.10 and duration=60m, returns a value between 54m and 66m
func addJitter(duration time.Duration) time.Duration {
	// Calculate the jitter range
	jitterRange := float64(duration) * jitterPercent
	// Generate a random value between -jitterRange and +jitterRange
	jitter := (rand.Float64()*2 - 1) * jitterRange
	// Add the jitter to the base duration
	return duration + time.Duration(jitter)
}

// Configure initializes delegated auth for a specific API key configuration.
// Can be called multiple times with different APIKeyConfigKey values.
func (d *delegatedAuthComponent) Configure(params delegatedauth.ConfigParams) {
	// Store the config component on first call
	if params.Config != nil && d.config == nil {
		d.config = params.Config.(config.Component)
	}

	// Determine the API key config key, defaulting to "api_key"
	apiKeyConfigKey := params.APIKeyConfigKey
	if apiKeyConfigKey == "" {
		apiKeyConfigKey = "api_key"
	}

	refreshInterval := time.Duration(params.RefreshInterval) * time.Minute
	if refreshInterval == 0 {
		// Default to 60 minutes if refresh interval was set incorrectly
		refreshInterval = 60 * time.Minute
		log.Warnf("Refresh interval was set to 0 for '%s', defaulting to 60 minutes", apiKeyConfigKey)
	}

	if params.OrgUUID == "" {
		log.Errorf("org_uuid is required when delegated_auth is enabled for '%s'", apiKeyConfigKey)
		return
	}

	// Auto-detect cloud provider if not specified
	provider := params.Provider
	awsRegion := params.AWSRegion

	if provider == "" {
		ctx := context.Background()
		if creds.IsRunningOnAWS(ctx) {
			provider = cloudauth.ProviderAWS
			log.Infof("Auto-detected AWS as cloud provider for '%s'", apiKeyConfigKey)

			// Auto-detect AWS region if not specified
			if awsRegion == "" {
				region, err := creds.GetAWSRegion(ctx)
				if err != nil {
					log.Warnf("Failed to auto-detect AWS region for '%s': %v. Will use default region.", apiKeyConfigKey, err)
				} else if region != "" {
					awsRegion = region
					log.Infof("Auto-detected AWS region '%s' for '%s'", awsRegion, apiKeyConfigKey)
				}
			}
		} else {
			log.Errorf("Could not auto-detect cloud provider for '%s'. Currently only 'aws' is supported.", apiKeyConfigKey)
			return
		}
	}

	// Create the appropriate provider
	var tokenProvider common.Provider
	switch provider {
	case cloudauth.ProviderAWS:
		tokenProvider = &cloudauth.AWSAuth{
			AwsRegion: awsRegion,
		}
	default:
		log.Errorf("unsupported delegated auth provider '%s' for '%s'. Currently only 'aws' is supported.", provider, apiKeyConfigKey)
		return
	}

	authConfig := &common.AuthConfig{
		OrgUUID: params.OrgUUID,
	}

	// Create a context for the background refresh goroutine
	refreshCtx, refreshCancel := context.WithCancel(context.Background())

	// Create new auth instance
	instance := &authInstance{
		provider:        tokenProvider,
		authConfig:      authConfig,
		refreshInterval: refreshInterval,
		apiKeyConfigKey: apiKeyConfigKey,
		refreshCtx:      refreshCtx,
		refreshCancel:   refreshCancel,
	}

	// Check if we're replacing an existing instance
	d.mu.Lock()
	if existingInstance, exists := d.instances[apiKeyConfigKey]; exists {
		log.Infof("Replacing existing delegated auth configuration for '%s'", apiKeyConfigKey)
		// Cancel the existing refresh goroutine
		if existingInstance.refreshCancel != nil {
			existingInstance.refreshCancel()
		}
	}
	d.instances[apiKeyConfigKey] = instance
	d.mu.Unlock()

	log.Infof("Delegated authentication is enabled for '%s', fetching initial API key...", apiKeyConfigKey)

	// Fetch the initial API key synchronously
	apiKey, _, err := d.refreshAndGetAPIKey(context.Background(), instance, false)
	if err != nil {
		log.Errorf("Failed to get initial delegated API key for '%s': %v", apiKeyConfigKey, err)
		// Track the initial failure for exponential backoff
		d.mu.Lock()
		instance.consecutiveFailures = 1
		d.mu.Unlock()
	} else {
		// Update the config with the initial API key
		d.updateConfigWithAPIKey(instance, *apiKey)
		log.Infof("Successfully fetched and set initial delegated API key for '%s'", apiKeyConfigKey)
	}

	// Always start the background refresh goroutine, even if initial fetch failed
	// This ensures retries will happen with exponential backoff
	d.startBackgroundRefresh(instance)
}

// refreshAndGetAPIKey is the internal implementation that can optionally force a refresh
func (d *delegatedAuthComponent) refreshAndGetAPIKey(_ context.Context, instance *authInstance, forceRefresh bool) (*string, bool, error) {
	// If not forcing refresh, check if we already have a cached key
	if !forceRefresh {
		d.mu.RLock()
		apiKey := instance.apiKey
		d.mu.RUnlock()

		if apiKey != nil {
			return apiKey, false, nil
		}
	}

	// Need to fetch a new key - acquire write lock
	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check pattern - another goroutine might have refreshed while we were waiting for the write lock
	if !forceRefresh && instance.apiKey != nil {
		return instance.apiKey, false, nil
	}

	log.Infof("Fetching delegated API key for '%s'", instance.apiKeyConfigKey)

	// Authenticate with the configured provider
	apiKey, err := d.authenticate(instance)
	if err != nil {
		log.Errorf("Failed to generate auth proof for '%s': %v", instance.apiKeyConfigKey, err)
		return nil, false, err
	}

	instance.apiKey = apiKey

	return apiKey, true, nil
}

// calculateNextRetryInterval calculates the next retry interval using exponential backoff
// First retry after failure is at the base interval, then doubles on each subsequent failure, capped at 1 hour
func (d *delegatedAuthComponent) calculateNextRetryInterval(instance *authInstance) time.Duration {
	// Base interval is the configured refresh interval
	baseInterval := instance.refreshInterval

	// Calculate exponential backoff: baseInterval * 2^max(0, consecutiveFailures-1)
	// This ensures the first retry is at the base interval, not doubled
	// Using math.Pow for clarity, though bit shifting could also be used
	exponent := float64(instance.consecutiveFailures - 1)
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
func (d *delegatedAuthComponent) startBackgroundRefresh(instance *authInstance) {
	// Start background refresh
	go func() {
		// Initialize with the configured refresh interval plus jitter
		d.mu.Lock()
		instance.nextRetryInterval = instance.refreshInterval
		d.mu.Unlock()

		// Add jitter to prevent all agents from hitting the API at the same time
		jitteredInterval := addJitter(instance.nextRetryInterval)
		ticker := time.NewTicker(jitteredInterval)
		defer ticker.Stop()

		for {
			select {
			case <-instance.refreshCtx.Done():
				// Context was canceled, exit the goroutine
				log.Debugf("Background refresh goroutine for '%s' exiting due to context cancellation", instance.apiKeyConfigKey)
				return
			case <-ticker.C:
				// Time to refresh
				lCreds, updated, lErr := d.refreshAndGetAPIKey(instance.refreshCtx, instance, true)

				d.mu.Lock()
				if lErr != nil {
					// Check if the error is due to context cancellation
					if instance.refreshCtx.Err() != nil {
						d.mu.Unlock()
						log.Debugf("Refresh for '%s' failed due to context cancellation, exiting", instance.apiKeyConfigKey)
						return
					}

					// Increment consecutive failures (capped to prevent overflow)
					if instance.consecutiveFailures < maxConsecutiveFailures {
						instance.consecutiveFailures++
					}
					instance.nextRetryInterval = d.calculateNextRetryInterval(instance)
					// Add jitter and log the actual retry time
					jitteredInterval := addJitter(instance.nextRetryInterval)
					log.Errorf("Failed to refresh delegated API key for '%s' (attempt %d): %v. Next retry in %v (base: %v)",
						instance.apiKeyConfigKey, instance.consecutiveFailures, lErr, jitteredInterval, instance.nextRetryInterval)
					ticker.Reset(jitteredInterval)
				} else {
					// Success - reset backoff
					if instance.consecutiveFailures > 0 {
						log.Infof("Successfully refreshed delegated API key for '%s' after %d failed attempts", instance.apiKeyConfigKey, instance.consecutiveFailures)
					}
					instance.consecutiveFailures = 0
					instance.nextRetryInterval = instance.refreshInterval

					// Update the config with the new API key
					if updated {
						d.updateConfigWithAPIKey(instance, *lCreds)
					}

					// Reset the ticker with the new interval plus jitter
					jitteredInterval := addJitter(instance.nextRetryInterval)
					ticker.Reset(jitteredInterval)
				}
				d.mu.Unlock()
			}
		}
	}()
}

// authenticate uses the configured provider to get creds
func (d *delegatedAuthComponent) authenticate(instance *authInstance) (*string, error) {
	key, err := instance.provider.GetAPIKey(d.config, instance.authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}
	return key, nil
}

// updateConfigWithAPIKey updates the config with the new API key
func (d *delegatedAuthComponent) updateConfigWithAPIKey(instance *authInstance, apiKey string) {
	// Update the config value using the Writer interface
	// This will trigger OnUpdate callbacks for any components listening to this config
	d.config.Set(instance.apiKeyConfigKey, apiKey, pkgconfigmodel.SourceAgentRuntime)
	log.Infof("Updated config key '%s' with new delegated API key", instance.apiKeyConfigKey)
}
