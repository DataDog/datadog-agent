// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package networkconfigmanagementimpl implements the networkconfigmanagement component interface
package networkconfigmanagementimpl

import (
	"path/filepath"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
)

// Requires defines the dependencies for the networkconfigmanagement component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Logger    log.Component
}

// Provides defines the output of the networkconfigmanagement component
type Provides struct {
	Comp     networkconfigmanagement.Component
	Endpoint api.EndpointProvider `group:"agent_endpoint"`
}

type networkDeviceConfigImpl struct {
	config *ProcessedNcmConfig
	log    log.Component
	store  ncmstore.ConfigStore
}

// NewComponent creates a new networkconfigmanagement component
func NewComponent(reqs Requires) (Provides, error) {
	ncmConfig, err := newConfig(reqs.Config)
	if err != nil {
		return Provides{}, reqs.Logger.Errorf("Failed to read network device configuration: %v", err)
	}

	runPath := reqs.Config.GetString("run_path")
	dbPath := filepath.Join(runPath, "ncm_config.db")
	store, err := ncmstore.Open(dbPath)
	if err != nil {
		return Provides{}, err
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStop: store.Close})

	impl := &networkDeviceConfigImpl{
		config: ncmConfig,
		log:    reqs.Logger,
		store:  store,
	}

	provides := Provides{
		Comp:     impl,
		Endpoint: api.NewAgentEndpointProvider(newConfigEndpointHandler(store), "/ncm/config", "GET").Provider,
	}
	return provides, nil
}

func (n *networkDeviceConfigImpl) GetConfigStore() ncmstore.ConfigStore {
	return n.store
}
