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
	"maps"
	"reflect"
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

	// maxAdditionalEndpointsWriteAttempts bounds the read-write-verify retry loop against
	// concurrent secrets-resolver writes to the same additional_endpoints config value.
	maxAdditionalEndpointsWriteAttempts = 3
)

// detectAWSCredentialSource is a seam for tests. The real detection probes IMDS as its last step,
// which succeeds on an AWS CI runner, so a test asserting the "no credential source" path cannot
// force a failure through configuration alone. The URLs it probes live in
// pkg/util/aws/creds/internal, which this package cannot import.
var detectAWSCredentialSource = creds.DetectAWSCredentialSource

// authInstance holds the state for a single delegated auth configuration (one API key target).
type authInstance struct {
	apiKey          *string
	provider        common.Provider
	authConfig      *common.AuthConfig
	refreshInterval time.Duration
	apiKeyConfigKey string // Configuration key where the API key should be written

	// targetSite is the site to exchange the auth proof against. Empty means use the primary site.
	targetSite string

	// additionalEndpointDomain, if set, routes the key into the map-shape config at
	// additionalEndpointsConfigKey under this domain instead of apiKeyConfigKey.
	additionalEndpointDomain string
	// additionalEndpointsConfigKey is the map-shape config path. Only set when additionalEndpointDomain is set.
	additionalEndpointsConfigKey string
	// additionalEndpointKeyIndex is this instance's position in the domain's key list. Only used
	// when additionalEndpointDomain is set. See InstanceParams.AdditionalEndpointKeyIndex.
	additionalEndpointKeyIndex int
	// additionalEndpointsListConfigKey, if set, routes the key into the list-shape config at
	// this path. Mutually exclusive with additionalEndpointDomain.
	additionalEndpointsListConfigKey string
	// listEntryIndex is this instance's position in the list. Only used when additionalEndpointsListConfigKey is set.
	listEntryIndex int
	// lastWrittenValue is the value most recently written to the target, starting with the
	// DELA(...) directive text. Used to find-and-replace this instance's own entry on each refresh.
	lastWrittenValue string
	// originalDirective is the literal DELA(...) text, never changes. Used as a fallback match
	// in case a racing write reverted the entry back to the raw directive.
	originalDirective string

	// Exponential backoff for retry intervals
	backoff *backoff.ExponentialBackOff

	// consecutiveFailures tracks failures for status reporting
	consecutiveFailures int

	// lastRefresh, nextRefresh, and lastError are for status reporting only.
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
	// disabledReason explains why no provider was resolved, for status display.
	disabledReason string

	// additionalEndpointsMu serializes read-modify-write access to additional_endpoints config
	// values across concurrent instances. Separate from mu to avoid deadlocking with OnUpdate callbacks.
	additionalEndpointsMu sync.Mutex
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

	if params.ProviderConfig != nil {
		// If provider config is explicitly specified, use it
		detectedConfig = params.ProviderConfig
		resolvedProvider = params.ProviderConfig.ProviderName()
		log.Infof("Using explicitly configured cloud provider '%s' for delegated auth", resolvedProvider)
	} else {
		// Auto-detect cloud provider (network I/O happens here, outside any lock)
		source, err := detectAWSCredentialSource(ctx)
		if err != nil {
			// No supported cloud provider detected. Warn and record the reason for the status page.
			disabledReason = fmt.Sprintf("no supported cloud provider detected: %v", err)
			log.Warnf("Delegated authentication is configured but no supported cloud provider was "+
				"detected, so it will stay disabled and the Agent will keep using its statically "+
				"configured API key. %v", err)
		} else {
			log.Infof("Auto-detected AWS as cloud provider for delegated auth (credential source: %s)", source)

			// A configured region wins over auto-detection.
			awsRegion := ""
			if params.Config != nil {
				awsRegion = params.Config.GetString("delegated_auth.aws.region")
			}
			if awsRegion != "" {
				log.Infof("Using configured AWS region for delegated auth: %s", awsRegion)
			} else if region, err := creds.GetAWSRegion(ctx); err != nil {
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
	if params.AdditionalEndpointDomain != "" {
		if params.AdditionalEndpointDirective == "" {
			return errors.New("additional_endpoint_directive is required when additional_endpoint_domain is set")
		}
		if params.AdditionalEndpointsConfigKey == "" {
			return errors.New("additional_endpoints_config_key is required when additional_endpoint_domain is set")
		}
	}
	if params.AdditionalEndpointsListConfigKey != "" && params.AdditionalEndpointDirective == "" {
		return errors.New("additional_endpoint_directive is required when additional_endpoints_list_config_key is set")
	}
	if params.AdditionalEndpointDomain != "" && params.AdditionalEndpointsListConfigKey != "" {
		return errors.New("additional_endpoint_domain and additional_endpoints_list_config_key are mutually exclusive")
	}

	// Check for context cancellation early
	if err := ctx.Err(); err != nil {
		return err
	}

	// Initialize on first call - this detects cloud provider without holding locks.
	// An explicit provider configuration belongs to this instance, so it can override
	// the process-wide detected/default configuration.
	providerConfig, err := d.initializeIfNeeded(ctx, params)
	if err != nil {
		return err
	}
	providerConfig = providerConfigForInstance(providerConfig, params.ProviderConfig)

	// No provider: skip this instance and keep the statically configured key. Write the fallback
	// now if set, so dual-shipping still works. No retry — detection only runs once.
	if providerConfig == nil {
		log.Warnf("Delegated auth is not available on this host, so '%s' will keep its statically configured value", params.APIKeyConfigKey)
		if params.FallbackAPIKey != "" {
			d.writeAPIKeyToTarget(fallbackTargetInstance(params), params.FallbackAPIKey, true)
		}
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
		provider:                         tokenProvider,
		authConfig:                       authConfig,
		refreshInterval:                  refreshInterval,
		apiKeyConfigKey:                  apiKeyConfigKey,
		targetSite:                       resolveTargetSite(params),
		additionalEndpointDomain:         params.AdditionalEndpointDomain,
		additionalEndpointsConfigKey:     params.AdditionalEndpointsConfigKey,
		additionalEndpointKeyIndex:       params.AdditionalEndpointKeyIndex,
		additionalEndpointsListConfigKey: params.AdditionalEndpointsListConfigKey,
		listEntryIndex:                   params.ListEntryIndex,
		lastWrittenValue:                 params.AdditionalEndpointDirective,
		originalDirective:                params.AdditionalEndpointDirective,
		backoff:                          newBackoff(refreshInterval),
		refreshCtx:                       refreshCtx,
		refreshCancel:                    refreshCancel,
		done:                             make(chan struct{}),
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
		// Write the fallback now so the target ships with a static key while retries continue.
		if params.FallbackAPIKey != "" {
			d.writeAPIKeyToTarget(instance, params.FallbackAPIKey, true)
		}
		// Record the failure so the status page shows it immediately.
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

// providerConfigForInstance applies a directive-specific provider configuration after shared
// initialization. This lets multiple delegated-auth instances use distinct provider settings.
func providerConfigForInstance(initialized, instance common.ProviderConfig) common.ProviderConfig {
	if instance != nil {
		return instance
	}
	return initialized
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

	// Exchange the proof for an API key. targetSite must be set for dual-shipping instances
	// targeting a different site than the agent's primary.
	key, err := api.GetAPIKey(d.config, authProof, instance.targetSite)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange auth proof for API key: %w", err)
	}
	return key, nil
}

// resolveTargetSite returns TargetSite if set, else AdditionalEndpointDomain, else empty (use primary site).
func resolveTargetSite(params delegatedauth.InstanceParams) string {
	if params.TargetSite != "" {
		return params.TargetSite
	}
	return params.AdditionalEndpointDomain
}

// fallbackTargetInstance builds a minimal authInstance for the no-cloud-provider case in AddInstance.
func fallbackTargetInstance(params delegatedauth.InstanceParams) *authInstance {
	return &authInstance{
		apiKeyConfigKey:                  params.APIKeyConfigKey,
		targetSite:                       resolveTargetSite(params),
		additionalEndpointDomain:         params.AdditionalEndpointDomain,
		additionalEndpointsConfigKey:     params.AdditionalEndpointsConfigKey,
		additionalEndpointKeyIndex:       params.AdditionalEndpointKeyIndex,
		additionalEndpointsListConfigKey: params.AdditionalEndpointsListConfigKey,
		listEntryIndex:                   params.ListEntryIndex,
		lastWrittenValue:                 params.AdditionalEndpointDirective,
		originalDirective:                params.AdditionalEndpointDirective,
	}
}

// updateConfigWithAPIKey updates the config with a newly-fetched, real (non-fallback) API key.
func (d *delegatedAuthComponent) updateConfigWithAPIKey(instance *authInstance, apiKey string) {
	d.writeAPIKeyToTarget(instance, apiKey, false)
}

// writeAPIKeyToTarget writes apiKey to the configured target (list-shape, map-shape, or flat key).
// isFallback only affects the log message.
func (d *delegatedAuthComponent) writeAPIKeyToTarget(instance *authInstance, apiKey string, isFallback bool) {
	switch {
	case instance.additionalEndpointsListConfigKey != "":
		d.mergeIntoAdditionalEndpointsList(instance, apiKey, isFallback)
	case instance.additionalEndpointDomain != "":
		d.mergeIntoAdditionalEndpoints(instance, apiKey, isFallback)
	default:
		// Update the config value using the Writer interface
		// This will trigger OnUpdate callbacks for any components listening to this config
		d.config.Set(instance.apiKeyConfigKey, apiKey, pkgconfigmodel.SourceAgentRuntime)
		if isFallback {
			log.Infof("Using fallback API key for '%s' (delegated auth unavailable), ending with: %s", instance.apiKeyConfigKey, scrubber.HideKeyExceptLastChars(apiKey))
		} else {
			log.Infof("Updated config key '%s' with new delegated API key ending with: %s", instance.apiKeyConfigKey, scrubber.HideKeyExceptLastChars(apiKey))
		}
	}
}

// mergeIntoAdditionalEndpoints writes apiKey into the map-shape config at
// additionalEndpointsConfigKey under additionalEndpointDomain, replacing the previous value.
// Serialized via additionalEndpointsMu. Writes at SourceSecret (not SourceAgentRuntime) to avoid
// permanently shadowing secret rotations; the retry loop mitigates concurrent writes.
func (d *delegatedAuthComponent) mergeIntoAdditionalEndpoints(instance *authInstance, apiKey string, isFallback bool) {
	d.additionalEndpointsMu.Lock()
	defer d.additionalEndpointsMu.Unlock()

	configKey := instance.additionalEndpointsConfigKey
	domain := instance.additionalEndpointDomain

	written := false
	for attempt := 1; attempt <= maxAdditionalEndpointsWriteAttempts; attempt++ {
		endpoints := d.config.GetStringMapStringSlice(configKey)
		merged := make(map[string][]string, len(endpoints))
		for k, v := range endpoints {
			merged[k] = append([]string{}, v...)
		}

		keys := merged[domain]

		// Prefer the recorded index; fall back to a value-only scan if the list was reordered or
		// the index wasn't provided. Matching by value alone is ambiguous when another entry
		// under the same domain (e.g. a static key equal to this instance's fallback value)
		// happens to share the same string.
		matchIndex := -1
		if instance.additionalEndpointKeyIndex >= 0 && instance.additionalEndpointKeyIndex < len(keys) {
			if v := keys[instance.additionalEndpointKeyIndex]; v == instance.lastWrittenValue || v == instance.originalDirective {
				matchIndex = instance.additionalEndpointKeyIndex
			}
		}
		if matchIndex == -1 {
			for i, key := range keys {
				// Also match originalDirective in case a racing write reverted the entry.
				if key == instance.lastWrittenValue || key == instance.originalDirective {
					matchIndex = i
					break
				}
			}
		}

		replaced := matchIndex != -1
		if replaced {
			keys[matchIndex] = apiKey
		}
		lastAttempt := attempt == maxAdditionalEndpointsWriteAttempts
		if !replaced {
			if !lastAttempt {
				// Expected value missing — concurrent writer may be mid-update. Retry.
				continue
			}
			// Unlike the list-shape path, appending here would orphan whatever key this
			// instance was tracking (it may be a live, unrelated key) rather than just
			// dropping this instance's own update.
			log.Warnf("Could not find previous delegated auth value for additional endpoint '%s' at '%s'; leaving domain's keys unchanged", domain, configKey)
			return
		}
		merged[domain] = keys

		// Re-check the whole value before writing to avoid discarding concurrent changes to other domains.
		if beforeWrite := d.config.GetStringMapStringSlice(configKey); !reflect.DeepEqual(beforeWrite, endpoints) {
			if !lastAttempt {
				continue
			}
			log.Warnf("Possible concurrent update to '%s' detected while writing delegated auth key for additional endpoint '%s'; writing anyway after %d attempts", configKey, domain, maxAdditionalEndpointsWriteAttempts)
		}

		d.config.Set(configKey, merged, pkgconfigmodel.SourceSecret)

		// Verify the write stuck.
		if current := d.config.GetStringMapStringSlice(configKey); reflect.DeepEqual(current, merged) {
			written = true
			break
		}
		if lastAttempt {
			log.Warnf("Possible concurrent update to '%s' while writing delegated auth key for additional endpoint '%s'; giving up after %d attempts, a later refresh will retry", configKey, domain, maxAdditionalEndpointsWriteAttempts)
		}
	}

	// Only advance lastWrittenValue once the write is confirmed.
	if written {
		instance.lastWrittenValue = apiKey
	}
	if isFallback {
		log.Infof("Using fallback API key for additional endpoint '%s' at '%s' (delegated auth unavailable), ending with: %s", domain, configKey, scrubber.HideKeyExceptLastChars(apiKey))
	} else {
		log.Infof("Updated additional endpoint '%s' with new delegated API key ending with: %s", domain, scrubber.HideKeyExceptLastChars(apiKey))
	}
}

// mergeIntoAdditionalEndpointsList writes apiKey into the list-shape config at
// additionalEndpointsListConfigKey, replacing the entry matching lastWrittenValue.
// Locking, write source, and retry behavior mirror mergeIntoAdditionalEndpoints.
func (d *delegatedAuthComponent) mergeIntoAdditionalEndpointsList(instance *authInstance, apiKey string, isFallback bool) {
	d.additionalEndpointsMu.Lock()
	defer d.additionalEndpointsMu.Unlock()

	configKey := instance.additionalEndpointsListConfigKey

	for attempt := 1; attempt <= maxAdditionalEndpointsWriteAttempts; attempt++ {
		entries, ok := common.NormalizeListShapeEntries(d.config.Get(configKey))
		if !ok {
			log.Warnf("Could not read list-shape additional endpoints at '%s' (unexpected type); skipping delegated auth update", configKey)
			return
		}

		merged := make([]any, len(entries))
		copy(merged, entries)

		// Prefer the recorded index; fall back to a value-only scan if the list was reordered.
		// Non-map entries (preserved as-is by NormalizeListShapeEntries) can never match and are
		// skipped rather than causing a type-assertion panic.
		matchIndex := -1
		apiKeyField := ""
		if instance.listEntryIndex >= 0 && instance.listEntryIndex < len(entries) {
			if entryMap, ok := entries[instance.listEntryIndex].(map[string]any); ok {
				if field, valStr, ok := common.CaseInsensitiveStringFieldWithKey(entryMap, "api_key"); ok && (valStr == instance.lastWrittenValue || valStr == instance.originalDirective) {
					matchIndex = instance.listEntryIndex
					apiKeyField = field
				}
			}
		}
		if matchIndex == -1 {
			for i, entry := range entries {
				entryMap, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				// Also match originalDirective in case a racing write reverted the entry.
				if field, valStr, ok := common.CaseInsensitiveStringFieldWithKey(entryMap, "api_key"); ok && (valStr == instance.lastWrittenValue || valStr == instance.originalDirective) {
					matchIndex = i
					apiKeyField = field
					break
				}
			}
		}

		replaced := matchIndex != -1
		if replaced {
			matchedEntry := entries[matchIndex].(map[string]any)
			newEntry := make(map[string]any, len(matchedEntry))
			maps.Copy(newEntry, matchedEntry)
			newEntry[apiKeyField] = apiKey
			merged[matchIndex] = newEntry
		}

		lastAttempt := attempt == maxAdditionalEndpointsWriteAttempts
		if !replaced {
			if !lastAttempt {
				// Expected value missing — concurrent writer may be mid-update. Retry.
				continue
			}
			log.Warnf("Could not find previous delegated auth value in list-shape additional endpoints at '%s'; leaving list unchanged", configKey)
			return
		}

		// Re-check the list before writing — see mergeIntoAdditionalEndpoints.
		entriesNormalized, _ := common.NormalizeListShapeEntries(entries)
		if beforeWrite, ok := common.NormalizeListShapeEntries(d.config.Get(configKey)); ok && !reflect.DeepEqual(beforeWrite, entriesNormalized) {
			if !lastAttempt {
				continue
			}
			log.Warnf("Possible concurrent update to '%s' detected while writing delegated auth key for additional endpoint entry; writing anyway after %d attempts", configKey, maxAdditionalEndpointsWriteAttempts)
		}

		d.config.Set(configKey, merged, pkgconfigmodel.SourceSecret)

		// Verify the write stuck; normalize both sides since merged's element representation
		// isn't necessarily identical to what a fresh read of the same data produces.
		mergedNormalized, _ := common.NormalizeListShapeEntries(merged)
		if current, ok := common.NormalizeListShapeEntries(d.config.Get(configKey)); ok && reflect.DeepEqual(current, mergedNormalized) {
			instance.lastWrittenValue = apiKey
			break
		}
		if lastAttempt {
			log.Warnf("Possible concurrent update to '%s' while writing delegated auth key for additional endpoint entry; giving up after %d attempts, a later refresh will retry", configKey, maxAdditionalEndpointsWriteAttempts)
		}
	}

	if isFallback {
		log.Infof("Using fallback API key for additional endpoint entry at '%s' (delegated auth unavailable), ending with: %s", configKey, scrubber.HideKeyExceptLastChars(apiKey))
	} else {
		log.Infof("Updated additional endpoint entry at '%s' with new delegated API key ending with: %s", configKey, scrubber.HideKeyExceptLastChars(apiKey))
	}
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
		// Distinguish "configured but could not start" from "never configured".
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

		// Refresh timestamps.
		if !instance.lastRefresh.IsZero() {
			instanceInfo["LastRefresh"] = instance.lastRefresh.Format(time.RFC3339)
		}
		if !instance.nextRefresh.IsZero() {
			instanceInfo["NextRefresh"] = instance.nextRefresh.Format(time.RFC3339)
		}

		// Credential source, reported for failed attempts too.
		if reporter, ok := instance.provider.(common.CredentialSourceReporter); ok {
			if source := reporter.LastCredentialSource(); source != "" {
				instanceInfo["CredentialSource"] = source
			}
		}

		// Additional endpoint domain, if this instance manages a dual-shipping key
		if instance.additionalEndpointDomain != "" {
			instanceInfo["AdditionalEndpointDomain"] = instance.additionalEndpointDomain
		}

		// Error info for consecutive failures.
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
