// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NetworkPathConfigProvider implements the Config Provider interface, it should
// be called periodically and returns templates from Cloud Foundry BBS for AutoConf.
type NetworkPathConfigProvider struct {
	lastCollected time.Time
}

// NewNetworkPathConfigProvider instantiates a new NetworkPathConfigProvider from given config
func NewNetworkPathConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	cfp := NetworkPathConfigProvider{
		lastCollected: time.Now(),
	}
	return cfp, nil
}

// String returns a string representation of the NetworkPathConfigProvider
func (cf NetworkPathConfigProvider) String() string {
	return names.NetworkPath
}

// IsUpToDate returns true if the last collection time was later than last BBS Cache refresh time
//
//nolint:revive // TODO(PLINT) Fix revive linter
func (cf NetworkPathConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	// TODO: update me
	return true, nil
}

// Collect collects AD config templates from all relevant BBS API information
func (cf NetworkPathConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	log.Debug("Collecting configs via the NetworkPathProvider")
	cf.lastCollected = time.Now()
	allConfigs := []integration.Config{
		{
			Name:       "network_path",
			InitConfig: integration.Data("{}"),
			Instances:  []integration.Data{integration.Data(`{"hostname":"bing.com"}`)},
		},
	}
	return allConfigs, nil
}

// GetConfigErrors is not implemented for the NetworkPathConfigProvider
func (cf NetworkPathConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
