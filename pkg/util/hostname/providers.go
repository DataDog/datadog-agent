// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostname

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Provider is a generic function to grab the hostname and return it
type Provider func(ctx context.Context, options map[string]interface{}) (string, error)

// providerCatalog holds all the various kinds of hostname providers
var providerCatalog = make(map[string]Provider)

// RegisterHostnameProvider registers a hostname provider as part of the catalog
func RegisterHostnameProvider(name string, p Provider) {
	providerCatalog[name] = p
}

// GetProvider returns a Provider if it was register before.
func GetProvider(providerName string) Provider {
	if provider, found := providerCatalog[providerName]; found {
		return provider
	}
	return nil
}

// GetHostname returns the hostname for a specific Provider if it was register
func GetHostname(ctx context.Context, providerName string, options map[string]interface{}) (string, error) {
	if provider, found := providerCatalog[providerName]; found {
		log.Debugf("GetHostname trying provider '%s' ...", providerName)
		name, err := provider(ctx, options)
		if err != nil {
			return "", err
		}
		if validate.ValidHostname(name) != nil {
			return "", fmt.Errorf("Invalid hostname '%s' from %s provider", name, providerName)
		}
		return name, nil
	}
	return "", fmt.Errorf("hostname provider %s not found", providerName)
}
