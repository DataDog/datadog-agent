// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package providers

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
)

// NetworkPathProvider receives configuration from network-path
type NetworkPathProvider struct {
	configErrors map[string]ErrorMsgSet
	mu           sync.RWMutex
	upToDate     bool
}

type NetworkPathIntegration struct {
	Name       string            `json:"name"`
	Instances  []json.RawMessage `json:"instances"`
	InitConfig json.RawMessage   `json:"init_config"`
	LogsConfig json.RawMessage   `json:"logs"`
}

// NewNetworkPathProvider creates a new NetworkPathProvider.
func NewNetworkPathProvider() *NetworkPathProvider {
	return &NetworkPathProvider{
		configErrors: make(map[string]ErrorMsgSet),
		upToDate:     false,
	}
}

// Collect retrieves integrations from the network-path, builds Config objects and returns them
func (rc *NetworkPathProvider) Collect(ctx context.Context) ([]integration.Config, error) { //nolint:revive // TODO fix revive unused-parameter
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	rc.upToDate = true

	// TODO: use the `Stream` interface instead of the `Collect`+`isUpToDate` interface
	// for the next implementation iteration
	integrationList := []integration.Config{
		{
			Name:       "network_path",
			InitConfig: integration.Data("{}"),
			Instances:  []integration.Data{integration.Data(`{"hostname":"bing.com"}`)},
		},
	}
	//for _, intg := range rc.configCache {
	//	integrationList = append(integrationList, intg)
	//}

	return integrationList, nil
}

// IsUpToDate allows to cache configs as long as no changes are detected in network-path
func (rc *NetworkPathProvider) IsUpToDate(ctx context.Context) (bool, error) { //nolint:revive // TODO fix revive unused-parameter
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.upToDate, nil
}

// String returns a string representation of the NetworkPathProvider
func (rc *NetworkPathProvider) String() string {
	return names.NetworkPath
}

// GetConfigErrors returns a map of configuration errors for each configuration path
func (rc *NetworkPathProvider) GetConfigErrors() map[string]ErrorMsgSet {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	errors := make(map[string]ErrorMsgSet, len(rc.configErrors))

	for entity, errset := range rc.configErrors {
		errors[entity] = errset
	}

	return errors
}

//// IntegrationScheduleCallback is called at every AGENT_INTEGRATIONS to schedule/unschedule integrations
//func (rc *NetworkPathProvider) IntegrationScheduleCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
//	rc.mu.Lock()
//	defer rc.mu.Unlock()
//	var err error
//
//	allowedIntegration := config.GetNetworkPathurationAllowedIntegrations(config.Datadog)
//
//	newCache := make(map[string]integration.Config, 0)
//	// Now schedule everything
//	for cfgPath, intg := range updates {
//		var d NetworkPathIntegration
//		err = json.Unmarshal(intg.Config, &d)
//		if err != nil {
//			log.Errorf("Can't decode agent configuration provided by network-path: %v", err)
//			rc.configErrors[cfgPath] = ErrorMsgSet{
//				err.Error(): struct{}{},
//			}
//			applyStateCallback(cfgPath, state.ApplyStatus{
//				State: state.ApplyStateError,
//				Error: err.Error(),
//			})
//			break
//		}
//
//		if !allowedIntegration[strings.ToLower(d.Name)] {
//			applyStateCallback(cfgPath, state.ApplyStatus{
//				State: state.ApplyStateError,
//				Error: fmt.Sprintf("Integration %s is not allowed to be scheduled in this agent", d.Name),
//			})
//			continue
//		}
//
//		applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateUnacknowledged})
//
//		source := cfgPath
//		matched := datadogConfigIDRegexp.FindStringSubmatch(cfgPath)
//		if len(matched) == 3 {
//			// Source is configID/configName
//			source = fmt.Sprintf("%s/%s", matched[1], matched[2])
//		}
//		// The ENC[] configuration resolution is done by configmgr
//		newConfig := integration.Config{
//			Name:       d.Name,
//			Instances:  []integration.Data{},
//			InitConfig: integration.Data(d.InitConfig),
//			LogsConfig: integration.Data(d.LogsConfig),
//			Source:     source,
//		}
//		for _, inst := range d.Instances {
//			newConfig.Instances = append(newConfig.Instances, integration.Data(inst))
//		}
//		newCache[cfgPath] = newConfig
//
//		// TODO: report errors in a sync way to get integration run errors
//		applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
//	}
//	if err == nil {
//		// Schedule new integrations set only if there was no error
//		rc.configCache = newCache
//		rc.upToDate = false
//	}
//}
