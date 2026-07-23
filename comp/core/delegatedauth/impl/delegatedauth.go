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
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v6"

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

	// additionalEndpointDomain, if set, means the API key should be merged into the map-shape
	// config map at additionalEndpointsConfigKey under this domain instead of being written to
	// apiKeyConfigKey as a flat value.
	additionalEndpointDomain string
	// additionalEndpointsConfigKey is the map-shape config path (e.g. "additional_endpoints",
	// "apm_config.additional_endpoints") that additionalEndpointDomain refers into. Only set when
	// additionalEndpointDomain is set.
	additionalEndpointsConfigKey string
	// additionalEndpointsListConfigKey, if set, means the API key should be merged into the
	// list-shape config value at this path (e.g. "logs_config.additional_endpoints") instead of
	// being written to apiKeyConfigKey as a flat value. Mutually exclusive with
	// additionalEndpointDomain.
	additionalEndpointsListConfigKey string
	// lastWrittenValue is the value this instance most recently wrote into its target (the
	// domain's key list in a map-shape additional_endpoints value, or the matching entry's
	// api_key in a list-shape one), starting with the literal DELA(...) directive text that
	// requested this instance. Used to find-and-replace only this instance's own entry on each
	// refresh, without disturbing any other entry (static or otherwise) for that target.
	lastWrittenValue string

	// Exponential backoff for retry intervals
	backoff *backoff.ExponentialBackOff

	// consecutiveFailures tracks failures for status reporting
	consecutiveFailures int

	// Context and cancellation for background refresh goroutine
	refreshCtx    context.Context
	refreshCancel context.CancelFunc

	// done is closed when the background refresh goroutine exits
	done chan struct{}

	// triggerRefresh is a non-blocking, buffered(1) channel used by Refresh() to wake up
	// startBackgroundRefresh's select loop for an early fetch attempt instead of waiting for the
	// next scheduled tick. Mirrors comp/core/secrets's own refreshTrigger channel pattern - never
	// call refreshAndGetAPIKey directly from Refresh() itself, that would block the caller (e.g.
	// the forwarder's transaction worker) on a real network round-trip.
	triggerRefresh chan struct{}
	// lastTriggeredRefresh is when Refresh() last actually sent on triggerRefresh for this
	// instance, protected by delegatedAuthComponent.mu like the instance's other mutable fields.
	// Used to throttle repeated triggers so a burst of 403s can't cause repeated real fetch
	// attempts against the auth-proof exchange endpoint.
	lastTriggeredRefresh time.Time
}

// minTriggerRefreshInterval is the minimum time between Refresh()-triggered early fetch attempts
// for a single instance.
const minTriggerRefreshInterval = 30 * time.Second

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

	// additionalEndpointsMu serializes read-modify-write access to the `additional_endpoints`
	// config value across concurrent instances (e.g. two DELA(...) entries refreshing at once).
	// Deliberately separate from mu: config writes happen outside mu to avoid deadlocking with
	// OnUpdate callbacks (see startBackgroundRefresh), but concurrent additional_endpoints merges
	// still need to be serialized against each other.
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

	// Initialize on first call - this detects cloud provider without holding locks
	providerConfig, err := d.initializeIfNeeded(ctx, params)
	if err != nil {
		return err
	}

	// If no provider is configured (unsupported cloud or not running in cloud),
	// skip - the agent will use whatever API key is already configured. For an
	// additional-endpoints target with a fallback, write that fallback now so dual-shipping still
	// works with a static key instead of silently shipping nothing; there's no retry here since
	// cloud-provider detection only runs once, at the first AddInstance call.
	if providerConfig == nil {
		log.Debugf("Delegated auth not available (no supported cloud provider), skipping configuration for '%s'", params.APIKeyConfigKey)
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
		additionalEndpointDomain:         params.AdditionalEndpointDomain,
		additionalEndpointsConfigKey:     params.AdditionalEndpointsConfigKey,
		additionalEndpointsListConfigKey: params.AdditionalEndpointsListConfigKey,
		lastWrittenValue:                 params.AdditionalEndpointDirective,
		backoff:                          newBackoff(refreshInterval),
		refreshCtx:                       refreshCtx,
		refreshCancel:                    refreshCancel,
		done:                             make(chan struct{}),
		triggerRefresh:                   make(chan struct{}, 1),
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
		// Backoff will be used for retry interval in startBackgroundRefresh. Write the fallback
		// now, if any, so the target ships with a static key while retries continue in the
		// background; a later successful fetch replaces it (see updateConfigWithAPIKey).
		if params.FallbackAPIKey != "" {
			d.writeAPIKeyToTarget(instance, params.FallbackAPIKey, true)
		}
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
		// Signal goroutine exit when we return
		defer close(instance.done)

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
			case <-instance.triggerRefresh:
				// A Refresh() nudge (e.g. the forwarder saw a 403) - do the same forced attempt as
				// a normal tick, just earlier. Runs in this same select loop as the ticker case
				// below, so the two can never race each other for this instance.
				if d.performRefreshAttempt(instance, ticker) {
					return
				}
			case <-ticker.C:
				if d.performRefreshAttempt(instance, ticker) {
					return
				}
			}
		}
	}()
}

// performRefreshAttempt does one forced refresh attempt and updates backoff/config exactly as
// startBackgroundRefresh's ticker case always has. Returns true if the goroutine should exit
// (context canceled).
func (d *delegatedAuthComponent) performRefreshAttempt(instance *authInstance, ticker *time.Ticker) bool {
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
			return true
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
	return false
}

// Refresh nudges any not-yet-resolved instances to retry sooner than their normal backoff,
// throttled per-instance (minTriggerRefreshInterval) so a burst of calls can't cause repeated real
// fetch attempts. Never performs the fetch itself - only wakes up the existing background refresh
// goroutine (see startBackgroundRefresh's triggerRefresh case) - so this never blocks the caller
// on network I/O. Mirrors comp/core/secrets.Component.Refresh()'s contract.
func (d *delegatedAuthComponent) Refresh() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	nudged := false
	now := time.Now()
	for _, instance := range d.instances {
		if instance.apiKey != nil {
			// Already resolved - don't force a healthy instance to re-authenticate just because
			// some unrelated domain's transaction got a 403.
			continue
		}
		nudged = true
		if now.Sub(instance.lastTriggeredRefresh) < minTriggerRefreshInterval {
			continue
		}
		select {
		case instance.triggerRefresh <- struct{}{}:
			instance.lastTriggeredRefresh = now
		default:
			// A trigger is already pending for this instance; nothing more to do.
		}
	}
	return nudged
}

// authenticate uses the configured provider to generate an auth proof, then exchanges it for an API key
func (d *delegatedAuthComponent) authenticate(ctx context.Context, instance *authInstance) (*string, error) {
	// Generate the cloud-specific auth proof
	authProof, err := instance.provider.GenerateAuthProof(ctx, d.config, instance.authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth proof: %w", err)
	}

	// Exchange the proof for an API key from Datadog. For a dual-shipping additional_endpoints
	// instance, additionalEndpointDomain is the actual site to exchange against - it is very
	// often a different site than the agent's primary dd_url/site (that's the whole point of
	// dual-shipping), so it must not be left to fall back to the primary site.
	key, err := api.GetAPIKey(d.config, authProof, instance.additionalEndpointDomain)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange auth proof for API key: %w", err)
	}
	return key, nil
}

// fallbackTargetInstance builds a minimal authInstance carrying only the fields
// writeAPIKeyToTarget needs, for use when no real authInstance exists yet (the no-cloud-provider
// case in AddInstance, which returns before creating one and never starts a refresh loop).
func fallbackTargetInstance(params delegatedauth.InstanceParams) *authInstance {
	return &authInstance{
		apiKeyConfigKey:                  params.APIKeyConfigKey,
		additionalEndpointDomain:         params.AdditionalEndpointDomain,
		additionalEndpointsConfigKey:     params.AdditionalEndpointsConfigKey,
		additionalEndpointsListConfigKey: params.AdditionalEndpointsListConfigKey,
		lastWrittenValue:                 params.AdditionalEndpointDirective,
	}
}

// updateConfigWithAPIKey updates the config with a newly-fetched, real (non-fallback) API key.
// Only called on a successful fetch - either the initial one in AddInstance, or a later one from
// startBackgroundRefresh. The fallback API key (see writeAPIKeyToTarget's isFallback param) is
// only ever written from AddInstance's two failure branches, both of which run before this is ever
// reached for a given instance - so a real key, once obtained, is never reverted back to a
// fallback by a later transient refresh failure.
func (d *delegatedAuthComponent) updateConfigWithAPIKey(instance *authInstance, apiKey string) {
	d.writeAPIKeyToTarget(instance, apiKey, false)
}

// writeAPIKeyToTarget writes apiKey to wherever this instance is configured to write: a list-shape
// additional_endpoints-style config value, a map-shape one, or a flat config key. isFallback only
// affects the log message - it does not change which target is written to.
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

// mergeIntoAdditionalEndpoints writes apiKey into the map-shape config value at
// instance.additionalEndpointsConfigKey under instance.additionalEndpointDomain, replacing the
// value this instance previously wrote there (starting with the original DELA(...) directive text)
// without disturbing any other entry for that domain, static or otherwise. Serialized via
// additionalEndpointsMu since multiple instances (one per DELA(...) entry, possibly across
// different config keys) can refresh concurrently.
func (d *delegatedAuthComponent) mergeIntoAdditionalEndpoints(instance *authInstance, apiKey string, isFallback bool) {
	d.additionalEndpointsMu.Lock()
	defer d.additionalEndpointsMu.Unlock()

	configKey := instance.additionalEndpointsConfigKey
	domain := instance.additionalEndpointDomain
	endpoints := d.config.GetStringMapStringSlice(configKey)
	merged := make(map[string][]string, len(endpoints))
	for k, v := range endpoints {
		merged[k] = append([]string{}, v...)
	}

	keys := merged[domain]
	replaced := false
	for i, key := range keys {
		if key == instance.lastWrittenValue {
			keys[i] = apiKey
			replaced = true
			break
		}
	}
	if !replaced {
		log.Warnf("Could not find previous delegated auth value for additional endpoint '%s' at '%s'; appending new key instead", domain, configKey)
		keys = append(keys, apiKey)
	}
	merged[domain] = keys

	d.config.Set(configKey, merged, pkgconfigmodel.SourceAgentRuntime)
	instance.lastWrittenValue = apiKey
	if isFallback {
		log.Infof("Using fallback API key for additional endpoint '%s' at '%s' (delegated auth unavailable), ending with: %s", domain, configKey, scrubber.HideKeyExceptLastChars(apiKey))
	} else {
		log.Infof("Updated additional endpoint '%s' with new delegated API key ending with: %s", domain, scrubber.HideKeyExceptLastChars(apiKey))
	}
}

// normalizeListShapeEntries converts a list-shape `additional_endpoints`-style config value into a
// slice of string-keyed maps, regardless of which underlying shape config.Get() returns:
//   - []any of map[any]any entries - what a real YAML-sourced value decodes to (the config
//     loader's YAML decoding produces yaml.v2-style nested maps for nested mappings, not
//     map[string]any)
//   - []map[string]any - what an unset key's registered empty/typed default looks like
//
// Duplicated from the identical helper in pkg/config/setup/config.go rather than shared: this
// package can't depend on pkg/config/setup or pkg/config/utils without risking a cycle back
// through comp/core/delegatedauth/def, which pkg/config/setup already imports.
func normalizeListShapeEntries(raw any) ([]map[string]any, bool) {
	switch typed := raw.(type) {
	case []any:
		entries := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			switch m := item.(type) {
			case map[string]any:
				entries = append(entries, m)
			case map[any]any:
				converted := make(map[string]any, len(m))
				for k, v := range m {
					converted[fmt.Sprintf("%v", k)] = v
				}
				entries = append(entries, converted)
			}
		}
		return entries, true
	case []map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

// caseInsensitiveStringField looks up a string-valued field in entry, matching the field name
// case-insensitively (list-shape additional_endpoints entries mix casing across products, e.g.
// "api_key" but "Host").
func caseInsensitiveStringField(entry map[string]any, field string) (string, bool) {
	for k, v := range entry {
		if !strings.EqualFold(k, field) {
			continue
		}
		s, ok := v.(string)
		return s, ok
	}
	return "", false
}

// mergeIntoAdditionalEndpointsList writes apiKey into the list-shape config value at
// instance.additionalEndpointsListConfigKey (a list of {api_key, Host, Port, ...} entries),
// replacing the entry whose api_key still holds this instance's lastWrittenValue - matching by
// value rather than list index/position, so a reordered or resized list doesn't silently drop the
// resolved key. Serialized via additionalEndpointsMu for the same reason as
// mergeIntoAdditionalEndpoints.
func (d *delegatedAuthComponent) mergeIntoAdditionalEndpointsList(instance *authInstance, apiKey string, isFallback bool) {
	d.additionalEndpointsMu.Lock()
	defer d.additionalEndpointsMu.Unlock()

	configKey := instance.additionalEndpointsListConfigKey
	entries, ok := normalizeListShapeEntries(d.config.Get(configKey))
	if !ok {
		log.Warnf("Could not read list-shape additional endpoints at '%s' (unexpected type); skipping delegated auth update", configKey)
		return
	}

	merged := make([]any, len(entries))
	replaced := false
	for i, entry := range entries {
		if !replaced {
			if valStr, ok := caseInsensitiveStringField(entry, "api_key"); ok && valStr == instance.lastWrittenValue {
				newEntry := make(map[string]any, len(entry))
				maps.Copy(newEntry, entry)
				newEntry["api_key"] = apiKey
				merged[i] = newEntry
				replaced = true
				continue
			}
		}
		merged[i] = entry
	}

	if !replaced {
		log.Warnf("Could not find previous delegated auth value in list-shape additional endpoints at '%s'; leaving list unchanged", configKey)
		return
	}

	d.config.Set(configKey, merged, pkgconfigmodel.SourceAgentRuntime)
	instance.lastWrittenValue = apiKey
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

		// Additional endpoint domain, if this instance manages a dual-shipping key
		if instance.additionalEndpointDomain != "" {
			instanceInfo["AdditionalEndpointDomain"] = instance.additionalEndpointDomain
		}

		// Add error info if there are consecutive failures
		if instance.consecutiveFailures > 0 {
			instanceInfo["Error"] = fmt.Sprintf("%d consecutive failures", instance.consecutiveFailures)
		}

		instances[key] = instanceInfo
	}
	stats["instances"] = instances
}
