// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RemoteConfigProvider receives configuration from remote-config
type RemoteConfigProvider struct {
	configErrors map[string]types.ErrorMsgSet
	configCache  map[string]integration.Config // map[entity name]map[config digest]integration.Config
	mu           sync.RWMutex
	upToDate     bool
}

type rcAgentIntegration struct {
	Name       string            `json:"name"`
	Instances  []json.RawMessage `json:"instances"`
	InitConfig json.RawMessage   `json:"init_config"`
	LogsConfig json.RawMessage   `json:"logs"`
}

var datadogConfigIDRegexp = regexp.MustCompile(`^datadog/\d+/AGENT_INTEGRATIONS/([^/]+)/([^/]+)$`)

// NewRemoteConfigProvider creates a new RemoteConfigProvider.
func NewRemoteConfigProvider() *RemoteConfigProvider {
	return &RemoteConfigProvider{
		configErrors: make(map[string]types.ErrorMsgSet),
		configCache:  make(map[string]integration.Config),
		upToDate:     false,
	}
}

// Collect retrieves integrations from the remote-config, builds Config objects and returns them
func (rc *RemoteConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	rc.upToDate = true

	// TODO: use the `Stream` interface instead of the `Collect`+`isUpToDate` interface
	// for the next implementation iteration
	integrationList := []integration.Config{}
	for _, intg := range rc.configCache {
		integrationList = append(integrationList, intg)
	}

	return integrationList, nil
}

// IsUpToDate allows to cache configs as long as no changes are detected in remote-config
func (rc *RemoteConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.upToDate, nil
}

// String returns a string representation of the RemoteConfigProvider
func (rc *RemoteConfigProvider) String() string {
	return names.RemoteConfig
}

// GetConfigErrors returns a map of configuration errors for each configuration path
func (rc *RemoteConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	errors := make(map[string]types.ErrorMsgSet, len(rc.configErrors))

	maps.Copy(errors, rc.configErrors)

	return errors
}

// IntegrationScheduleCallback is called at every AGENT_INTEGRATIONS to schedule/unschedule integrations
func (rc *RemoteConfigProvider) IntegrationScheduleCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var err error

	allowedIntegration := pkgconfigsetup.GetRemoteConfigurationAllowedIntegrations(pkgconfigsetup.Datadog())

	newCache := make(map[string]integration.Config, 0)
	// Now schedule everything
	for cfgPath, intg := range updates {
		var d rcAgentIntegration
		err = json.Unmarshal(intg.Config, &d)
		if err != nil {
			log.Errorf("Can't decode agent configuration provided by remote-config: %v", err)
			rc.configErrors[cfgPath] = types.ErrorMsgSet{
				err.Error(): struct{}{},
			}
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			break
		}

		if !allowedIntegration[strings.ToLower(d.Name)] {
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: fmt.Sprintf("Integration %s is not allowed to be scheduled in this agent", d.Name),
			})
			continue
		}

		applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateUnacknowledged})

		source := cfgPath
		matched := datadogConfigIDRegexp.FindStringSubmatch(cfgPath)
		if len(matched) == 3 {
			// Source is configID/configName
			source = fmt.Sprintf("%s/%s", matched[1], matched[2])
		}
		// The ENC[] configuration resolution is done by configmgr
		newConfig := integration.Config{
			Name:       d.Name,
			Instances:  []integration.Data{},
			InitConfig: integration.Data(d.InitConfig),
			LogsConfig: integration.Data(d.LogsConfig),
			Source:     source,
		}
		for _, inst := range d.Instances {
			newConfig.Instances = append(newConfig.Instances, integration.Data(inst))
		}
		newCache[cfgPath] = newConfig

		// TODO: report errors in a sync way to get integration run errors
		applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	if err == nil {
		// Schedule new integrations set only if there was no error
		rc.configCache = newCache
		rc.upToDate = false
	}
}
