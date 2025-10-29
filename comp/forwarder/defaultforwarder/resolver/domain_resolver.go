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
	"github.com/DataDog/datadog-agent/pkg/config/model"
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

// DomainResolver is a syntactic backwards compatibility shim
type DomainResolver = *domainResolver

// SingleDomainResolver will always return the same host
type domainResolver struct {
	domain          string
	apiKeys         []utils.APIKeys
	keyVersion      int
	dedupedAPIKeys  []string
	mu              sync.Mutex
	healthChecker   ForwarderHealth
	destinationType DestinationType
	authToken       string

	overrides           map[string]destination
	alternateDomainList []string
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
func NewSingleDomainResolver(domain string, apiKeys []utils.APIKeys) (DomainResolver, error) {
	// Ensure all API keys have a config setting path so we can keep track to ensure they are updated
	// when the config changes.
	for key := range apiKeys {
		if apiKeys[key].ConfigSettingPath == "" {
			return nil, fmt.Errorf("API key for %v does not specify a config setting path", domain)
		}
	}

	deduped := utils.DedupAPIKeys(apiKeys)

	return &domainResolver{
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
		keys[i] = scrubber.HideKeyExceptLastFiveChars(key)
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
		domain:              domain,
		apiKeys:             apiKeys,
		keyVersion:          0,
		dedupedAPIKeys:      deduped,
		overrides:           make(map[string]destination),
		alternateDomainList: []string{},
		mu:                  sync.Mutex{},
	}, nil
}

// Resolve returns the destiation for a given request endpoint
func (r *domainResolver) Resolve(endpoint transaction.Endpoint) (string, DestinationType) {
	if r.overrides != nil {
		if d, ok := r.overrides[endpoint.Name]; ok {
			return d.domain, d.dType
		}
	}
	return r.domain, r.destinationType
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
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SketchSeriesEndpoint.Name, Vector)
	return r, nil
}

// NewLocalDomainResolver creates a LocalDomainResolver with domain in local cluster and authToken for internal communication
// For example, the internal cluster-agent endpoint
func NewLocalDomainResolver(domain string, authToken string) DomainResolver {
	return &domainResolver{
		domain:          domain,
		authToken:       authToken,
		destinationType: Local,
	}
}

// IsUsable returns true if the resolver has valid configuration.
func (r *domainResolver) IsUsable() bool {
	return r.IsLocal() || len(r.dedupedAPIKeys) > 0
}

// IsLocal returns true if the domain corresponds to another agent.
func (r *domainResolver) IsLocal() bool {
	return r.destinationType == Local
}
