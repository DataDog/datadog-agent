// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resolver contains logic to perform per `transaction.Endpoint` domain resolution. The idea behind this package
// is to allow the forwarder to send some data to a given domain and other kinds of data to other domains based on the
// targeted `transaction.Endpoint`.
package resolver

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/credential"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// DestinationType is used to identified the expected endpoint
type DestinationType int

// ForwarderHealth interface is implemented by the health checker. The resolver
// uses this method to inform the healthchecker when API keys have been updated.
type ForwarderHealth interface {
	UpdateAPIKeys(domain string, old []string, new []string)
}

const (
	// Datadog enpoints
	Datadog DestinationType = iota
	// Vector endpoints
	Vector
	// Local endpoints
	Local
)

// DomainResolver is a syntactic backwards compatibility shim
type DomainResolver = *domainResolver

// SingleDomainResolver will always return the same host
type domainResolver struct {
	// configName is the url as it was configured by the user.
	configName string
	// domain is the url base to be used for network requests, it is modified by the forwarder.
	domain         string
	apiKeys        []utils.APIKeys
	keyVersion     int
	dedupedAPIKeys []string
	// credentialProviders supply credentials at send time for destinations whose key is not a
	// config value - delegated auth, today. Each one gets its own authorization slot, and hence
	// its own transaction, even before it has a credential to offer.
	credentialProviders []CredentialProvider
	// hasPendingDelegatedAuth mirrors HasPendingDelegatedAuth across apiKeys: true when the domain
	// has no real API key yet but is waiting on one from the delegatedauth component. Kept in sync
	// with apiKeys by hasPendingDelegatedAuthKeys() wherever apiKeys is replaced.
	hasPendingDelegatedAuth bool
	mu                      sync.Mutex
	healthChecker           ForwarderHealth
	destinationType         DestinationType
	authToken               string

	overrides           map[string]destination
	alternateDomainList []string

	isMRF            bool
	isMetricToVector bool
}

// OnUpdateConfig adds a hook into the config which will listen for updates to the API keys
// of the resolver.
func OnUpdateConfig(resolver DomainResolver, log log.Component, config config.Component) {
	config.OnUpdate(func(setting string, _ model.Source, oldValue, newValue any, _ uint64) {
		found := false

		apiKeys, _ := resolver.GetAPIKeysInfo()
		for _, endpoint := range apiKeys {
			if endpoint.ConfigSettingPath == setting {
				found = true
				break
			}
		}

		if !found {
			return
		}

		if strings.Contains(setting, "additional_endpoints") {
			// Updating additional endpoints don't give us the exact key that has been updated so we reload the whole config section.
			updateAdditionalEndpoints(resolver, setting, config, log)
			return
		}

		oldAPIKey, ok1 := oldValue.(string)
		newAPIKey, ok2 := newValue.(string)
		if ok1 && ok2 {
			resolver.UpdateAPIKey(setting, oldAPIKey, newAPIKey)

			if health := resolver.GetForwarderHealth(); health != nil {
				health.UpdateAPIKeys(resolver.GetConfigName(), []string{oldAPIKey}, []string{newAPIKey})
			}

			log.Infof("rotating API key for '%s': %s -> %s",
				setting,
				scrubber.HideKeyExceptLastChars(oldAPIKey),
				scrubber.HideKeyExceptLastChars(newAPIKey),
			)

			return
		}

		if ok1 {
			log.Errorf("new API key for '%s' is invalid (not a string) ignoring new value", setting)
		} else if ok2 {
			log.Errorf("old API key for '%s' is invalid (not a string) ignoring new value", setting)
		} else {
			log.Errorf("new and old API key for '%s' is invalid (not a string) ignoring new value", setting)
		}
	})
}

// updateAdditionalEndpoints handles updating an API key that is a part of additional endpoints.
// Since additional_endpoints are in the config as a map of domain to api key array, when the api key updates the updater
// will not know exactly which api key has been updated so we reload the whole list from the config and insert this
// into our list before deduping.
func updateAdditionalEndpoints(resolver DomainResolver, setting string, config config.Component, log log.Component) {
	additionalEndpoints := utils.MakeEndpoints(config.GetStringMapStringSlice(setting), setting)
	endpoints, ok := additionalEndpoints[resolver.GetBaseDomain()]
	if !ok {
		log.Errorf("error: the domain in additional_endpoints changed at runtime for '%s', discarding update.", resolver.GetBaseDomain())
		return
	}

	oldKeys := resolver.GetAPIKeys()
	resolver.UpdateAPIKeys(setting, endpoints)
	newKeys := resolver.GetAPIKeys()

	removed := missing(oldKeys, newKeys)
	added := missing(newKeys, oldKeys)

	if health := resolver.GetForwarderHealth(); health != nil {
		health.UpdateAPIKeys(resolver.GetConfigName(), removed, added)
	}

	removed = scrubKeys(removed)
	added = scrubKeys(added)

	// Not all calls here will involve changes to the api keys since we are just reloading every time something with
	// `additional_endpoints` contains a key that changes, there are potentially multiple resolvers for different
	// `additional_endpoints` configurations (eg, `process_config.additional_endpoints` and `additional_endpoints`)
	if len(removed) > 0 && len(added) > 0 {
		log.Infof("rotating API key for '%s': %s -> %s", setting, strings.Join(removed, ","), strings.Join(added, ","))
	} else if len(removed) > 0 {
		log.Infof("removing API key for '%s': %s", setting, strings.Join(removed, ","))
	} else if len(added) > 0 {
		log.Infof("adding API key for '%s': %s", setting, strings.Join(added, ","))
	}
}

// NewSingleDomainResolver2 creates a DomainResolver from an endpoint configuration object.
func NewSingleDomainResolver2(descriptor utils.EndpointDescriptor) (DomainResolver, error) {
	// Ensure all API keys have a config setting path so we can keep track to ensure they are updated
	// when the config changes.
	for _, keys := range descriptor.APIKeySet {
		if keys.ConfigSettingPath == "" {
			return nil, fmt.Errorf("API key for %v does not specify a config setting path", descriptor.BaseURL)
		}
	}

	deduped := utils.DedupAPIKeys(descriptor.APIKeySet)

	return &domainResolver{
		configName:     descriptor.BaseURL,
		domain:         descriptor.BaseURL,
		apiKeys:        descriptor.APIKeySet,
		keyVersion:     0,
		dedupedAPIKeys: deduped,
		// Derived from APIKeySet directly rather than trusting descriptor.HasPendingDelegatedAuth,
		// so this is correct even for a descriptor built without going through
		// utils.newEndpointDescriptor's aggregation (e.g. constructed directly in a test).
		hasPendingDelegatedAuth: hasPendingDelegatedAuthKeys(descriptor.APIKeySet),
		mu:                      sync.Mutex{},
		isMRF:                   descriptor.IsMRF,
	}, nil
}

// hasPendingDelegatedAuthKeys reports whether any of the given APIKeys is still waiting on a
// delegated-auth key. Mirrors the aggregation in utils.newEndpointDescriptor.
func hasPendingDelegatedAuthKeys(apiKeys []utils.APIKeys) bool {
	for _, keys := range apiKeys {
		if keys.HasPendingDelegatedAuth {
			return true
		}
	}
	return false
}

// NewSingleDomainResolvers converts a map of domain/api keys into a map of DomainResolver
func NewSingleDomainResolvers(keysPerDomain map[string][]utils.APIKeys) (map[string]DomainResolver, error) {
	return NewSingleDomainResolvers2(utils.EndpointDescriptorSetFromKeysPerDomain(keysPerDomain))
}

// NewSingleDomainResolvers2 creates a set of domain resolvers from an EndpointDescriptorSet.
func NewSingleDomainResolvers2(eds utils.EndpointDescriptorSet) (map[string]DomainResolver, error) {
	resolvers := make(map[string]DomainResolver)
	for _, ed := range eds {
		var err error
		resolvers[ed.BaseURL], err = NewSingleDomainResolver2(ed)
		if err != nil {
			return nil, err
		}
	}
	return resolvers, nil
}

// GetBaseDomain returns the only destination available for a SingleDomainResolver
func (r *domainResolver) GetBaseDomain() string {
	return r.domain
}

// GetAPIKeys returns the slice of API keys associated with this SingleDomainResolver
func (r *domainResolver) GetAPIKeys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dedupedAPIKeys
}

// GetAPIKeyVersion get the version of the keys.
func (r *domainResolver) GetAPIKeyVersion() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.keyVersion
}

// missing returns a list of elements that are in list a, but not in list b.
// This is inefficient for large lists, but the assumption is that a config
// will only have a very small number of API keys specified.
func missing(a []string, b []string) []string {
	missing := []string{}

	for _, key := range a {
		if !slices.Contains(b, key) {
			missing = append(missing, key)
		}
	}

	return missing
}

// scrubKeys scrubs the API key to avoid leaking the key when logging.
func scrubKeys(keys []string) []string {
	for i, key := range keys {
		keys[i] = scrubber.HideKeyExceptLastChars(key)
	}
	return keys
}

// GetAPIKeysInfo returns the list of APIKeys and config paths associated with this `DomainResolver`
func (r *domainResolver) GetAPIKeysInfo() ([]utils.APIKeys, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.apiKeys, r.keyVersion
}

// SetBaseDomain sets the only destination available for a SingleDomainResolver
func (r *domainResolver) SetBaseDomain(domain string) {
	r.domain = domain
}

// UpdateAPIKeys updates the api keys at the given config path and sets the deduped keys to the new list.
func (r *domainResolver) UpdateAPIKeys(configPath string, newKeys []utils.APIKeys) {
	r.mu.Lock()
	defer r.mu.Unlock()
	newAPIKeys := make([]utils.APIKeys, 0)
	for idx := range r.apiKeys {
		if r.apiKeys[idx].ConfigSettingPath != configPath {
			newAPIKeys = append(newAPIKeys, r.apiKeys[idx])
		}
	}

	r.apiKeys = append(newAPIKeys, newKeys...)
	r.dedupedAPIKeys = utils.DedupAPIKeys(r.apiKeys)
	r.hasPendingDelegatedAuth = hasPendingDelegatedAuthKeys(r.apiKeys)
	r.keyVersion++
}

// UpdateAPIKey replaces instances of the oldKey with the newKey
func (r *domainResolver) UpdateAPIKey(configPath, oldKey, newKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for idx := range r.apiKeys {
		if r.apiKeys[idx].ConfigSettingPath == configPath {
			replace := make([]string, 0, len(r.apiKeys[idx].Keys))
			for _, key := range r.apiKeys[idx].Keys {
				if key == oldKey {
					replace = append(replace, newKey)
				} else {
					replace = append(replace, key)
				}
			}

			r.apiKeys[idx].Keys = replace
		}
	}

	r.dedupedAPIKeys = utils.DedupAPIKeys(r.apiKeys)
	r.keyVersion++
}

// GetBearerAuthToken is not implemented for SingleDomainResolver
func (r *domainResolver) GetBearerAuthToken() string {
	return r.authToken

}

// GetForwarderHealth returns the health checker
func (r *domainResolver) GetForwarderHealth() ForwarderHealth {
	return r.healthChecker
}

// SetForwarderHealth sets the health checker for this domain
// Needed so we update the health checker when API keys are updated
func (r *domainResolver) SetForwarderHealth(healthChecker ForwarderHealth) {
	r.healthChecker = healthChecker
}

type destination struct {
	domain string
	dType  DestinationType
}

// NewMultiDomainResolver initializes a MultiDomainResolver with its API keys and base destination
func NewMultiDomainResolver(domain string, apiKeys []utils.APIKeys) (DomainResolver, error) {
	// Ensure all API keys have a config setting path so we can keep track to ensure they are updated
	// when the config changes.
	for key := range apiKeys {
		if apiKeys[key].ConfigSettingPath == "" {
			return nil, fmt.Errorf("API key for %v does not specify a config setting path", domain)
		}
	}

	deduped := utils.DedupAPIKeys(apiKeys)

	return &domainResolver{
		configName:              domain,
		domain:                  domain,
		apiKeys:                 apiKeys,
		keyVersion:              0,
		dedupedAPIKeys:          deduped,
		hasPendingDelegatedAuth: hasPendingDelegatedAuthKeys(apiKeys),
		overrides:               make(map[string]destination),
		alternateDomainList:     []string{},
		mu:                      sync.Mutex{},
	}, nil
}

// Resolve returns the destiation for a given request endpoint
func (r *domainResolver) Resolve(endpoint transaction.Endpoint) string {
	if r.overrides != nil {
		if d, ok := r.overrides[endpoint.Name]; ok {
			return d.domain
		}
	}
	return r.domain
}

// GetAlternateDomains returns a slice with all alternate domain
func (r *domainResolver) GetAlternateDomains() []string {
	return r.alternateDomainList
}

// RegisterAlternateDestination adds an alternate destination to a MultiDomainResolver.
// The resolver will match transaction.Endpoint.Name against forwarderName to check if the request shall
// be diverted.
func (r *domainResolver) RegisterAlternateDestination(domain string, forwarderName string, dType DestinationType) {
	d := destination{
		domain: domain,
		dType:  dType,
	}
	r.overrides[forwarderName] = d
	if slices.Contains(r.alternateDomainList, domain) {
		return
	}
	r.alternateDomainList = append(r.alternateDomainList, domain)
}

// NewDomainResolverWithMetricToVector initialize a resolver with metrics diverted to a vector endpoint
func NewDomainResolverWithMetricToVector(mainEndpoint string, apiKeys []utils.APIKeys, vectorEndpoint string) (DomainResolver, error) {
	r, err := NewMultiDomainResolver(mainEndpoint, apiKeys)
	if err != nil {
		return nil, err
	}
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.V1SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.V3SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SketchSeriesEndpoint.Name, Vector)
	r.isMetricToVector = true
	return r, nil
}

// NewLocalDomainResolver creates a LocalDomainResolver with domain in local cluster and authToken for internal communication
// For example, the internal cluster-agent endpoint
func NewLocalDomainResolver(domain string, authToken string) DomainResolver {
	return &domainResolver{
		configName:      domain,
		domain:          domain,
		authToken:       authToken,
		destinationType: Local,
	}
}

// IsUsable returns true if the resolver has valid configuration. A domain with no real API keys
// yet, but with a delegated-auth directive still resolving, is kept usable so it stays registered
// and can pick up the real key once delegatedauth writes it back into config - otherwise the
// domain would be dropped at startup and the resolved key would have nowhere to go.
func (r *domainResolver) IsUsable() bool {
	return r.IsLocal() || len(r.dedupedAPIKeys) > 0 || r.hasPendingDelegatedAuth
}

// IsLocal returns true if the domain corresponds to another agent.
func (r *domainResolver) IsLocal() bool {
	return r.destinationType == Local
}

// IsMRF returns true when the domain is used as the target for multi region failover.
func (r *domainResolver) IsMRF() bool {
	return r.isMRF
}

// IsMetricToVector returns true when the resolver was constructed to divert metrics to a
// Vector/Observability Pipelines Worker endpoint via NewDomainResolverWithMetricToVector.
func (r *domainResolver) IsMetricToVector() bool {
	return r.isMetricToVector
}

// CredentialProvider supplies a credential for outbound requests at send time, rather than the
// resolver holding a key string up front. It is an alias for delegatedauth.Provider so consumers
// that import this package get the canonical interface without a separate import.
type CredentialProvider = credential.Provider

// credentialSource is one authorization slot on a domain. The forwarder creates one transaction
// per slot, so a slot must exist even when its credential has not arrived yet - otherwise the
// payload is dropped at creation instead of being retried once the credential lands.
type credentialSource interface {
	// authorize stamps credentials onto headers and reports whether it could.
	authorize(http.Header) bool
}

// staticAuthHeader is a fixed header/value pair, known at construction: an API key from config, or
// the cluster-agent bearer token.
type staticAuthHeader struct {
	key, value string
}

func (a staticAuthHeader) authorize(headers http.Header) bool {
	headers.Set(a.key, a.value)
	return true
}

// providerAuth defers to a CredentialProvider on every request, so a credential that resolves
// after startup is picked up with no rebuild and no config write.
type providerAuth struct {
	provider CredentialProvider
}

func (a providerAuth) authorize(headers http.Header) bool {
	return a.provider.Authorize(headers)
}

// GetAuthorizers returns one entry per authorization slot on this domain: one per credential
// provider, then the deduped static API keys. Providers come first so their indices are stable
// regardless of how many static keys are configured: a config update that adds or removes a
// static key shifts only the static-key indices, not the provider indices. This matters because
// on-disk transactions carry their APIKeyIndex, and a provider slot that shifted would either
// be dropped (out of range) or, worse, authenticated under a different credential.
func (r *domainResolver) GetAuthorizers() (res []credentialSource) {
	if r.IsLocal() {
		res = append(res, staticAuthHeader{
			key:   "Authorization",
			value: "Bearer " + r.authToken,
		})
	} else {
		for _, p := range r.GetCredentialProviders() {
			res = append(res, providerAuth{provider: p})
		}
		for _, key := range r.GetAPIKeys() {
			res = append(res, staticAuthHeader{
				key:   "DD-Api-Key",
				value: key,
			})
		}
	}
	return
}

// GetCredentialProviders returns the providers supplying credentials for this domain.
func (r *domainResolver) GetCredentialProviders() []CredentialProvider {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.credentialProviders
}

// SetCredentialProviders attaches credential providers to this domain, each getting its own
// authorization slot. Call it before the resolver is handed to the forwarder: the slot count
// determines how many transactions a payload fans out to, and on-disk transactions serialized
// against a different count are discarded on load.
func (r *domainResolver) SetCredentialProviders(providers []CredentialProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.credentialProviders = providers
}

// ErrCredentialNotReady reports that a credential for this slot has not arrived yet. Callers must
// not send. It is defined in the transaction package so the forwarder's circuit breaker can
// recognize it without resolver importing back into it.
var ErrCredentialNotReady = transaction.ErrCredentialNotReady

// Authorize stamps the credential for slot apiKeyIdx onto headers.
//
// It returns ErrCredentialNotReady when the slot is backed by a provider that has nothing yet. The
// caller must treat that as "retry later", never as "send unauthenticated".
func (r *domainResolver) Authorize(apiKeyIdx uint, headers http.Header, log log.Component) error {
	authorizers := r.GetAuthorizers()

	if apiKeyIdx >= uint(len(authorizers)) {
		log.Errorf("API key index %d is greater than the number of available authorizers (%d)", apiKeyIdx, len(authorizers))
		return fmt.Errorf("API key index %d out of range (have %d authorizers)", apiKeyIdx, len(authorizers))
	}
	if !authorizers[apiKeyIdx].authorize(headers) {
		return ErrCredentialNotReady
	}
	return nil
}

// GetConfigName returns the base url as it was originally written in the config.
func (r *domainResolver) GetConfigName() string {
	return r.configName
}
