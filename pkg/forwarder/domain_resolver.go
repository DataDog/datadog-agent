// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import "github.com/DataDog/datadog-agent/pkg/forwarder/transaction"

// DestinationType is use to identified the expected endpoint
type DestinationType int

const (
	// Datadog enpoints
	Datadog DestinationType = iota
	// Vector endpoints
	Vector
)

// DomainResolver interface abstracts endpoints host selection
type DomainResolver interface {
	Resolve(endpoint transaction.Endpoint) (string, DestinationType)
	GetAPIKeys() []string
	GetBaseDomain() string
	GetAlternateDomains() []string
	SetBaseDomain(domain string)
}

// SingleDomainResolver will always return the same host
type SingleDomainResolver struct {
	domain  string
	apiKeys []string
}

// NewSingleDomainResolver create a SingleDomainResolver with its destination domain & API keys
func NewSingleDomainResolver(domain string, apiKeys []string) *SingleDomainResolver {
	return &SingleDomainResolver{
		domain,
		apiKeys,
	}
}

// NewSingleDomainResolvers convert a map of domain/api keys into a map of SingleDomainResolver
func NewSingleDomainResolvers(keysPerDomain map[string][]string) map[string]DomainResolver {
	resolvers := make(map[string]DomainResolver)
	for domain, keys := range keysPerDomain {
		resolvers[domain] = NewSingleDomainResolver(domain, keys)
	}
	return resolvers
}

// Resolve always returns the only destination available for a SingleDomainResolver
func (r *SingleDomainResolver) Resolve(endpoint transaction.Endpoint) (string, DestinationType) {
	return r.domain, Datadog
}

// GetBaseDomain returns the only destination available for a SingleDomainResolver
func (r *SingleDomainResolver) GetBaseDomain() string {
	return r.domain
}

// GetAPIKeys returns the slice of API keys associated with this SingleDomainResolver
func (r *SingleDomainResolver) GetAPIKeys() []string {
	return r.apiKeys
}

// SetBaseDomain sets the only destination available for a SingleDomainResolver
func (r *SingleDomainResolver) SetBaseDomain(domain string) {
	r.domain = domain
}

// GetAlternateDomains always return an empty slice for SingleDomainResolver
func (r *SingleDomainResolver) GetAlternateDomains() []string {
	return []string{}
}

type destination struct {
	domain string
	dType  DestinationType
}

// MultiDomainResolver holds a default value and can provide alternate domain for some route
type MultiDomainResolver struct {
	baseDomain string
	apiKeys    []string
	// endpoint name => overriden hostname map
	overrides map[string]destination
}

// NewMultiDomainResolver initialize a MultiDomainResolver with its API keys and base destination
func NewMultiDomainResolver(baseDomain string, apiKeys []string) *MultiDomainResolver {
	return &MultiDomainResolver{
		baseDomain,
		apiKeys,
		make(map[string]destination),
	}
}

// GetAPIKeys returns the slice of API keys associated with this SingleDomainResolver
func (r *MultiDomainResolver) GetAPIKeys() []string {
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

// SetBaseDomain update the base domain
func (r *MultiDomainResolver) SetBaseDomain(domain string) {
	r.baseDomain = domain
}

// GetAlternateDomains return a slice with all alternate domain
func (r *MultiDomainResolver) GetAlternateDomains() []string {
	dedupDomainMap := make(map[string]bool)
	for _, dest := range r.overrides {
		dedupDomainMap[dest.domain] = true
	}
	domains := make([]string, 0, len(dedupDomainMap))
	for domain := range dedupDomainMap {
		domains = append(domains, domain)
	}
	return domains
}

// NewDomainResolverWithMetricToVector initialize a resolver with metrics diverted to a vector endpoint
func NewDomainResolverWithMetricToVector(mainEndpoint string, apiKeys []string, vectorEndpoint string) *MultiDomainResolver {
	dest := destination{
		vectorEndpoint,
		Vector,
	}
	overrides := map[string]destination{
		v1SeriesEndpoint.Name:     dest,
		seriesEndpoint.Name:       dest,
		sketchSeriesEndpoint.Name: dest,
	}
	return &MultiDomainResolver{
		mainEndpoint,
		apiKeys,
		overrides,
	}
}
