// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type remoteConfigClient interface {
	Start()
	Close()
	Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
	SetUpdaterPackagesState(packages []*pbgo.PackageState)
}

type remoteConfig struct {
	client remoteConfigClient
}

func newRemoteConfig(rcFetcher client.ConfigFetcher) (*remoteConfig, error) {
	client, err := client.NewClient(
		rcFetcher,
		client.WithUpdater(),
		client.WithProducts(state.ProductUpdaterCatalogDD, state.ProductUpdaterTask),
		client.WithoutTufVerification(),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create rc client: %w", err)
	}
	return &remoteConfig{client: client}, nil
}

func newNoopRemoteConfig() *remoteConfig {
	return &remoteConfig{}
}

// Start starts the remote config client.
func (rc *remoteConfig) Start(handleCatalogUpdate handleCatalogUpdate, handleRemoteAPIRequest handleRemoteAPIRequest) {
	if rc.client == nil {
		return
	}
	rc.client.Subscribe(state.ProductUpdaterCatalogDD, handleUpdaterCatalogDDUpdate(handleCatalogUpdate))
	rc.client.Subscribe(state.ProductUpdaterTask, handleUpdaterTaskUpdate(handleRemoteAPIRequest))
	rc.client.Start()
}

// Close closes the remote config client.
func (rc *remoteConfig) Close() {
	if rc.client == nil {
		return
	}
	rc.client.Close()
}

// SetState sets the state of the given package.
func (rc *remoteConfig) SetState(packages []*pbgo.PackageState) {
	if rc.client == nil {
		return
	}
	rc.client.SetUpdaterPackagesState(packages)
}

// Package represents a downloadable package.
type Package struct {
	Name     string `json:"package"`
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

type catalog struct {
	Packages []Package `json:"packages"`
}

func (c *catalog) getPackage(pkg string, version string, arch string, platform string) (Package, bool) {
	for _, p := range c.Packages {
		if p.Name == pkg && p.Version == version && (p.Arch == "" || p.Arch == arch) && (p.Platform == "" || p.Platform == platform) {
			return p, true
		}
	}
	return Package{}, false
}

func (c *catalog) getDefaultPackage(defaults bootstrapVersions, pkg string, arch string, platform string) (Package, bool) {
	version, ok := defaults[pkg]
	if !ok {
		return Package{}, false
	}
	return c.getPackage(pkg, version, arch, platform)
}

type bootstrapVersions map[string]string

type handleCatalogUpdate func(catalog catalog) error

func handleUpdaterCatalogDDUpdate(h handleCatalogUpdate) client.Handler {
	return func(catalogConfigs map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		var mergedCatalog catalog
		for configPath, config := range catalogConfigs {
			var catalog catalog
			err := json.Unmarshal(config.Config, &catalog)
			if err != nil {
				log.Errorf("could not unmarshal updater catalog: %s", err)
				applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				return
			}
			mergedCatalog.Packages = append(mergedCatalog.Packages, catalog.Packages...)
		}
		err := h(mergedCatalog)
		if err != nil {
			log.Errorf("could not update catalog: %s", err)
			for configPath := range catalogConfigs {
				applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			}
			return
		}
		for configPath := range catalogConfigs {
			applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
}

const (
	methodStartExperiment   = "start_experiment"
	methodStopExperiment    = "stop_experiment"
	methodPromoteExperiment = "promote_experiment"
	methodBootstrap         = "bootstrap"
)

type remoteAPIRequest struct {
	ID            string          `json:"id"`
	Package       string          `json:"package_name"`
	ExpectedState expectedState   `json:"expected_state"`
	Method        string          `json:"method"`
	Params        json.RawMessage `json:"params"`
}

type expectedState struct {
	Stable     string `json:"stable"`
	Experiment string `json:"experiment"`
}

type taskWithVersionParams struct {
	Version string `json:"version"`
}

type handleRemoteAPIRequest func(request remoteAPIRequest) error

func handleUpdaterTaskUpdate(h handleRemoteAPIRequest) client.Handler {
	var executedRequests = make(map[string]struct{})
	return func(requestConfigs map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		requests := map[string]remoteAPIRequest{}
		for id, requestConfig := range requestConfigs {
			var request remoteAPIRequest
			err := json.Unmarshal(requestConfig.Config, &request)
			if err != nil {
				log.Errorf("could not unmarshal request: %s", err)
				applyStateCallback(id, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				return
			}
			requests[id] = request
		}
		for configID, request := range requests {
			if _, ok := executedRequests[request.ID]; ok {
				log.Debugf("request %s already executed", request.ID)
				continue
			}
			executedRequests[request.ID] = struct{}{}
			err := h(request)
			if err != nil {
				log.Errorf("could not execute request: %s", err)
				applyStateCallback(configID, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				return
			}
			applyStateCallback(configID, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
}
