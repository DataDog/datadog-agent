// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resolver contains logic to perform per `transaction.Endpoint` domain resolution. The idea behind this package
// is to allow the forwarder to send some data to a given domain and other kinds of data to other domains based on the
// targeted `transaction.Endpoint`.
package resolver

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// DestinationType is used to identified the expected endpoint
type DestinationType int

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
	// GetAPIKeys returns the list of API Keys associated with this `DomainResolver`
	GetAPIKeys() []string
	// GetBaseDomain returns the base domain for this `DomainResolver`
	GetBaseDomain() string
	// GetAlternateDomains returns all the domains that can be returned by `Resolve()` minus the base domain
	GetAlternateDomains() []string
	// SetBaseDomain sets the base domain to a new value
	SetBaseDomain(domain string)
	// UpdateAPIKey replaces instances of the oldKey with the newKey
	UpdateAPIKey(oldKey, newKey string)
	// GetBearerAuthToken returns Bearer authtoken, used for internal communication
	GetBearerAuthToken() string
}

// SingleDomainResolver will always return the same host
type SingleDomainResolver struct {
	domain  string
	apiKeys []string
	mu      sync.Mutex
}

// NewSingleDomainResolver creates a SingleDomainResolver with its destination domain & API keys
func NewSingleDomainResolver(domain string, apiKeys []string) *SingleDomainResolver {
	return &SingleDomainResolver{
		domain,
		apiKeys,
		sync.Mutex{},
	}
}

// NewSingleDomainResolvers converts a map of domain/api keys into a map of SingleDomainResolver
func NewSingleDomainResolvers(keysPerDomain map[string][]string) map[string]DomainResolver {
	resolvers := make(map[string]DomainResolver)
	for domain, keys := range keysPerDomain {
		resolvers[domain] = NewSingleDomainResolver(domain, keys)
	}
	return resolvers
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
	return r.apiKeys
}

// SetBaseDomain sets the only destination available for a SingleDomainResolver
func (r *SingleDomainResolver) SetBaseDomain(domain string) {
	r.domain = domain
}

// GetAlternateDomains always returns an empty slice for SingleDomainResolver
func (r *SingleDomainResolver) GetAlternateDomains() []string {
	return []string{}
}

// UpdateAPIKey replaces instances of the oldKey with the newKey
func (r *SingleDomainResolver) UpdateAPIKey(oldKey, newKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	replace := make([]string, 0, len(r.apiKeys))
	for _, key := range r.apiKeys {
		if key == oldKey {
			replace = append(replace, newKey)
		} else {
			replace = append(replace, key)
		}
	}
	r.apiKeys = replace
}

// GetBearerAuthToken is not implemented for SingleDomainResolver
func (r *SingleDomainResolver) GetBearerAuthToken() string {
	return ""
}

type destination struct {
	domain string
	dType  DestinationType
}

// MultiDomainResolver holds a default value and can provide alternate domain for some route
type MultiDomainResolver struct {
	baseDomain          string
	apiKeys             []string
	overrides           map[string]destination
	alternateDomainList []string
	mu                  sync.Mutex
}

// NewMultiDomainResolver initializes a MultiDomainResolver with its API keys and base destination
func NewMultiDomainResolver(baseDomain string, apiKeys []string) *MultiDomainResolver {
	return &MultiDomainResolver{
		baseDomain,
		apiKeys,
		make(map[string]destination),
		[]string{},
		sync.Mutex{},
	}
}

// GetAPIKeys returns the slice of API keys associated with this SingleDomainResolver
func (r *MultiDomainResolver) GetAPIKeys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.apiKeys
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
func (r *MultiDomainResolver) UpdateAPIKey(oldKey, newKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	replace := make([]string, 0, len(r.apiKeys))
	for _, key := range r.apiKeys {
		if key == oldKey {
			replace = append(replace, newKey)
		} else {
			replace = append(replace, key)
		}
	}
	r.apiKeys = replace
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
	for _, alternateDomain := range r.alternateDomainList {
		if alternateDomain == domain {
			return
		}
	}
	r.alternateDomainList = append(r.alternateDomainList, domain)
}

// GetBearerAuthToken is not implemented for MultiDomainResolver
func (r *MultiDomainResolver) GetBearerAuthToken() string {
	return ""
}

// NewDomainResolverWithMetricToVector initialize a resolver with metrics diverted to a vector endpoint
func NewDomainResolverWithMetricToVector(mainEndpoint string, apiKeys []string, vectorEndpoint string) *MultiDomainResolver {
	r := NewMultiDomainResolver(mainEndpoint, apiKeys)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.V1SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SketchSeriesEndpoint.Name, Vector)
	return r
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

// SetBaseDomain sets the base domain to a new value
func (r *LocalDomainResolver) SetBaseDomain(domain string) {
	r.domain = domain
}

// GetAlternateDomains is not implemented for LocalDomainResolver
func (r *LocalDomainResolver) GetAlternateDomains() []string {
	return []string{}
}

// UpdateAPIKey is not implemented for LocalDomainResolver
func (r *LocalDomainResolver) UpdateAPIKey(_, _ string) {
}

// GetBearerAuthToken returns Bearer authtoken, used for internal communication
func (r *LocalDomainResolver) GetBearerAuthToken() string {
	return r.authToken
}
