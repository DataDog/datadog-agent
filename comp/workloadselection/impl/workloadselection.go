// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package workloadselectionimpl implements the workloadselection component interface
package workloadselectionimpl

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	workloadselection "github.com/DataDog/datadog-agent/comp/workloadselection/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// var configPath = filepath.Join(config.DefaultConfPath, "wls-policy.bin") // TODO: is this the right path?
var configPath = "/tmp/wls-policy.json"

// Requires defines the dependencies for the workloadselection component
type Requires struct {
	Log    log.Component
	Config config.Component
}

// Provides defines the output of the workloadselection component
type Provides struct {
	Comp       workloadselection.Component
	RCListener rctypes.ListenerProvider
}

// NewComponent creates a new workloadselection component
func NewComponent(reqs Requires) (Provides, error) {
	wls := &workloadselectionComponent{
		log:    reqs.Log,
		config: reqs.Config,
	}

	var rcListener rctypes.ListenerProvider
	if reqs.Config.GetBool("apm_config.instrumentation.workload_selection") || !isCompilePolicyBinaryAvailable() {
		reqs.Log.Debug("Enabling APM SSI Workload Selection listener")
		rcListener.ListenerProvider = rctypes.RCListener{
			state.ProductApmPolicies: wls.onConfigUpdate,
		}
	} else {
		reqs.Log.Debug("Disabling APM SSI Workload Selection listener as the compile policy binary is not available or workload selection is disabled")
	}

	provides := Provides{
		Comp:       wls,
		RCListener: rcListener,
	}
	return provides, nil
}

type workloadselectionComponent struct {
	log    log.Component
	config config.Component
}

func isCompilePolicyBinaryAvailable() bool {
	compilePath := filepath.Join(config.InstallPath, "embedded", "bin", "dd-policy-compile")
	_, err := os.Stat(compilePath)
	return err == nil
}

func (c *workloadselectionComponent) onConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	c.log.Debugf("workload selection config update received: %d", len(updates))
	if len(updates) == 0 {
		err := c.removeConfig() // No config received, we have to remove the file. Nothing to acknowledge.
		if err != nil {
			c.log.Errorf("failed to remove workload selection config: %v", err)
		}
		return
	}

	if len(updates) > 1 {
		c.log.Warnf("workload selection received %d configs, expected only 1. Taking the first one alphabetically", len(updates))
	}

	// Sort paths alphabetically for consistent behavior
	var sortedPaths []string
	for path := range updates {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	// Emit error for configs that are not treated (we only accept 1)
	for i := 1; i < len(sortedPaths); i++ {
		rejectedPath := sortedPaths[i]
		applyStateCallback(rejectedPath, state.ApplyStatus{
			State: state.ApplyStateError,
			Error: "workload selection only accepts one configuration, rejecting additional configs",
		})
	}

	// Process only the first config alphabetically
	path := sortedPaths[0]
	err := c.compileAndWriteConfig(updates[path].Config)
	if err != nil {
		c.log.Errorf("failed to compile workload selection config: %v", err)
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		return
	}
	applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
}

func (c *workloadselectionComponent) compileAndWriteConfig(rawConfig []byte) error {
	c.log.Debugf("Writing workload selection config: %s", configPath)
	// TODO: call dd-policy-compile instead
	// TODO: shouldn't write in containerised environments (should the feature even run in containerised environments?)
	return os.WriteFile(configPath, rawConfig, 0644)
}

func (c *workloadselectionComponent) removeConfig() error {
	// os.RemoveAll does not fail if the path doesn't exist, it returns nil
	c.log.Debugf("Removing workload selection config: %s", configPath)
	return os.RemoveAll(configPath)
}
