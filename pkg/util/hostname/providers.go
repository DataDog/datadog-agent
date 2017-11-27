// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package hostname

// ProviderMethod is a generic function to grab the hostname and return it
type ProviderMethod func(string) (string, error)

// ProviderPriority allows to specify a lookup priority for a provider
type ProviderPriority uint

// ProviderPriority values, in order of preference
const (
	Hosting ProviderPriority = iota
	Docker
	Lowest // Must stay last of the list, of course
)

// Provider holds a provider's name and method
type Provider struct {
	Name   string
	Method ProviderMethod
}

// ProviderCatalog holds all the various hostname providers compiled in
var ProviderCatalog = make([][]Provider, int(Lowest)+1)

// RegisterHostnameProvider registers a hostname provider as part of the catalog
func RegisterHostnameProvider(priority ProviderPriority, name string, p ProviderMethod) {
	prov := Provider{
		Name:   name,
		Method: p,
	}
	ProviderCatalog[priority] = append(ProviderCatalog[priority], prov)
}
