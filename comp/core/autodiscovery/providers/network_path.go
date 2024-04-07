// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"errors"
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// clientID network path autodiscovery provider unique ID
	clientID = "network-path-autodiscovery-provider"
)

// NetworkPathConfigProvider implements the Config Provider interface, it should
// be called periodically and returns network_path integration instances.
type NetworkPathConfigProvider struct {
	lastCollected          time.Time
	notInitializedLogLimit *log.Limit
}

// NewNetworkPathConfigProvider instantiates a new NetworkPathConfigProvider from given config
func NewNetworkPathConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	cfp := NetworkPathConfigProvider{
		lastCollected:          time.Now(),
		notInitializedLogLimit: log.NewLogLimit(1, time.Minute*10),
	}
	return cfp, nil
}

// String returns a string representation of the NetworkPathConfigProvider
func (np NetworkPathConfigProvider) String() string {
	return names.NetworkPath
}

// IsUpToDate returns true if the last collection time was later than last BBS Cache refresh time
//
//nolint:revive // TODO(PLINT) Fix revive linter
func (np NetworkPathConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	// TODO: update me
	return true, nil
}

// Collect collects AD config templates from all relevant BBS API information
func (np NetworkPathConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	log.Debug("Collecting configs via the NetworkPathProvider")
	np.lastCollected = time.Now()
	connections, err := np.getConnections()
	if err != nil {
		return nil, err
	}
	log.Warnf("Active Connections: %+v", connections)
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
func (np NetworkPathConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}

func (np NetworkPathConfigProvider) getConnections() (*model.Connections, error) {
	tu, err := net.GetRemoteSystemProbeUtil(config.SystemProbe.GetString("system_probe_config.sysprobe_socket"))
	if err != nil {
		if np.notInitializedLogLimit.ShouldLog() {
			log.Warnf("could not initialize system-probe connection: %v (will only log every 10 minutes)", err)
		}
		return nil, errors.New("remote tracer is still not initialized")
	}
	return tu.GetConnections(clientID)
}
