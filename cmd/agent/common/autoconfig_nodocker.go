// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !docker

package common

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	log "github.com/cihub/seelog"
)

// SetupAutoConfig only starts the Autoconfig subsystem if Docker is not available
func SetupAutoConfig(confdPath string) {
	// create the Collector instance and start all the components
	// NOTICE: this will also setup the Python environment, if available
	Coll = collector.NewCollector(GetPythonPaths()...)

	// create the Autoconfig instance
	AC = autodiscovery.NewAutoConfig(Coll)

	// add the check loaders
	for _, loader := range loaders.LoaderCatalog() {
		AC.AddLoader(loader)
		log.Debugf("Added %s to AutoConfig", loader)
	}

	// Add the configuration providers

	// BUG(massi): configuration providers should not depend on the `docker` build tag.
	// For the time being, providers other than `FileConfigProvider` are only used
	// by Autodiscovery, and Autodiscovery only works with a Docker backend but
	// this will change in the future.
	// A legit use case would be using etcd to store configurations without
	// polling it because you don't use Autodiscovery and you only need a place
	// where to store configurations.

	// File Provider is hardcoded and always enabled
	confSearchPaths := []string{
		confdPath,
		filepath.Join(GetDistPath(), "conf.d"),
	}
	AC.AddProvider(providers.NewFileConfigProvider(confSearchPaths), false)
}

// StartAutoConfig only loads configs once at startup if Docker is disabled
func StartAutoConfig() {
	AC.LoadAndRun()
}
