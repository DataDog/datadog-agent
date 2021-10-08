// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package container

import "github.com/DataDog/datadog-agent/pkg/util/log"

// Catalog holds available metadata providers
type Catalog map[string]MetadataProvider

// DefaultCatalog holds every compiled-in container metadata provider
var DefaultCatalog = make(Catalog)

// RegisterMetadataProvider a container metadata provider
func RegisterMetadataProvider(name string, m MetadataProvider) {
	if _, ok := DefaultCatalog[name]; ok {
		log.Warnf("Container metadata provider %s already registered, overriding it", name)
	}
	DefaultCatalog[name] = m
}

// MetadataProvider should return a map of metadata
type MetadataProvider func() (map[string]string, error)
