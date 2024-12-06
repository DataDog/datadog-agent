// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type remoteConfigClient interface {
	Start()
	Close()
	Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
	GetInstallerState() *pbgo.ClientUpdater
	SetInstallerState(state *pbgo.ClientUpdater)
}

type remoteConfig struct {
	client remoteConfigClient
}

func newRemoteConfig(rcFetcher client.ConfigFetcher) (*remoteConfig, error) {
	client, err := client.NewClient(
		rcFetcher,
		client.WithUpdater(),
		client.WithProducts(state.ProductUpdaterCatalogDD),
		client.WithoutTufVerification(),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create rc client: %w", err)
	}
	return &remoteConfig{client: client}, nil
}

// Start starts the remote config client.
func (rc *remoteConfig) Start(handleCatalogUpdate handleCatalogUpdate, handleRemoteAPIRequest handleRemoteAPIRequest) {
	if rc.client == nil {
		return
	}
	subscribeToTask := func() {
		// only subscribe to tasks once the first catalog has been applied
		// subscribe in a goroutine to avoid deadlocking the client
		go rc.client.Subscribe(state.ProductUpdaterTask, handleUpdaterTaskUpdate(handleRemoteAPIRequest))
	}
	rc.client.Subscribe(state.ProductUpdaterCatalogDD, handleUpdaterCatalogDDUpdate(handleCatalogUpdate, subscribeToTask))
	rc.client.Start()
}

// Close closes the remote config client.
func (rc *remoteConfig) Close() {
	rc.client.Close()
}

// GetState gets the state of the remote config client.
func (rc *remoteConfig) GetState() *pbgo.ClientUpdater {
	return rc.client.GetInstallerState()
}

// SetState sets the state of the remote config client.
func (rc *remoteConfig) SetState(state *pbgo.ClientUpdater) {
	rc.client.SetInstallerState(state)
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

type handleCatalogUpdate func(catalog catalog) error

func handleUpdaterCatalogDDUpdate(h handleCatalogUpdate, firstCatalogApplied func()) func(map[string]state.RawConfig, func(cfgPath string, status state.ApplyStatus)) {
	var catalogOnce sync.Once
	return func(catalogConfigs map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
		var mergedCatalog catalog
		for configPath, config := range catalogConfigs {
			var catalog catalog
			err := json.Unmarshal(config.Config, &catalog)
			if err != nil {
				log.Errorf("could not unmarshal installer catalog: %s", err)
				applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				return
			}
			for _, p := range catalog.Packages {
				err := validatePackage(p)
				if err != nil {
					log.Errorf("invalid package in catalog: %s", err)
					applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
					return
				}
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
		catalogOnce.Do(firstCatalogApplied)
		for configPath := range catalogConfigs {
			applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
}

func validatePackage(pkg Package) error {
	if pkg.Name == "" {
		return fmt.Errorf("package name is empty")
	}
	if pkg.Version == "" {
		return fmt.Errorf("package version is empty")
	}
	if pkg.URL == "" {
		return fmt.Errorf("package URL is empty")
	}
	url, err := url.Parse(pkg.URL)
	if err != nil {
		return fmt.Errorf("could not parse package URL: %w", err)
	}
	if url.Scheme == "oci" {
		ociURL := strings.TrimPrefix(pkg.URL, "oci://")
		// Check if the URL is a valid *digest* URL.
		// We do not allow referencing images by tag when sent over RC.
		_, err := name.NewDigest(ociURL)
		if err != nil {
			return fmt.Errorf("could not parse oci digest URL: %w", err)
		}
	}
	return nil
}

const (
	methodStartExperiment   = "start_experiment"
	methodStopExperiment    = "stop_experiment"
	methodPromoteExperiment = "promote_experiment"

	methodStartConfigExperiment   = "start_experiment_config"
	methodStopConfigExperiment    = "stop_experiment_config"
	methodPromoteConfigExperiment = "promote_experiment_config"
)

type remoteAPIRequest struct {
	ID            string          `json:"id"`
	Package       string          `json:"package_name"`
	TraceID       string          `json:"trace_id"`
	ParentSpanID  string          `json:"parent_span_id"`
	ExpectedState expectedState   `json:"expected_state"`
	Method        string          `json:"method"`
	Params        json.RawMessage `json:"params"`
}

type expectedState struct {
	InstallerVersion string `json:"installer_version"`
	Stable           string `json:"stable"`
	Experiment       string `json:"experiment"`
	StableConfig     string `json:"stable_config"`
	ExperimentConfig string `json:"experiment_config"`
}

type taskWithVersionParams struct {
	Version     string   `json:"version"`
	InstallArgs []string `json:"install_args"`
}

type handleRemoteAPIRequest func(request remoteAPIRequest) error

func handleUpdaterTaskUpdate(h handleRemoteAPIRequest) func(map[string]state.RawConfig, func(cfgPath string, status state.ApplyStatus)) {
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
