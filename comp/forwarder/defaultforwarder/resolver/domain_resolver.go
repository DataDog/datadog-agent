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
	"slices"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
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

// DomainResolver interface abstracts domain selection by `transaction.Endpoint`
type DomainResolver interface {
	// Resolve returns the domain to be used to send data for a given `transaction.Endpoint` along with a
	// destination type
	Resolve(endpoint transaction.Endpoint) (string, DestinationType)
	// GetAPIKeysInfo returns the list of API Keys and config paths associated with this `DomainResolver`
	GetAPIKeysInfo() ([]utils.APIKeys, int)
	// GetAPIKeys returns the list of API Keys associated with this `DomainResolver`
	GetAPIKeys() []string
	// UpdateAPIKeys updates the api keys at the given config path and sets the deduped keys to the new list.
	UpdateAPIKeys(configPath string, newKeys []utils.APIKeys)
	// GetAPIKeyVersion gets the current version for the API keys (version should be incremented each time the
	// keys are updated).
	GetAPIKeyVersion() int
	// GetBaseDomain returns the base domain for this `DomainResolver`
	GetBaseDomain() string
	// GetAlternateDomains returns all the domains that can be returned by `Resolve()` minus the base domain
	GetAlternateDomains() []string
	// SetBaseDomain sets the base domain to a new value
	SetBaseDomain(domain string)
	// UpdateAPIKey replaces instances of the oldKey with the newKey
	UpdateAPIKey(configPath, oldKey, newKey string)
	// GetBearerAuthToken returns Bearer authtoken, used for internal communication
	GetBearerAuthToken() string
	// GetForwarderHealth returns the health checker
	GetForwarderHealth() ForwarderHealth
	// SetForwarderHealth sets the health checker for this domain
	// Needed so we update the health checker when API keys are updated
	SetForwarderHealth(ForwarderHealth)
}

// SingleDomainResolver will always return the same host
type SingleDomainResolver struct {
	domain         string
	apiKeys        []utils.APIKeys
	keyVersion     int
	dedupedAPIKeys []string
	mu             sync.Mutex
	healthChecker  ForwarderHealth
}

// OnUpdateConfig adds a hook into the config which will listen for updates to the API keys
// of the resolver.
func OnUpdateConfig(resolver DomainResolver, log log.Component, config config.Component) {
	config.OnUpdate(func(setting string, oldValue, newValue any, _ uint64) {
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
				health.UpdateAPIKeys(resolver.GetBaseDomain(), []string{oldAPIKey}, []string{newAPIKey})
			}

			log.Infof("rotating API key for '%s': %s -> %s",
				setting,
				scrubber.HideKeyExceptLastFiveChars(oldAPIKey),
				scrubber.HideKeyExceptLastFiveChars(newAPIKey),
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
		health.UpdateAPIKeys(resolver.GetBaseDomain(), removed, added)
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

// NewSingleDomainResolver creates a SingleDomainResolver with its destination domain & API keys
func NewSingleDomainResolver(domain string, apiKeys []utils.APIKeys) (*SingleDomainResolver, error) {
	// Ensure all API keys have a config setting path so we can keep track to ensure they are updated
	// when the config changes.
	for key := range apiKeys {
		if apiKeys[key].ConfigSettingPath == "" {
			return nil, fmt.Errorf("Api key for %v does not specify a config setting path", domain)
		}
	}

	deduped := utils.DedupAPIKeys(apiKeys)

	return &SingleDomainResolver{
		domain:         domain,
		apiKeys:        apiKeys,
		keyVersion:     0,
		dedupedAPIKeys: deduped,
		mu:             sync.Mutex{},
	}, nil
}

// NewSingleDomainResolvers converts a map of domain/api keys into a map of SingleDomainResolver
func NewSingleDomainResolvers(keysPerDomain map[string][]utils.APIKeys) (map[string]DomainResolver, error) {
	resolvers := make(map[string]DomainResolver)
	for domain, keys := range keysPerDomain {
		var err error
		resolvers[domain], err = NewSingleDomainResolver(domain, keys)
		if err != nil {
			return nil, err
		}
	}
	return resolvers, nil
}

// Resolve always returns the only destination available for a SingleDomainResolver
func (r *SingleDomainResolver) Resolve(transaction.Endpoint) (string, DestinationType) {
	return r.domain, Datadog
}

// GetBaseDomain returns the only destination available for a SingleDomainResolver
func (r *SingleDomainResolver) GetBaseDomain() string {
	return r.domain
}

// GetAPIKeys returns the slice of API keys associated with this SingleDomainResolver
func (r *SingleDomainResolver) GetAPIKeys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dedupedAPIKeys
}

// GetAPIKeyVersion get the version of the keys.
func (r *SingleDomainResolver) GetAPIKeyVersion() int {
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
		keys[i] = scrubber.HideKeyExceptLastFiveChars(key)
	}
	return keys
}

// GetAPIKeysInfo returns the list of APIKeys and config paths associated with this `DomainResolver`
func (r *SingleDomainResolver) GetAPIKeysInfo() ([]utils.APIKeys, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.apiKeys, r.keyVersion
}

// SetBaseDomain sets the only destination available for a SingleDomainResolver
func (r *SingleDomainResolver) SetBaseDomain(domain string) {
	r.domain = domain
}

// GetAlternateDomains always returns an empty slice for SingleDomainResolver
func (r *SingleDomainResolver) GetAlternateDomains() []string {
	return []string{}
}

// UpdateAPIKeys updates the api keys at the given config path and sets the deduped keys to the new list.
func (r *SingleDomainResolver) UpdateAPIKeys(configPath string, newKeys []utils.APIKeys) {
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
	r.keyVersion++
}

// UpdateAPIKey replaces instances of the oldKey with the newKey
func (r *SingleDomainResolver) UpdateAPIKey(configPath, oldKey, newKey string) {
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
func (r *SingleDomainResolver) GetBearerAuthToken() string {
	return ""

}

// GetForwarderHealth returns the health checker
func (r *SingleDomainResolver) GetForwarderHealth() ForwarderHealth {
	return r.healthChecker
}

// SetForwarderHealth sets the health checker for this domain
// Needed so we update the health checker when API keys are updated
func (r *SingleDomainResolver) SetForwarderHealth(healthChecker ForwarderHealth) {
	r.healthChecker = healthChecker
}

type destination struct {
	domain string
	dType  DestinationType
}

// MultiDomainResolver holds a default value and can provide alternate domain for some route
type MultiDomainResolver struct {
	baseDomain          string
	apiKeys             []utils.APIKeys
	keyVersion          int
	dedupedAPIKeys      []string
	overrides           map[string]destination
	alternateDomainList []string
	mu                  sync.Mutex
	healthChecker       ForwarderHealth
}

// NewMultiDomainResolver initializes a MultiDomainResolver with its API keys and base destination
func NewMultiDomainResolver(baseDomain string, apiKeys []utils.APIKeys) (*MultiDomainResolver, error) {
	// Ensure all API keys have a config setting path so we can keep track to ensure they are updated
	// when the config changes.
	for key := range apiKeys {
		if apiKeys[key].ConfigSettingPath == "" {
			return nil, fmt.Errorf("Api key for %v does not specify a config setting path", baseDomain)
		}
	}

	deduped := utils.DedupAPIKeys(apiKeys)

	return &MultiDomainResolver{
		baseDomain:          baseDomain,
		apiKeys:             apiKeys,
		keyVersion:          0,
		dedupedAPIKeys:      deduped,
		overrides:           make(map[string]destination),
		alternateDomainList: []string{},
		mu:                  sync.Mutex{},
	}, nil
}

// GetAPIKeys returns the slice of API keys associated with this SingleDomainResolver
func (r *MultiDomainResolver) GetAPIKeys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dedupedAPIKeys
}

// GetAPIKeyVersion get the version of the keys
func (r *MultiDomainResolver) GetAPIKeyVersion() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.keyVersion
}

// UpdateAPIKeys updates the api keys at the given config path and sets the deduped keys to the new list.
func (r *MultiDomainResolver) UpdateAPIKeys(configPath string, newKeys []utils.APIKeys) {
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

	r.keyVersion++
}

// GetAPIKeysInfo returns the list of endpoints associated with this `DomainResolver`
func (r *MultiDomainResolver) GetAPIKeysInfo() ([]utils.APIKeys, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.apiKeys, r.keyVersion
}

// Resolve returns the destiation for a given request endpoint
func (r *MultiDomainResolver) Resolve(endpoint transaction.Endpoint) (string, DestinationType) {
	if d, ok := r.overrides[endpoint.Name]; ok {
		return d.domain, d.dType
	}
	return r.baseDomain, Datadog
}

// GetBaseDomain returns the base domain
func (r *MultiDomainResolver) GetBaseDomain() string {
	return r.baseDomain
}

// SetBaseDomain updates the base domain
func (r *MultiDomainResolver) SetBaseDomain(domain string) {
	r.baseDomain = domain
}

// GetAlternateDomains returns a slice with all alternate domain
func (r *MultiDomainResolver) GetAlternateDomains() []string {
	return r.alternateDomainList
}

// UpdateAPIKey replaces instances of the oldKey with the newKey
func (r *MultiDomainResolver) UpdateAPIKey(configPath, oldKey, newKey string) {
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
}

// RegisterAlternateDestination adds an alternate destination to a MultiDomainResolver.
// The resolver will match transaction.Endpoint.Name against forwarderName to check if the request shall
// be diverted.
func (r *MultiDomainResolver) RegisterAlternateDestination(domain string, forwarderName string, dType DestinationType) {
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

// GetBearerAuthToken is not implemented for MultiDomainResolver
func (r *MultiDomainResolver) GetBearerAuthToken() string {
	return ""
}

// GetForwarderHealth returns the health checker
func (r *MultiDomainResolver) GetForwarderHealth() ForwarderHealth {
	return r.healthChecker
}

// SetForwarderHealth sets the health checker for this domain
// Needed so we update the health checker when API keys are updated
func (r *MultiDomainResolver) SetForwarderHealth(healthChecker ForwarderHealth) {
	r.healthChecker = healthChecker
}

// NewDomainResolverWithMetricToVector initialize a resolver with metrics diverted to a vector endpoint
func NewDomainResolverWithMetricToVector(mainEndpoint string, apiKeys []utils.APIKeys, vectorEndpoint string) (*MultiDomainResolver, error) {
	r, err := NewMultiDomainResolver(mainEndpoint, apiKeys)
	if err != nil {
		return nil, err
	}
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.V1SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SketchSeriesEndpoint.Name, Vector)
	return r, nil
}

// LocalDomainResolver contains domain address in local cluster and authToken for internal communication
type LocalDomainResolver struct {
	domain    string
	authToken string
}

// NewLocalDomainResolver creates a LocalDomainResolver with domain in local cluster and authToken for internal communication
// For example, the internal cluster-agent endpoint
func NewLocalDomainResolver(domain string, authToken string) *LocalDomainResolver {
	return &LocalDomainResolver{
		domain,
		authToken,
	}
}

// Resolve returns the domain to be used to send data and local destination type
func (r *LocalDomainResolver) Resolve(transaction.Endpoint) (string, DestinationType) {
	return r.domain, Local
}

// GetBaseDomain returns the base domain for this LocalDomainResolver
func (r *LocalDomainResolver) GetBaseDomain() string {
	return r.domain
}

// GetAPIKeys is not implemented for LocalDomainResolver
func (r *LocalDomainResolver) GetAPIKeys() []string {
	return []string{}
}

// GetAPIKeyVersion get the version of the keys
func (r *LocalDomainResolver) GetAPIKeyVersion() int {
	return 0
}

// GetAPIKeysInfo returns the list of endpoints associated with this `DomainResolver`
func (r *LocalDomainResolver) GetAPIKeysInfo() ([]utils.APIKeys, int) {
	return []utils.APIKeys{}, 0
}

// SetBaseDomain sets the base domain to a new value
func (r *LocalDomainResolver) SetBaseDomain(domain string) {
	r.domain = domain
}

// GetAlternateDomains is not implemented for LocalDomainResolver
func (r *LocalDomainResolver) GetAlternateDomains() []string {
	return []string{}
}

// UpdateAPIKeys is not implemented for LocalDomainResolver
func (r *LocalDomainResolver) UpdateAPIKeys(_ string, _ []utils.APIKeys) {
}

// UpdateAPIKey is not implemented for LocalDomainResolver
func (r *LocalDomainResolver) UpdateAPIKey(_, _, _ string) {
}

// GetBearerAuthToken returns Bearer authtoken, used for internal communication
func (r *LocalDomainResolver) GetBearerAuthToken() string {
	return r.authToken
}

// GetForwarderHealth returns the health checker
// Not used for LocalDomainResolver
func (r *LocalDomainResolver) GetForwarderHealth() ForwarderHealth {
	return nil
}

// SetForwarderHealth sets the health checker for this domain
// Not used for LocalDomainResolver
func (r *LocalDomainResolver) SetForwarderHealth(_ ForwarderHealth) {
}
