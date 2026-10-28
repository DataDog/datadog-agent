// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package smb

import "github.com/DataDog/datadog-agent/pkg/logs/sources"

// configEntry identifies the configuration entry a source was created from.
// Both launchers use it to find the source a newer one replaces.
type configEntry struct {
	name       string
	file       string // the integration config's source, e.g. file:/etc/datadog-agent/conf.d/app.d/conf.yaml
	index      int    // the entry's index in the config's logs list
	identifier string // the service, for autodiscovered configs
}

// configEntryOf returns the configuration entry of source, if it comes from an
// integration config.
func configEntryOf(source *sources.LogSource) (configEntry, bool) {
	cfg := source.Config
	if cfg.IntegrationSource == "" {
		return configEntry{}, false
	}
	return configEntry{name: source.Name, file: cfg.IntegrationSource, index: cfg.IntegrationSourceIndex, identifier: cfg.Identifier}, true
}
