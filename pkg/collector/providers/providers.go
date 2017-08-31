// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

// ProviderCatalog keeps track of config providers by name
var ProviderCatalog = make(map[string]struct{})

// RegisterProvider adds a loader to the providers catalog
func RegisterProvider(name string) {
	ProviderCatalog[name] = struct{}{}
}
