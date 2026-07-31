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

	"github.com/cenkalti/backoff/v7"

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

	// lastRefresh is when this key was last fetched successfully, and nextRefresh when the next
	// attempt is scheduled. lastError is the most recent failure. All three are for status
	// reporting only: without lastError the status page can only report a failure count, which
	// tells an operator that something is broken but not what.
	lastRefresh time.Time
	nextRefresh time.Time
	lastError   error

	// Context and cancellation for background refresh goroutine
	refreshCtx    context.Context
	refreshCancel context.CancelFunc

	// done is closed when the background refresh goroutine exits
	done chan struct{}
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
	// disabledReason explains why no provider was resolved, for status display. Empty when a
	// provider was resolved. Without it the status page reports only "not enabled", which is
	// indistinguishable from "never configured".
	disabledReason string
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
	var disabledReason string

	// Some flavors (IoT, Heroku) are built without the AWS credential providers because they cannot
	// encounter AWS workload identity. Check that before detecting rather than after: detection is
	// compiled unconditionally and would happily report EC2 IMDS, enabling the component so that
	// every subsequent refresh fails with an error the operator has no way to act on. Recording it
	// as a disable reason instead keeps the status page honest and the logs quiet.
	if !aws.Supported {
		disabledReason = "this Agent flavor is not built with AWS Cloud Auth support"
		log.Warnf("Delegated authentication is configured but this Agent flavor is not built with " +
			"AWS Cloud Auth support, so it will stay disabled and the Agent will keep using its " +
			"statically configured API key.")
	} else if params.ProviderConfig != nil {
		// If provider config is explicitly specified, use it
		detectedConfig = params.ProviderConfig
		resolvedProvider = params.ProviderConfig.ProviderName()
		log.Infof("Using explicitly configured cloud provider '%s' for delegated auth", resolvedProvider)
	} else {
		// Auto-detect cloud provider (network I/O happens here, outside any lock)
		source, err := creds.DetectAWSCredentialSource(ctx)
		if err != nil {
			// No supported cloud provider detected, so delegated auth stays disabled. This is only
			// reached when the operator asked for delegated auth (AddInstance is only called for a
			// prefix with org_uuid set), so it is a misconfiguration rather than a normal state:
			// warn, and record the reason for the status page.
			disabledReason = fmt.Sprintf("no supported cloud provider detected: %v", err)
			log.Warnf("Delegated authentication is configured but no supported cloud provider was "+
				"detected, so it will stay disabled and the Agent will keep using its statically "+
				"configured API key. %v", err)
		} else {
			log.Infof("Auto-detected AWS as cloud provider for delegated auth (credential source: %s)", source)

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
	d.disabledReason = disabledReason
	d.initialized = true

	return d.providerConfig, nil
}

// AddInstance configures delegated auth for a specific API key.
// On the first call, it detects the cloud provider and initializes the component.
// The context is used for the initial API key fetch and cloud provider detection.
func (d *delegatedAuthComponent) AddInstance(ctx context.Context, params delegatedauth.InstanceParams) error {
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

	// Check for context cancellation early
	if err := ctx.Err(); err != nil {
		return err
	}

	// Initialize on first call - this detects cloud provider without holding locks
	providerConfig, err := d.initializeIfNeeded(ctx, params)
	if err != nil {
		return err
	}

	// If no provider is configured (unsupported cloud or not running in cloud), skip this instance;
	// the agent will use whatever API key is already configured. initializeIfNeeded already warned
	// with the detection failure, so this only names the affected key.
	if providerConfig == nil {
		log.Warnf("Delegated auth is not available on this host, so '%s' will keep its statically configured value", params.APIKeyConfigKey)
		return nil
	}

	apiKeyConfigKey := params.APIKeyConfigKey

	refreshInterval := time.Duration(params.RefreshInterval) * time.Minute
	if refreshInterval <= 0 {
		// Default to 60 minutes if refresh interval was set to 0 or negative
		// This prevents panics from time.NewTicker with non-positive duration
		refreshInterval = 60 * time.Minute
		log.Warnf("Refresh interval was set to %d for '%s', defaulting to 60 minutes", params.RefreshInterval, apiKeyConfigKey)
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
		done:            make(chan struct{}),
	}

	// Check if we're replacing an existing instance.
	// This is expected behavior - callers may reconfigure delegated auth (e.g., with different org UUID
	// or refresh interval). When this happens, we cancel the old refresh goroutine and wait for it to
	// exit before starting a new one to prevent goroutine leaks.
	var existingDone chan struct{}
	d.mu.Lock()
	if existingInstance, exists := d.instances[apiKeyConfigKey]; exists {
		log.Infof("Replacing existing delegated auth configuration for '%s'", apiKeyConfigKey)
		// Cancel the existing refresh goroutine
		if existingInstance.refreshCancel != nil {
			existingInstance.refreshCancel()
		}
		existingDone = existingInstance.done
	}
	d.instances[apiKeyConfigKey] = instance
	d.mu.Unlock()

	// Wait for the old goroutine to exit outside the lock to avoid blocking other operations
	if existingDone != nil {
		select {
		case <-existingDone:
			// Old goroutine has exited
		case <-ctx.Done():
			// Context was canceled while waiting - clean up and return error
			refreshCancel()
			return ctx.Err()
		}
	}

	log.Infof("Delegated authentication is enabled for '%s', fetching initial API key...", apiKeyConfigKey)

	// Fetch the initial API key synchronously using the caller's context
	apiKey, _, err := d.refreshAndGetAPIKey(ctx, instance, false)
	if err != nil {
		log.Errorf("Failed to get initial delegated API key for '%s': %v", apiKeyConfigKey, err)
		// Record the failure so the status page shows it. Without this the instance renders as
		// "Pending" with no error until the first background refresh, which is a full refresh
		// interval away, so a startup failure would look identical to a fetch still in flight.
		d.mu.Lock()
		instance.consecutiveFailures++
		instance.lastError = err
		d.mu.Unlock()
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
	instance.lastRefresh = time.Now()
	instance.lastError = nil
	d.mu.Unlock()

	return apiKey, true, nil
}

// startBackgroundRefresh starts the background goroutine that periodically refreshes the API key
// with exponential backoff on failures
func (d *delegatedAuthComponent) startBackgroundRefresh(instance *authInstance) {
	go func() {
		// Signal goroutine exit when we return
		defer close(instance.done)

		// Get initial interval with jitter from backoff
		d.mu.Lock()
		nextInterval := instance.backoff.NextBackOff()
		instance.nextRefresh = time.Now().Add(nextInterval)
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
					instance.lastError = lErr

					// Get next backoff interval (exponentially increasing with jitter)
					nextInterval := instance.backoff.NextBackOff()
					instance.nextRefresh = time.Now().Add(nextInterval)
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
					instance.nextRefresh = time.Now().Add(nextInterval)

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
	log.Infof("Updated config key '%s' with new delegated API key ending with: %s", instance.apiKeyConfigKey, scrubber.HideKeyExceptLastChars(apiKey))
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
		// Distinguish "configured but could not start" from "never configured". Detection only runs
		// when an org_uuid was set, so a reason here means the operator asked for delegated auth and
		// it could not be brought up.
		if d.disabledReason != "" {
			stats["disabledReason"] = d.disabledReason
		}
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

		// Refresh timestamps. The status templates have always rendered these when present; before
		// they were never populated, so the section could not answer "is this key still refreshing".
		if !instance.lastRefresh.IsZero() {
			instanceInfo["LastRefresh"] = instance.lastRefresh.Format(time.RFC3339)
		}
		if !instance.nextRefresh.IsZero() {
			instanceInfo["NextRefresh"] = instance.nextRefresh.Format(time.RFC3339)
		}

		// Which credential mechanism was selected for this key. Reported for failed attempts too,
		// since "which source did it even try" is the first thing to establish when delegated auth
		// works on one workload and not another.
		if reporter, ok := instance.provider.(common.CredentialSourceReporter); ok {
			if source := reporter.LastCredentialSource(); source != "" {
				instanceInfo["CredentialSource"] = source
			}
		}

		// Add error info if there are consecutive failures. The message carries the last error, which
		// names the credential mechanism that was tried and why it failed.
		if instance.consecutiveFailures > 0 {
			if instance.lastError != nil {
				instanceInfo["Error"] = fmt.Sprintf("%d consecutive failures, last error: %v", instance.consecutiveFailures, instance.lastError)
			} else {
				instanceInfo["Error"] = fmt.Sprintf("%d consecutive failures", instance.consecutiveFailures)
			}
		}

		instances[key] = instanceInfo
	}
	stats["instances"] = instances
}
