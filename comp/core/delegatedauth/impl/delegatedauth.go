// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauthimpl implements the delegatedauth component interface
package delegatedauthimpl

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/api"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/aws"
	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

//go:embed status_templates
var templatesFS embed.FS

const (
	// maxBackoffInterval is the maximum time to wait between retries (1 hour)
	maxBackoffInterval = time.Hour
	// backoffRandomizationFactor is the percentage of jitter to add to refresh intervals
	// This prevents all agents from hitting the intake-key API at the same time
	backoffRandomizationFactor = 0.10
)

// authInstance holds the state for a single delegated auth configuration (one API key target).
type authInstance struct {
	apiKey          *string
	provider        common.Provider
	authConfig      *common.AuthConfig
	refreshInterval time.Duration
	apiKeyConfigKey string // Configuration key where the API key should be written

	// Exponential backoff for retry intervals
	backoff *backoff.ExponentialBackOff

	// consecutiveFailures tracks failures for status reporting
	consecutiveFailures int

	// Context and cancellation for background refresh goroutine
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
}

// delegatedAuthComponent implements the delegatedauth.Component interface.
//
// Thread-safety: This struct uses sync.RWMutex (mu) to protect concurrent access to all
// mutable fields.
type delegatedAuthComponent struct {
	// Mutable fields (protected by mu)
	mu               sync.RWMutex
	config           pkgconfigmodel.ReaderWriter
	instances        map[string]*authInstance // Map of APIKeyConfigKey -> authInstance
	initialized      bool                     // Whether Initialize() has been called
	providerConfig   common.ProviderConfig    // Resolved provider configuration
	resolvedProvider string                   // Resolved provider name (e.g., "aws") - for status display
}

// Provides list the provided interfaces from the delegatedauth Component
type Provides struct {
	Comp           delegatedauth.Component
	StatusProvider status.InformationProvider
}

// NewComponent creates a new delegated auth Component
func NewComponent() Provides {
	comp := &delegatedAuthComponent{
		instances: make(map[string]*authInstance),
	}

	return Provides{
		Comp:           comp,
		StatusProvider: status.NewInformationProvider(comp),
	}
}

// newBackoff creates an ExponentialBackOff configured for delegated auth refresh.
// It uses the refresh interval as the initial interval and caps at maxBackoffInterval.
func newBackoff(refreshInterval time.Duration) *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = refreshInterval
	b.MaxInterval = maxBackoffInterval
	b.Multiplier = 2.0
	b.RandomizationFactor = backoffRandomizationFactor
	b.Reset()
	return b
}

// initializeIfNeeded performs lazy initialization on first AddInstance call.
// Returns the provider config if initialized, or nil if not available.
// This function performs cloud detection without holding locks to avoid blocking during network I/O.
func (d *delegatedAuthComponent) initializeIfNeeded(ctx context.Context, params delegatedauth.InstanceParams) (common.ProviderConfig, error) {
	// Quick check with read lock - if already initialized, return current config
	d.mu.RLock()
	if d.initialized {
		providerConfig := d.providerConfig
		storedConfig := d.config
		d.mu.RUnlock()
		// Warn if a different config is passed on subsequent calls
		if storedConfig != params.Config {
			log.Warnf("AddInstance called with different Config than the first call; the new Config will be ignored. Only the Config from the first AddInstance call is used.")
		}
		return providerConfig, nil
	}
	d.mu.RUnlock()

	// Need to initialize - first detect the cloud provider WITHOUT holding the lock
	// to avoid blocking during IMDS network calls
	var detectedConfig common.ProviderConfig
	var resolvedProvider string

	// If provider config is explicitly specified, use it
	if params.ProviderConfig != nil {
		detectedConfig = params.ProviderConfig
		resolvedProvider = params.ProviderConfig.ProviderName()
		log.Infof("Using explicitly configured cloud provider '%s' for delegated auth", resolvedProvider)
	} else {
		// Auto-detect cloud provider (network I/O happens here, outside any lock)
		if creds.IsRunningOnAWS(ctx) {
			log.Info("Auto-detected AWS as cloud provider for delegated auth")

			// Auto-detect AWS region
			awsRegion := ""
			region, err := creds.GetAWSRegion(ctx)
			if err != nil {
				log.Warnf("Failed to auto-detect AWS region: %v. Will use default region.", err)
			} else if region != "" {
				awsRegion = region
				log.Infof("Auto-detected AWS region: %s", awsRegion)
			}

			detectedConfig = &cloudauthconfig.AWSProviderConfig{
				Region: awsRegion,
			}
			resolvedProvider = cloudauthconfig.ProviderAWS
		} else {
			// No supported cloud provider detected - delegated auth will be disabled
			log.Debug("No supported cloud provider detected for delegated auth, feature will be disabled")
			detectedConfig = nil
			resolvedProvider = ""
		}
	}

	// Now acquire write lock to update state
	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check pattern - another goroutine might have initialized while we were detecting
	if d.initialized {
		return d.providerConfig, nil
	}

	// Store the config and detected provider
	d.config = params.Config
	d.providerConfig = detectedConfig
	d.resolvedProvider = resolvedProvider
	d.initialized = true

	return d.providerConfig, nil
}

// AddInstance configures delegated auth for a specific API key.
// On the first call, it detects the cloud provider and initializes the component.
func (d *delegatedAuthComponent) AddInstance(params delegatedauth.InstanceParams) error {
	// Validate required parameters first
	if params.Config == nil {
		return errors.New("config is required")
	}
	if params.OrgUUID == "" {
		return errors.New("org_uuid is required")
	}
	if params.APIKeyConfigKey == "" {
		return errors.New("api_key_config_key is required")
	}

	// Initialize on first call - this detects cloud provider without holding locks
	ctx := context.Background()
	providerConfig, err := d.initializeIfNeeded(ctx, params)
	if err != nil {
		return err
	}

	// If no provider is configured (unsupported cloud or not running in cloud),
	// silently skip - the agent will use whatever API key is already configured
	if providerConfig == nil {
		log.Debugf("Delegated auth not available (no supported cloud provider), skipping configuration for '%s'", params.APIKeyConfigKey)
		return nil
	}

	apiKeyConfigKey := params.APIKeyConfigKey

	refreshInterval := time.Duration(params.RefreshInterval) * time.Minute
	if refreshInterval == 0 {
		// Default to 60 minutes if refresh interval was set incorrectly
		refreshInterval = 60 * time.Minute
		log.Warnf("Refresh interval was set to 0 for '%s', defaulting to 60 minutes", apiKeyConfigKey)
	}

	// Create the appropriate provider based on the provider config type
	var tokenProvider common.Provider
	switch cfg := providerConfig.(type) {
	case *cloudauthconfig.AWSProviderConfig:
		tokenProvider = aws.NewAWSAuth(cfg)
	default:
		return fmt.Errorf("unsupported delegated auth provider config type: %T", providerConfig)
	}

	authConfig := &common.AuthConfig{
		OrgUUID: params.OrgUUID,
	}

	// Create a context for the background refresh goroutine
	refreshCtx, refreshCancel := context.WithCancel(context.Background())

	// Create new auth instance with backoff configured
	instance := &authInstance{
		provider:        tokenProvider,
		authConfig:      authConfig,
		refreshInterval: refreshInterval,
		apiKeyConfigKey: apiKeyConfigKey,
		backoff:         newBackoff(refreshInterval),
		refreshCtx:      refreshCtx,
		refreshCancel:   refreshCancel,
	}

	// Check if we're replacing an existing instance.
	// This is expected behavior - callers may reconfigure delegated auth (e.g., with different org UUID
	// or refresh interval). When this happens, we cancel the old refresh goroutine and start a new one.
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
		// Backoff will be used for retry interval in startBackgroundRefresh
	} else {
		// Update the config with the initial API key
		d.updateConfigWithAPIKey(instance, *apiKey)
		log.Infof("Successfully fetched and set initial delegated API key for '%s'", apiKeyConfigKey)
	}

	// Always start the background refresh goroutine, even if initial fetch failed
	// This ensures retries will happen with exponential backoff
	d.startBackgroundRefresh(instance)

	return nil
}

// refreshAndGetAPIKey is the internal implementation that can optionally force a refresh
func (d *delegatedAuthComponent) refreshAndGetAPIKey(ctx context.Context, instance *authInstance, forceRefresh bool) (*string, bool, error) {
	// If not forcing refresh, check if we already have a cached key
	if !forceRefresh {
		d.mu.RLock()
		apiKey := instance.apiKey
		d.mu.RUnlock()

		if apiKey != nil {
			return apiKey, false, nil
		}
	}

	// Double-check pattern with brief lock - another goroutine might be refreshing
	d.mu.RLock()
	if !forceRefresh && instance.apiKey != nil {
		apiKey := instance.apiKey
		d.mu.RUnlock()
		return apiKey, false, nil
	}
	d.mu.RUnlock()

	log.Infof("Fetching delegated API key for '%s'", instance.apiKeyConfigKey)

	// Authenticate with the configured provider - done WITHOUT holding the lock
	// to avoid blocking other goroutines during network I/O
	apiKey, err := d.authenticate(ctx, instance)
	if err != nil {
		log.Errorf("Failed to generate auth proof for '%s': %v", instance.apiKeyConfigKey, err)
		return nil, false, err
	}

	// Now acquire write lock briefly to update state
	d.mu.Lock()
	instance.apiKey = apiKey
	d.mu.Unlock()

	return apiKey, true, nil
}

// startBackgroundRefresh starts the background goroutine that periodically refreshes the API key
// with exponential backoff on failures
func (d *delegatedAuthComponent) startBackgroundRefresh(instance *authInstance) {
	go func() {
		// Get initial interval with jitter from backoff
		d.mu.Lock()
		nextInterval := instance.backoff.NextBackOff()
		d.mu.Unlock()

		ticker := time.NewTicker(nextInterval)
		defer ticker.Stop()

		for {
			select {
			case <-instance.refreshCtx.Done():
				log.Debugf("Background refresh goroutine for '%s' exiting due to context cancellation", instance.apiKeyConfigKey)
				return
			case <-ticker.C:
				lCreds, updated, lErr := d.refreshAndGetAPIKey(instance.refreshCtx, instance, true)

				// Variables to capture state updates
				var shouldUpdateConfig bool
				var apiKeyToUpdate string

				d.mu.Lock()
				if lErr != nil {
					// Check if the error is due to context cancellation
					if instance.refreshCtx.Err() != nil {
						d.mu.Unlock()
						log.Debugf("Refresh for '%s' failed due to context cancellation, exiting", instance.apiKeyConfigKey)
						return
					}

					// Track failures for status reporting
					instance.consecutiveFailures++

					// Get next backoff interval (exponentially increasing with jitter)
					nextInterval := instance.backoff.NextBackOff()
					log.Errorf("Failed to refresh delegated API key for '%s' (attempt %d): %v. Next retry in %v",
						instance.apiKeyConfigKey, instance.consecutiveFailures, lErr, nextInterval)
					ticker.Reset(nextInterval)
				} else {
					// Success - reset backoff and failure counter
					if instance.consecutiveFailures > 0 {
						log.Infof("Successfully refreshed delegated API key for '%s' after %d failed attempts",
							instance.apiKeyConfigKey, instance.consecutiveFailures)
					}
					instance.consecutiveFailures = 0
					instance.backoff.Reset()
					nextInterval := instance.backoff.NextBackOff()

					// Capture the API key to update config outside the lock
					if updated && lCreds != nil {
						shouldUpdateConfig = true
						apiKeyToUpdate = *lCreds
					}

					ticker.Reset(nextInterval)
				}
				d.mu.Unlock()

				// Update the config OUTSIDE the lock to avoid potential deadlocks
				// with config callbacks that might try to acquire locks
				if shouldUpdateConfig {
					d.updateConfigWithAPIKey(instance, apiKeyToUpdate)
				}
			}
		}
	}()
}

// authenticate uses the configured provider to generate an auth proof, then exchanges it for an API key
func (d *delegatedAuthComponent) authenticate(ctx context.Context, instance *authInstance) (*string, error) {
	// Generate the cloud-specific auth proof
	authProof, err := instance.provider.GenerateAuthProof(ctx, d.config, instance.authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth proof: %w", err)
	}

	// Exchange the proof for an API key from Datadog
	key, err := api.GetAPIKey(d.config, authProof)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange auth proof for API key: %w", err)
	}
	return key, nil
}

// updateConfigWithAPIKey updates the config with the new API key
func (d *delegatedAuthComponent) updateConfigWithAPIKey(instance *authInstance, apiKey string) {
	// Update the config value using the Writer interface
	// This will trigger OnUpdate callbacks for any components listening to this config
	d.config.Set(instance.apiKeyConfigKey, apiKey, pkgconfigmodel.SourceAgentRuntime)
	log.Infof("Updated config key '%s' with new delegated API key ending with: %s", instance.apiKeyConfigKey, scrubber.HideKeyExceptLastFiveChars(apiKey))
}

// Status Provider implementation for delegated auth

// Name returns the name for status sorting
func (d *delegatedAuthComponent) Name() string {
	return "Delegated Auth"
}

// Section returns the section name for status grouping
func (d *delegatedAuthComponent) Section() string {
	return "delegatedauth"
}

// JSON populates the status stats map
func (d *delegatedAuthComponent) JSON(_ bool, stats map[string]interface{}) error {
	d.populateStatusInfo(stats)
	return nil
}

// Text renders the text status output
func (d *delegatedAuthComponent) Text(_ bool, buffer io.Writer) error {
	stats := make(map[string]interface{})
	d.populateStatusInfo(stats)
	return status.RenderText(templatesFS, "delegatedauth.tmpl", buffer, stats)
}

// HTML renders the HTML status output
func (d *delegatedAuthComponent) HTML(_ bool, buffer io.Writer) error {
	stats := make(map[string]interface{})
	d.populateStatusInfo(stats)
	return status.RenderHTML(templatesFS, "delegatedauthHTML.tmpl", buffer, stats)
}

// populateStatusInfo gathers the current status information for delegated auth
func (d *delegatedAuthComponent) populateStatusInfo(stats map[string]interface{}) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check if delegated auth is enabled (has any configured instances)
	stats["enabled"] = len(d.instances) > 0

	if len(d.instances) == 0 {
		return
	}

	// Add resolved provider information
	if d.initialized {
		stats["provider"] = d.resolvedProvider
		// Add provider-specific details
		if awsConfig, ok := d.providerConfig.(*cloudauthconfig.AWSProviderConfig); ok && awsConfig.Region != "" {
			stats["awsRegion"] = awsConfig.Region
		}
	}

	// Add information about each configured instance
	instances := make(map[string]map[string]interface{})
	for key, instance := range d.instances {
		instanceInfo := make(map[string]interface{})

		// Status
		if instance.apiKey != nil {
			instanceInfo["Status"] = "Active"
		} else {
			instanceInfo["Status"] = "Pending"
		}

		// Refresh interval
		instanceInfo["RefreshInterval"] = instance.refreshInterval.String()

		// Add error info if there are consecutive failures
		if instance.consecutiveFailures > 0 {
			instanceInfo["Error"] = fmt.Sprintf("%d consecutive failures", instance.consecutiveFailures)
		}

		instances[key] = instanceInfo
	}
	stats["instances"] = instances
}
