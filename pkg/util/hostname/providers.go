// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package hostname

// Provider is a generic function to grab the hostname and return it
type Provider func() (string, error)

// ProviderCatalog holds all the various kinds of hostname providers
var ProviderCatalog = make(map[string]Provider)

// RegisterHostnameProvider registers a hostname provider as part of the catalog
func RegisterHostnameProvider(name string, p Provider) {
	ProviderCatalog[name] = p
}
