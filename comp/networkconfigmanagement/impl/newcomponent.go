// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package networkconfigmanagementimpl implements the networkconfigmanagement component interface
package networkconfigmanagementimpl

import (
	"context"
	"path/filepath"

	"github.com/benbjohnson/clock"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/comp/networkconfigmanagement/stub"
	ncmprofile "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
)

// Requires defines the dependencies for the networkconfigmanagement component
type Requires struct {
	compdef.In
	Lifecycle       compdef.Lifecycle
	Config          config.Component
	Logger          log.Component
	Demux           demultiplexer.Component
	HostnameService hostname.Component
}

// NewComponent creates a new networkconfigmanagement component
func NewComponent(reqs Requires) (Provides, error) {
	var comp networkconfigmanagement.Component
	compImpl, err := newComponent(reqs)
	if err != nil {
		reqs.Logger.Errorf("NCM service could not be initialized: %s", err)
		comp = stub.NewStub("network config management could not be initialized")
	} else {
		comp = compImpl
	}
	return NewProvides(comp), nil

}

func newComponent(reqs Requires) (*networkDeviceConfigImpl, error) {
	rollbackEnabled := reqs.Config.GetBool("network_devices.config_management.rollback.enabled")
	hostname, err := reqs.HostnameService.Get(context.Background())
	if err != nil {
		return nil, err
	}
	sender, err := reqs.Demux.GetSender(CheckName)
	if err != nil {
		return nil, err
	}
	profiles, err := ncmprofile.GetProfileMap()
	if err != nil {
		return nil, err
	}
	var store ncmstore.ConfigStore
	if rollbackEnabled {
		runPath := reqs.Config.GetString("run_path")
		dbPath := filepath.Join(runPath, "ncm_config.db")
		store, err = ncmstore.Open(dbPath)
		if err != nil {
			store = nil
			reqs.Logger.Errorf("ncm: rollback is enabled but storage db %v could not be opened: %v", dbPath, err)
			reqs.Logger.Errorf("ncm: running in no-rollback mode - configs will be not saved locally for rollback")
		} else {
			reqs.Logger.Debugf("ncm: config rollback enabled; local db is %v", dbPath)
			reqs.Lifecycle.Append(compdef.Hook{OnStop: store.Close})
		}
	}

	impl := newNetworkDeviceConfigImpl(
		reqs.Logger,
		store,
		sender,
		hostname,
		profiles,
		ncmremote.ConnectOverSSH,
		clock.New(),
	)
	return impl, nil
}
