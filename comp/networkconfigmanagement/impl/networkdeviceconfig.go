// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkconfigmanagementimpl implements the networkconfigmanagement component interface
package networkconfigmanagementimpl

import (
	"path/filepath"

	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Provides defines the output of the networkconfigmanagement component
type Provides struct {
	compdef.Out

	Comp              option.Option[networkconfigmanagement.Component]
	GetConfigEndpoint api.EndpointProvider `group:"agent_endpoint"`
}

// Requires defines the dependencies for the networkconfigmanagement component
type Requires struct {
	compdef.In
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Logger    log.Component
}

type networkDeviceConfigImpl struct {
	log   log.Component
	store ncmstore.ConfigStore
}

// NewComponent creates a new networkconfigmanagement component
func NewComponent(reqs Requires) (Provides, error) {
	enabled := reqs.Config.GetBool("network_config_management.rollback.enabled")
	if !enabled {
		return NewNoopComponent()
	}
	comp, err := newComponent(reqs)
	if err != nil {
		reqs.Logger.Errorf("NCM config store service could not be initialized: %s", err)
		return NewNoopComponent()
	}
	return Provides{
		Comp:              option.New(comp),
		GetConfigEndpoint: api.NewAgentEndpointProvider(newConfigEndpointHandler(comp.GetConfigStore()), "/ncm/config", "GET").Provider,
	}, nil

}

func newComponent(reqs Requires) (networkconfigmanagement.Component, error) {
	runPath := reqs.Config.GetString("run_path")
	dbPath := filepath.Join(runPath, "ncm_config.db")
	store, err := ncmstore.Open(dbPath)
	if err != nil {
		return nil, err
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStop: store.Close})

	impl := &networkDeviceConfigImpl{
		log:   reqs.Logger,
		store: store,
	}
	return impl, nil
}

func (n *networkDeviceConfigImpl) GetConfigStore() ncmstore.ConfigStore {
	return n.store
}

// NewComponent creates a stub networkconfigmanagement component
func NewNoopComponent() (Provides, error) {
	endpoint := api.NewAgentEndpointProvider(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error": "ncm not enabled for agent"}`, http.StatusBadRequest)
	}, "/ncm/config", "GET")
	provides := Provides{
		Comp:              option.None[networkconfigmanagement.Component](),
		GetConfigEndpoint: endpoint.Provider,
	}
	return provides, nil
}
