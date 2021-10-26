// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resolver contains logic to perform per `transaction.Endpoint` domain resolution. The idea behind this package
// is to allow the forwarder to send some data to a given domain and other kinds of data to other domains based on the
// targeted `transaction.Endpoint`.
package resolver

import (
	"github.com/DataDog/datadog-agent/pkg/forwarder/endpoints"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
)

// DestinationType is used to identified the expected endpoint
type DestinationType int

const (
	// Datadog enpoints
	Datadog DestinationType = iota
	// Vector endpoints
	Vector
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
}

// SingleDomainResolver will always return the same host
type SingleDomainResolver struct {
	domain  string
	apiKeys []string
}

// NewSingleDomainResolver creates a SingleDomainResolver with its destination domain & API keys
func NewSingleDomainResolver(domain string, apiKeys []string) *SingleDomainResolver {
	return &SingleDomainResolver{
		domain,
		apiKeys,
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

// GetAlternateDomains always returns an empty slice for SingleDomainResolver
func (r *SingleDomainResolver) GetAlternateDomains() []string {
	return []string{}
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
}

// NewMultiDomainResolver initializes a MultiDomainResolver with its API keys and base destination
func NewMultiDomainResolver(baseDomain string, apiKeys []string) *MultiDomainResolver {
	return &MultiDomainResolver{
		baseDomain,
		apiKeys,
		make(map[string]destination),
		[]string{},
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

// SetBaseDomain updates the base domain
func (r *MultiDomainResolver) SetBaseDomain(domain string) {
	r.baseDomain = domain
}

// GetAlternateDomains returns a slice with all alternate domain
func (r *MultiDomainResolver) GetAlternateDomains() []string {
	return r.alternateDomainList
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

// NewDomainResolverWithMetricToVector initialize a resolver with metrics diverted to a vector endpoint
func NewDomainResolverWithMetricToVector(mainEndpoint string, apiKeys []string, vectorEndpoint string) *MultiDomainResolver {
	r := NewMultiDomainResolver(mainEndpoint, apiKeys)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.V1SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SeriesEndpoint.Name, Vector)
	r.RegisterAlternateDestination(vectorEndpoint, endpoints.SketchSeriesEndpoint.Name, Vector)
	return r
}
