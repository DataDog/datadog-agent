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
	"strings"
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

	// maxAdditionalEndpointsWriteAttempts bounds the optimistic read-write-verify retry loop in
	// mergeIntoAdditionalEndpoints and mergeIntoAdditionalEndpointsList, which race the secrets
	// resolver's independent, unsynchronized read-modify-write of the same additional_endpoints
	// config value (pkg/config/setup/config.go's configAssignAtPath).
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

	// targetSite is the site to exchange the auth proof against - decoupled from
	// additionalEndpointDomain because a list-shape entry's target site (its Host field) is
	// independent of the map-shape merge routing that additionalEndpointDomain also controls.
	// Empty means use the agent's primary site.
	targetSite string

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
	// listEntryIndex is this instance's position within additionalEndpointsListConfigKey's list.
	// Only meaningful when additionalEndpointsListConfigKey is set - it's how IsManaged tells
	// apart several DELA(...) entries at the same list-shape config key.
	listEntryIndex int
	// lastWrittenValue is the value this instance most recently wrote into its target (the
	// domain's key list in a map-shape additional_endpoints value, or the matching entry's
	// api_key in a list-shape one), starting with the literal DELA(...) directive text that
	// requested this instance. Used to find-and-replace only this instance's own entry on each
	// refresh, without disturbing any other entry (static or otherwise) for that target.
	lastWrittenValue string
	// originalDirective is the literal DELA(...) directive text this instance was created for and
	// never changes. Used as a fallback match in mergeIntoAdditionalEndpoints/List: if a racing
	// secrets-resolver write reverts this instance's entry back to the raw directive text,
	// matching on lastWrittenValue alone would miss it and append a duplicate instead of healing it.
	originalDirective string

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

	// triggerRefresh wakes up startBackgroundRefresh for an early fetch; buffered(1) and
	// non-blocking so Refresh() never blocks the caller on network I/O.
	triggerRefresh chan struct{}
	// lastTriggeredRefresh throttles repeated Refresh() triggers; protected by
	// delegatedAuthComponent.mu.
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
	// disabledReason explains why no provider was resolved, for status display. Empty when a
	// provider was resolved. Without it the status page reports only "not enabled", which is
	// indistinguishable from "never configured".
	disabledReason string

	// additionalEndpointsMu serializes read-modify-write access to `additional_endpoints` config
	// values across concurrent instances. Kept separate from mu because config writes happen
	// outside mu to avoid deadlocking with OnUpdate callbacks (see startBackgroundRefresh).
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

			// A configured delegated_auth.aws.region wins over detection. Setting it must not imply
			// an explicitly configured provider, or it would suppress the detection above and the
			// disable reason that goes with it.
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

	// Initialize on first call - this detects cloud provider without holding locks
	providerConfig, err := d.initializeIfNeeded(ctx, params)
	if err != nil {
		return err
	}

	// If no provider is configured (unsupported cloud or not running in cloud), skip this instance;
	// the agent will use whatever API key is already configured. initializeIfNeeded already warned
	// with the detection failure, so this only names the affected key. For an additional-endpoints
	// target with a fallback, write that fallback now so dual-shipping still works with a static key
	// instead of silently shipping nothing; there's no retry here since cloud-provider detection only
	// runs once, at the first AddInstance call.
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
		additionalEndpointsListConfigKey: params.AdditionalEndpointsListConfigKey,
		listEntryIndex:                   params.ListEntryIndex,
		lastWrittenValue:                 params.AdditionalEndpointDirective,
		originalDirective:                params.AdditionalEndpointDirective,
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
			case <-instance.triggerRefresh:
				// A Refresh() nudge - same forced attempt as a tick, just earlier.
				if d.performRefreshAttempt(instance, ticker) {
					return
				}
				// Drain a tick that may have landed at the same instant, since Reset() doesn't.
				select {
				case <-ticker.C:
				default:
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
	return false
}

// Refresh nudges every instance to retry sooner than its normal backoff, throttled per-instance to
// avoid repeated real fetch attempts. Only sends on triggerRefresh, never fetches inline. Nudges
// already-resolved instances too, since the common case is a previously-good key expiring rather
// than a cold start.
func (d *delegatedAuthComponent) Refresh() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.instances) == 0 {
		return false
	}

	now := time.Now()
	for _, instance := range d.instances {
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
	return true
}

// authenticate uses the configured provider to generate an auth proof, then exchanges it for an API key
func (d *delegatedAuthComponent) authenticate(ctx context.Context, instance *authInstance) (*string, error) {
	// Generate the cloud-specific auth proof
	authProof, err := instance.provider.GenerateAuthProof(ctx, d.config, instance.authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate auth proof: %w", err)
	}

	// Exchange the proof for an API key from Datadog. For a dual-shipping additional_endpoints
	// instance, targetSite is the actual site to exchange against - it is very often a different
	// site than the agent's primary dd_url/site (that's the whole point of dual-shipping), so it
	// must not be left to fall back to the primary site.
	key, err := api.GetAPIKey(d.config, authProof, instance.targetSite)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange auth proof for API key: %w", err)
	}
	return key, nil
}

// resolveTargetSite returns the site to exchange the auth proof against: TargetSite if set
// (the list-shape case, an entry's Host field), else AdditionalEndpointDomain (the map-shape
// case, where the domain key doubles as the target site), else empty (use the primary site).
func resolveTargetSite(params delegatedauth.InstanceParams) string {
	if params.TargetSite != "" {
		return params.TargetSite
	}
	return params.AdditionalEndpointDomain
}

// fallbackTargetInstance builds a minimal authInstance carrying only the fields
// writeAPIKeyToTarget needs, for use when no real authInstance exists yet (the no-cloud-provider
// case in AddInstance, which returns before creating one and never starts a refresh loop).
func fallbackTargetInstance(params delegatedauth.InstanceParams) *authInstance {
	return &authInstance{
		apiKeyConfigKey:                  params.APIKeyConfigKey,
		targetSite:                       resolveTargetSite(params),
		additionalEndpointDomain:         params.AdditionalEndpointDomain,
		additionalEndpointsConfigKey:     params.AdditionalEndpointsConfigKey,
		additionalEndpointsListConfigKey: params.AdditionalEndpointsListConfigKey,
		listEntryIndex:                   params.ListEntryIndex,
		lastWrittenValue:                 params.AdditionalEndpointDirective,
		originalDirective:                params.AdditionalEndpointDirective,
	}
}

// IsManaged reports whether an active instance currently manages target, matching on whichever
// identity fields target sets (see delegatedauth.Target's doc). Unlike inspecting the current
// api_key config value, this stays true even after the DELA(...) directive has already been
// resolved to a real key.
func (d *delegatedAuthComponent) IsManaged(target delegatedauth.Target) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, instance := range d.instances {
		switch {
		case target.AdditionalEndpointsListConfigKey != "":
			if instance.additionalEndpointsListConfigKey == target.AdditionalEndpointsListConfigKey &&
				instance.listEntryIndex == target.ListEntryIndex {
				return true
			}
		case target.AdditionalEndpointsConfigKey != "":
			if instance.additionalEndpointsConfigKey == target.AdditionalEndpointsConfigKey &&
				instance.additionalEndpointDomain == target.AdditionalEndpointDomain {
				return true
			}
		case target.APIKeyConfigKey != "":
			if instance.apiKeyConfigKey == target.APIKeyConfigKey &&
				instance.additionalEndpointDomain == "" &&
				instance.additionalEndpointsListConfigKey == "" {
				return true
			}
		}
	}
	return false
}

// updateConfigWithAPIKey updates the config with a newly-fetched, real (non-fallback) API key.
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
// value this instance previously wrote there without disturbing any other entry for that domain.
// Serialized via additionalEndpointsMu against other delegated-auth writers.
//
// Writes at SourceSecret, not SourceAgentRuntime: a domain's additional_endpoints list can mix
// DELA(...) and ENC[...] entries, and both this function and the secrets resolver's
// configAssignAtPath read-modify-write the same key. SourceAgentRuntime outranks SourceSecret, so
// writing there would let this component permanently shadow later secret rotations. The
// read-write-verify retry loop below is a best-effort mitigation (not true mutual exclusion,
// since configAssignAtPath has no coordinated lock) to narrow that race - see
// maxAdditionalEndpointsWriteAttempts.
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
		replaced := false
		for i, key := range keys {
			// Also match originalDirective in case a racing write reverted this entry back to
			// the raw directive text - see originalDirective's doc comment.
			if key == instance.lastWrittenValue || key == instance.originalDirective {
				keys[i] = apiKey
				replaced = true
				break
			}
		}
		lastAttempt := attempt == maxAdditionalEndpointsWriteAttempts
		if !replaced {
			if !lastAttempt {
				// Our expected previous value is missing from this read - a concurrent writer may be
				// mid-update. Retry with a fresh read rather than risking a duplicate append.
				continue
			}
			log.Warnf("Could not find previous delegated auth value for additional endpoint '%s' at '%s'; appending new key instead", domain, configKey)
			keys = append(keys, apiKey)
		}
		merged[domain] = keys

		// Re-check the whole value (not just this domain) right before writing, since merged
		// carries every other domain through unchanged and a concurrent change to any of them
		// would otherwise be silently discarded by our write below.
		if beforeWrite := d.config.GetStringMapStringSlice(configKey); !reflect.DeepEqual(beforeWrite, endpoints) {
			if !lastAttempt {
				continue
			}
			log.Warnf("Possible concurrent update to '%s' detected while writing delegated auth key for additional endpoint '%s'; writing anyway after %d attempts", configKey, domain, maxAdditionalEndpointsWriteAttempts)
		}

		d.config.Set(configKey, merged, pkgconfigmodel.SourceSecret)

		// Verify the write stuck; a concurrent writer could still have raced us between the
		// check above and this Set call.
		if current := d.config.GetStringMapStringSlice(configKey); reflect.DeepEqual(current, merged) {
			written = true
			break
		}
		if lastAttempt {
			log.Warnf("Possible concurrent update to '%s' while writing delegated auth key for additional endpoint '%s'; giving up after %d attempts, a later refresh will retry", configKey, domain, maxAdditionalEndpointsWriteAttempts)
		}
	}

	// Only advance lastWrittenValue once the write is confirmed live; otherwise the next refresh
	// would search for a value that was never actually written and append a duplicate instead of
	// healing the entry.
	if written {
		instance.lastWrittenValue = apiKey
	}
	if isFallback {
		log.Infof("Using fallback API key for additional endpoint '%s' at '%s' (delegated auth unavailable), ending with: %s", domain, configKey, scrubber.HideKeyExceptLastChars(apiKey))
	} else {
		log.Infof("Updated additional endpoint '%s' with new delegated API key ending with: %s", domain, scrubber.HideKeyExceptLastChars(apiKey))
	}
}

// normalizeListShapeEntries converts a list-shape `additional_endpoints`-style config value into a
// slice of string-keyed maps, regardless of whether config.Get() returns []any of map[any]any
// (real YAML-sourced values) or []map[string]any (a registered default).
//
// Duplicated from the identical helper in pkg/config/setup/config.go rather than shared, since
// this package can't depend on pkg/config/setup without risking an import cycle.
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
// resolved key. Locking, write source, and retry behavior mirror mergeIntoAdditionalEndpoints.
func (d *delegatedAuthComponent) mergeIntoAdditionalEndpointsList(instance *authInstance, apiKey string, isFallback bool) {
	d.additionalEndpointsMu.Lock()
	defer d.additionalEndpointsMu.Unlock()

	configKey := instance.additionalEndpointsListConfigKey

	for attempt := 1; attempt <= maxAdditionalEndpointsWriteAttempts; attempt++ {
		entries, ok := normalizeListShapeEntries(d.config.Get(configKey))
		if !ok {
			log.Warnf("Could not read list-shape additional endpoints at '%s' (unexpected type); skipping delegated auth update", configKey)
			return
		}

		merged := make([]any, len(entries))
		replaced := false
		for i, entry := range entries {
			if !replaced {
				// Also match originalDirective - see its doc comment on authInstance.
				if valStr, ok := caseInsensitiveStringField(entry, "api_key"); ok && (valStr == instance.lastWrittenValue || valStr == instance.originalDirective) {
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

		lastAttempt := attempt == maxAdditionalEndpointsWriteAttempts
		if !replaced {
			if !lastAttempt {
				// Our expected previous value is missing from this read - a concurrent writer may be
				// mid-update. Retry with a fresh read.
				continue
			}
			log.Warnf("Could not find previous delegated auth value in list-shape additional endpoints at '%s'; leaving list unchanged", configKey)
			return
		}

		// Re-check the list right before writing - see mergeIntoAdditionalEndpoints.
		entriesNormalized, _ := normalizeListShapeEntries(entries)
		if beforeWrite, ok := normalizeListShapeEntries(d.config.Get(configKey)); ok && !reflect.DeepEqual(beforeWrite, entriesNormalized) {
			if !lastAttempt {
				continue
			}
			log.Warnf("Possible concurrent update to '%s' detected while writing delegated auth key for additional endpoint entry; writing anyway after %d attempts", configKey, maxAdditionalEndpointsWriteAttempts)
		}

		d.config.Set(configKey, merged, pkgconfigmodel.SourceSecret)

		// Verify the write stuck; normalize both sides since merged's element representation
		// isn't necessarily identical to what a fresh read of the same data produces.
		mergedNormalized, _ := normalizeListShapeEntries(merged)
		if current, ok := normalizeListShapeEntries(d.config.Get(configKey)); ok && reflect.DeepEqual(current, mergedNormalized) {
			instance.lastWrittenValue = apiKey
			break
		}
		if lastAttempt {
			// Do NOT advance lastWrittenValue here - see the same guard in mergeIntoAdditionalEndpoints.
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

		// Additional endpoint domain, if this instance manages a dual-shipping key
		if instance.additionalEndpointDomain != "" {
			instanceInfo["AdditionalEndpointDomain"] = instance.additionalEndpointDomain
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
