// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauthimpl implements the delegatedauth component interface
package delegatedauthimpl

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
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
)

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
	writebackPath   []string

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

	// credProvider is this instance's credential, read by consumers on the request path. Always
	// non-nil. See instanceProvider for the resolving/resolved/fallback lifecycle.
	// Distinct from provider above, which is the cloud auth-proof generator.
	credProvider *instanceProvider
	// fallbackAPIKey is the operator-supplied static key, applied only once an exchange has
	// actually failed.
	fallbackAPIKey string
	// skipConfigWriteback suppresses writing the resolved key back into the config tree, for
	// consumers that read the credential from provider instead.
	skipConfigWriteback bool

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
	// providers indexes each instance's credential provider so consumers built after discovery
	// has run can find the one they need. Keyed by config setting + destination; a destination
	// can carry more than one provider when several orgs dual-ship to it.
	providers map[providerKey][]registeredProvider

	// credentialCache shares instances only when identity and lifecycle settings match.
	credentialCache map[credentialCacheKey]*instanceProvider

	// additionalEndpointsMu serializes read-modify-write access to additional_endpoints config
	// values across concurrent instances. Separate from mu to avoid deadlocking with OnUpdate callbacks.
	additionalEndpointsMu sync.Mutex
}

// providerKey identifies the credential(s) for one destination of one config setting.
type providerKey struct {
	configKey   string
	destination string
}

// credentialCacheKey identifies one exchange and fallback lifecycle.
type credentialCacheKey struct {
	orgUUID             string
	targetSite          string
	providerName        string
	providerRegion      string
	refreshIntervalMins int
	fallbackFingerprint [sha256.Size]byte
}

func newCredentialCacheKey(params delegatedauth.InstanceParams, targetSite string, providerConfig common.ProviderConfig) credentialCacheKey {
	providerName := ""
	providerRegion := ""
	if providerConfig != nil {
		providerName = providerConfig.ProviderName()
		if awsConfig, ok := providerConfig.(*cloudauthconfig.AWSProviderConfig); ok {
			providerRegion = awsConfig.Region
		}
	}
	refreshInterval := params.RefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = 60
	}
	return credentialCacheKey{
		orgUUID:             params.OrgUUID,
		targetSite:          targetSite,
		providerName:        providerName,
		providerRegion:      providerRegion,
		refreshIntervalMins: refreshInterval,
		fallbackFingerprint: sha256.Sum256([]byte(params.FallbackAPIKey)),
	}
}

func (d *delegatedAuthComponent) recordUnavailableInstance(params delegatedauth.InstanceParams, provider *instanceProvider, err error) {
	instance := fallbackTargetInstance(params)
	refreshInterval := params.RefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = 60
	}
	instance.refreshInterval = time.Duration(refreshInterval) * time.Minute
	instance.credProvider = provider
	instance.fallbackAPIKey = params.FallbackAPIKey
	instance.skipConfigWriteback = params.SkipConfigWriteback
	instance.consecutiveFailures = 1
	instance.lastError = err

	d.mu.Lock()
	d.instances[params.APIKeyConfigKey] = instance
	d.mu.Unlock()
}

// registeredProvider keeps the directive alongside its provider so a consumer that owns one
// specific directive can find its own credential rather than whichever was registered first.
type registeredProvider struct {
	directive string
	provider  delegatedauth.Provider
}

// Provides list the provided interfaces from the delegatedauth Component
type Provides struct {
	Comp           delegatedauth.Component
	StatusProvider status.InformationProvider
}

// NewComponent creates a new delegated auth Component
func NewComponent() Provides {
	comp := &delegatedAuthComponent{
		instances:       make(map[string]*authInstance),
		providers:       make(map[providerKey][]registeredProvider),
		credentialCache: make(map[credentialCacheKey]*instanceProvider),
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

	// Shared initialization always auto-detects the cloud provider and region from the
	// environment and the process-wide config. It must never use a per-instance ProviderConfig:
	// the first directive to call AddInstance would pin the component-wide default to its own
	// overrides (e.g. a specific AWS region), and later directives with no overrides would inherit
	// that default instead of auto-detecting. Per-instance config is applied later by
	// providerConfigForInstance.
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
func (d *delegatedAuthComponent) AddInstance(ctx context.Context, params delegatedauth.InstanceParams) (delegatedauth.Provider, error) {
	// Validate required parameters first
	if params.Config == nil {
		return nil, errors.New("config is required")
	}
	if params.OrgUUID == "" {
		return nil, errors.New("org_uuid is required")
	}
	if params.APIKeyConfigKey == "" {
		return nil, errors.New("api_key_config_key is required")
	}
	if params.AdditionalEndpointDomain != "" {
		if params.AdditionalEndpointDirective == "" {
			return nil, errors.New("additional_endpoint_directive is required when additional_endpoint_domain is set")
		}
		if params.AdditionalEndpointsConfigKey == "" {
			return nil, errors.New("additional_endpoints_config_key is required when additional_endpoint_domain is set")
		}
	}
	if params.AdditionalEndpointsListConfigKey != "" && params.AdditionalEndpointDirective == "" {
		return nil, errors.New("additional_endpoint_directive is required when additional_endpoints_list_config_key is set")
	}
	if params.AdditionalEndpointDomain != "" && params.AdditionalEndpointsListConfigKey != "" {
		return nil, errors.New("additional_endpoint_domain and additional_endpoints_list_config_key are mutually exclusive")
	}

	// Check for context cancellation early
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Initialize on first call - this detects cloud provider without holding locks.
	// An explicit provider configuration belongs to this instance, so it can override
	// the process-wide detected/default configuration.
	providerConfig, err := d.initializeIfNeeded(ctx, params)
	if err != nil {
		return nil, err
	}
	providerConfig = providerConfigForInstance(providerConfig, params.ProviderConfig)

	// No provider: delegated auth cannot work on this host at all. That is a terminal failure to
	// resolve - detection only runs once, there is no retry to wait for - so the fallback applies
	// immediately. Without a fallback the provider keeps reporting "no credential" and consumers
	// buffer rather than ship unauthenticated.
	if providerConfig == nil {
		log.Warnf("Delegated auth is not available on this host, so '%s' will keep its statically configured value", params.APIKeyConfigKey)
		unavailableErr := errors.New("no delegated auth provider is available on this host")

		// Deduplicate only in directive mode (SkipConfigWriteback). In flat-key mode each config
		// slot needs its own instance to write the fallback key back into config.
		if params.SkipConfigWriteback {
			targetSite := resolveTargetSite(params)
			cacheKey := newCredentialCacheKey(params, targetSite, params.ProviderConfig)
			d.mu.Lock()
			if existing, ok := d.credentialCache[cacheKey]; ok {
				d.mu.Unlock()
				d.recordUnavailableInstance(params, existing, unavailableErr)
				d.registerProvider(params, existing)
				return existing, nil
			}
			credProvider := newInstanceProvider()
			credProvider.setFallback(params.FallbackAPIKey)
			if d.credentialCache == nil {
				d.credentialCache = make(map[credentialCacheKey]*instanceProvider)
			}
			d.credentialCache[cacheKey] = credProvider
			d.mu.Unlock()

			d.recordUnavailableInstance(params, credProvider, unavailableErr)
			d.registerProvider(params, credProvider)
			return credProvider, nil
		}

		// Flat-key path: each slot needs its own write-back.
		credProvider := newInstanceProvider()
		credProvider.setFallback(params.FallbackAPIKey)
		if params.FallbackAPIKey != "" {
			d.writeAPIKeyToTarget(fallbackTargetInstance(params), params.FallbackAPIKey, true)
		}
		d.recordUnavailableInstance(params, credProvider, unavailableErr)
		d.registerProvider(params, credProvider)
		return credProvider, nil
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
		return nil, fmt.Errorf("unsupported delegated auth provider config type: %T", providerConfig)
	}

	authConfig := &common.AuthConfig{
		OrgUUID: params.OrgUUID,
	}

	targetSite := resolveTargetSite(params)

	// Deduplicate WIF exchanges only in directive mode (SkipConfigWriteback). In flat-key mode
	// each config slot needs its own authInstance to write the resolved key back into config.
	var credProvider *instanceProvider
	if params.SkipConfigWriteback {
		cacheKey := newCredentialCacheKey(params, targetSite, providerConfig)
		d.mu.Lock()
		if existing, ok := d.credentialCache[cacheKey]; ok {
			d.mu.Unlock()
			log.Infof("Reusing existing delegated auth credential for '%s' (org %s already has a provider for %s)",
				apiKeyConfigKey, params.OrgUUID, targetSite)
			d.registerProvider(params, existing)
			return existing, nil
		}
		// Reserve the slot so a concurrent AddInstance for the same org+site sees it.
		credProvider = newInstanceProvider()
		if d.credentialCache == nil {
			d.credentialCache = make(map[credentialCacheKey]*instanceProvider)
		}
		d.credentialCache[cacheKey] = credProvider
		d.mu.Unlock()
	} else {
		// Flat-key path: each slot gets its own provider and its own write-back goroutine.
		credProvider = newInstanceProvider()
	}

	// Create a context for the background refresh goroutine
	refreshCtx, refreshCancel := context.WithCancel(context.Background())

	// Create new auth instance with backoff configured
	instance := &authInstance{
		provider:                         tokenProvider,
		authConfig:                       authConfig,
		refreshInterval:                  refreshInterval,
		apiKeyConfigKey:                  apiKeyConfigKey,
		writebackPath:                    append([]string(nil), params.WritebackPath...),
		targetSite:                       targetSite,
		additionalEndpointDomain:         params.AdditionalEndpointDomain,
		additionalEndpointsConfigKey:     params.AdditionalEndpointsConfigKey,
		additionalEndpointKeyIndex:       params.AdditionalEndpointKeyIndex,
		additionalEndpointsListConfigKey: params.AdditionalEndpointsListConfigKey,
		listEntryIndex:                   params.ListEntryIndex,
		lastWrittenValue:                 params.AdditionalEndpointDirective,
		originalDirective:                params.AdditionalEndpointDirective,
		backoff:                          newBackoff(refreshInterval),
		credProvider:                     credProvider,
		fallbackAPIKey:                   params.FallbackAPIKey,
		skipConfigWriteback:              params.SkipConfigWriteback,
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
			return nil, ctx.Err()
		}
	}

	log.Infof("Delegated authentication is enabled for '%s', fetching initial API key...", apiKeyConfigKey)

	// Fetch the initial API key synchronously using the caller's context
	apiKey, _, err := d.refreshAndGetAPIKey(ctx, instance, false)
	if err != nil {
		log.Errorf("Failed to get initial delegated API key for '%s': %v", apiKeyConfigKey, err)
		// The exchange failed, so the fallback applies now and consumers can start shipping under
		// it while retries continue. With no fallback the provider stays in its buffering state.
		instance.credProvider.setFallback(instance.fallbackAPIKey)
		if instance.fallbackAPIKey != "" && !instance.skipConfigWriteback {
			d.writeAPIKeyToTarget(instance, instance.fallbackAPIKey, true)
		}
		// Record the failure so the status page shows it immediately.
		d.mu.Lock()
		instance.consecutiveFailures++
		instance.lastError = err
		d.mu.Unlock()
	} else {
		d.deliverAPIKey(instance, *apiKey)
		log.Infof("Successfully fetched and set initial delegated API key for '%s'", apiKeyConfigKey)
	}

	// refreshTrigger is the channel Refresh() sends to for an immediate re-exchange (e.g. on
	// 403). Capacity 1 so a burst of calls coalesces into one refresh.
	instance.credProvider.setRefreshTrigger(make(chan struct{}, 1))

	// Always start the background refresh goroutine, even if initial fetch failed
	// This ensures retries will happen with exponential backoff
	d.startBackgroundRefresh(instance)

	d.registerProvider(params, instance.credProvider)
	return instance.credProvider, nil
}

// deliverAPIKey hands a freshly fetched key to this instance's consumers: always through the
// provider, and additionally through the config tree unless the caller opted out.
func (d *delegatedAuthComponent) deliverAPIKey(instance *authInstance, apiKey string) {
	instance.credProvider.setResolved(apiKey)
	if !instance.skipConfigWriteback {
		d.updateConfigWithAPIKey(instance, apiKey)
	}
}

// registerProvider indexes a provider so consumers constructed after discovery can find it.
func (d *delegatedAuthComponent) registerProvider(params delegatedauth.InstanceParams, p delegatedauth.Provider) {
	configKey, destination := params.ProviderKey()
	key := providerKey{configKey: configKey, destination: destination}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.providers == nil {
		// Tests construct the component struct directly rather than through NewComponent.
		d.providers = make(map[providerKey][]registeredProvider)
	}

	// If this directive was previously registered (e.g. AddInstance replacing an existing
	// instance), remove the old entry before appending the new one. Without this, lookup can
	// return the old (cancelled) provider, and ProvidersFor returns both.
	existing := d.providers[key]
	for i, r := range existing {
		if r.directive == params.Directive {
			existing = append(existing[:i], existing[i+1:]...)
			break
		}
	}
	d.providers[key] = append(existing, registeredProvider{directive: params.Directive, provider: p})
}

// ProvidersFor implements delegatedauth.Component.
func (d *delegatedAuthComponent) ProvidersFor(configKey, destination string) []delegatedauth.Provider {
	d.mu.RLock()
	defer d.mu.RUnlock()
	found := d.providers[providerKey{configKey: configKey, destination: destination}]
	if len(found) == 0 {
		return nil
	}
	// Copy so callers cannot mutate the registry, and so a concurrent AddInstance appending to the
	// same key cannot be observed mid-append.
	out := make([]delegatedauth.Provider, len(found))
	for i, r := range found {
		out[i] = r.provider
	}
	return out
}

// ProviderForDirective implements delegatedauth.Component.
func (d *delegatedAuthComponent) ProviderForDirective(configKey, destination, directive string) delegatedauth.Provider {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, r := range d.providers[providerKey{configKey: configKey, destination: destination}] {
		// Two identical directives under one destination name the same org with the same
		// parameters, so they resolve to interchangeable credentials; the first is correct.
		if r.directive == directive {
			return r.provider
		}
	}
	return nil
}

// RefreshFor implements delegatedauth.Component.
func (d *delegatedAuthComponent) RefreshFor(configKey, destination, credential string) bool {
	d.mu.RLock()
	registered := slices.Clone(d.providers[providerKey{configKey: configKey, destination: destination}])
	d.mu.RUnlock()

	for _, entry := range registered {
		provider, ok := entry.provider.(*instanceProvider)
		if ok && provider.matches(credential) {
			return provider.Refresh()
		}
	}
	return false
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
// with exponential backoff on failures. It also listens on the instance's refreshTrigger channel
// for immediate refreshes requested by Refresh() (e.g. on a 403 from the intake).
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
				d.doRefresh(instance, ticker, false)
			case <-instance.credProvider.refreshTrigger:
				d.doRefresh(instance, ticker, true)
				// Drain any triggers that accumulated during the refresh so a burst of 403s
				// does not cause back-to-back refreshes.
				select {
				case <-instance.credProvider.refreshTrigger:
				default:
				}
			}
		}
	}()
}

// doRefresh performs one refresh cycle, updates the instance state under mu, resets the ticker
// for the next interval, and delivers the new key (if any) outside the lock.
func (d *delegatedAuthComponent) doRefresh(instance *authInstance, ticker *time.Ticker, _ bool) {
	lCreds, updated, lErr := d.refreshAndGetAPIKey(instance.refreshCtx, instance, true)

	var shouldUpdateConfig bool
	var apiKeyToUpdate string
	var fallbackActivated bool

	d.mu.Lock()
	if lErr != nil {
		if instance.refreshCtx.Err() != nil {
			d.mu.Unlock()
			log.Debugf("Refresh for '%s' failed due to context cancellation, exiting", instance.apiKeyConfigKey)
			return
		}

		instance.consecutiveFailures++
		instance.lastError = lErr
		fallbackActivated = instance.credProvider.setFallback(instance.fallbackAPIKey)

		nextInterval := instance.backoff.NextBackOff()
		instance.nextRefresh = time.Now().Add(nextInterval)
		log.Errorf("Failed to refresh delegated API key for '%s' (attempt %d): %v. Next retry in %v",
			instance.apiKeyConfigKey, instance.consecutiveFailures, lErr, nextInterval)
		ticker.Reset(nextInterval)
	} else {
		if instance.consecutiveFailures > 0 {
			log.Infof("Successfully refreshed delegated API key for '%s' after %d failed attempts",
				instance.apiKeyConfigKey, instance.consecutiveFailures)
		}
		instance.consecutiveFailures = 0
		instance.backoff.Reset()
		nextInterval := instance.backoff.NextBackOff()
		instance.nextRefresh = time.Now().Add(nextInterval)

		if updated && lCreds != nil {
			shouldUpdateConfig = true
			apiKeyToUpdate = *lCreds
		}

		ticker.Reset(nextInterval)
	}
	d.mu.Unlock()

	if shouldUpdateConfig {
		d.deliverAPIKey(instance, apiKeyToUpdate)
	} else if fallbackActivated && !instance.skipConfigWriteback {
		d.writeAPIKeyToTarget(instance, instance.fallbackAPIKey, true)
	}
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
