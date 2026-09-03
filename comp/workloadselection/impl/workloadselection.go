// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package workloadselectionimpl implements the workloadselection component interface
package workloadselectionimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	workloadselection "github.com/DataDog/datadog-agent/comp/workloadselection/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// targetedPolicyIDPrefix identifies host-targeted policies within a policy ID.
// Policy IDs of the form "targeted.<name>" are merged into configPathTargeted;
// all other policy IDs are treated as org-wide and merged into configPath.
const targetedPolicyIDPrefix = "targeted."

var (
	configPath         = filepath.Join(config.DefaultConfPath, "managed", "rc-orgwide-wls-policy.bin")
	configPathTargeted = filepath.Join(config.DefaultConfPath, "managed", "rc-targeted-wls-policy.bin")
	// Pattern to extract policy ID from config path: datadog/\d+/<product>/<config_id>/<hash>
	policyIDPattern = regexp.MustCompile(`^datadog/\d+/[^/]+/([^/]+)/`)
	// Pattern to extract numeric prefix from policy ID: N.<name>
	policyPrefixPattern = regexp.MustCompile(`^(\d+)\.`)

	// getInstallPath is a variable that can be overridden in tests
	getInstallPath = config.GetInstallPath
)

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
	if reqs.Config.GetBool("apm_config.workload_selection") && wls.isCompilePolicyBinaryAvailable() {
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

// policyConfig represents a config with its ordering information
type policyConfig struct {
	path   string
	order  int
	config []byte
}

// extractPolicyID extracts the policy ID from a config path
// Path format: configs/\d+/<ID>/<gibberish>
func extractPolicyID(path string) string {
	matches := policyIDPattern.FindStringSubmatch(path)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractOrderFromPolicyID extracts the numeric order from a policy ID
// If policy ID is in format N.<name>, returns N. Otherwise returns 0.
func extractOrderFromPolicyID(policyID string) int {
	matches := policyPrefixPattern.FindStringSubmatch(policyID)
	if len(matches) > 1 {
		if order, err := strconv.Atoi(matches[1]); err == nil {
			return order
		}
	}
	return 0
}

// isTargetedPolicyID reports whether a policy ID identifies a host-targeted policy,
// as opposed to an org-wide policy.
func isTargetedPolicyID(policyID string) bool {
	return strings.HasPrefix(policyID, targetedPolicyIDPrefix)
}

// mergeConfigs merges multiple configs by concatenating their policies in order
func mergeConfigs(configs []policyConfig) ([]byte, error) {
	type policyJSON struct {
		Policies []json.RawMessage `json:"policies"`
	}

	allPolicies := make([]json.RawMessage, 0)

	for _, cfg := range configs {
		var parsed policyJSON
		if err := json.Unmarshal(cfg.config, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse config from %s: %w", cfg.path, err)
		}
		allPolicies = append(allPolicies, parsed.Policies...)
	}

	merged := policyJSON{Policies: allPolicies}
	return json.Marshal(merged)
}

// onConfigUpdate is the callback function called by Remote Config when the workload selection config is updated.
// Configs are split into org-wide and host-targeted groups, merged and compiled
// independently, and written to separate output files so that a host's targeted policies never affect
// another host's org-wide policies and vice versa.
func (c *workloadselectionComponent) onConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	c.log.Debugf("workload selection config update received: %d", len(updates))
	if len(updates) == 0 {
		// No config received at all, we have to remove both files. Nothing to acknowledge.
		c.removeConfigs()
		return
	}

	// Build lists of configs with their ordering information, split by scope
	var orgWideConfigs, targetedConfigs []policyConfig
	for path, rawConfig := range updates {
		policyID := extractPolicyID(path)
		order := extractOrderFromPolicyID(policyID)

		c.log.Debugf("Processing config path=%s policyID=%s order=%d", path, policyID, order)

		cfg := policyConfig{
			path:   path,
			order:  order,
			config: rawConfig.Config,
		}

		if isTargetedPolicyID(policyID) {
			targetedConfigs = append(targetedConfigs, cfg)
		} else {
			orgWideConfigs = append(orgWideConfigs, cfg)
		}
	}

	c.processConfigGroup(orgWideConfigs, configPath, "org-wide", applyStateCallback)
	c.processConfigGroup(targetedConfigs, configPathTargeted, "targeted", applyStateCallback)
}

// processConfigGroup sorts, merges, compiles, and writes a group of policy configs to outputPath,
// invoking applyStateCallback for each config's path. If configs is empty, the file previously
// written at outputPath (if any) is removed instead, and applyStateCallback is not invoked.
func (c *workloadselectionComponent) processConfigGroup(configs []policyConfig, outputPath, groupName string, applyStateCallback func(string, state.ApplyStatus)) {
	if len(configs) == 0 {
		if err := c.removeConfig(outputPath); err != nil {
			c.log.Errorf("failed to remove %s workload selection config: %v", groupName, err)
		}
		return
	}

	// Sort configs by order, then alphabetically by path for deterministic ordering
	sort.SliceStable(configs, func(i, j int) bool {
		if configs[i].order != configs[j].order {
			return configs[i].order < configs[j].order
		}
		// Secondary sort by path for deterministic ordering when order values are equal
		return configs[i].path < configs[j].path
	})

	// Track error state and apply callbacks on function exit
	var processingErr error
	defer func() {
		for _, cfg := range configs {
			if processingErr != nil {
				applyStateCallback(cfg.path, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: processingErr.Error(),
				})
			} else {
				applyStateCallback(cfg.path, state.ApplyStatus{
					State: state.ApplyStateAcknowledged,
				})
			}
		}
	}()

	// Log the ordering for debugging
	var orderInfo []string
	for _, cfg := range configs {
		policyID := extractPolicyID(cfg.path)
		orderInfo = append(orderInfo, fmt.Sprintf("%s (order=%d)", policyID, cfg.order))
	}
	c.log.Debugf("Merging %d %s workload selection configs in order: %s", len(configs), groupName, strings.Join(orderInfo, ", "))

	// Merge all configs into one
	mergedConfig, err := mergeConfigs(configs)
	if err != nil {
		c.log.Errorf("failed to merge %s workload selection configs: %v", groupName, err)
		processingErr = err
		return
	}

	// Compile and write the merged config
	if err := c.compileAndWriteConfig(mergedConfig, outputPath); err != nil {
		c.log.Errorf("failed to compile %s workload selection config: %v", groupName, err)
		processingErr = err
		return
	}
}

// removeConfigs removes both the org-wide and targeted workload selection config files.
func (c *workloadselectionComponent) removeConfigs() {
	if err := c.removeConfig(configPath); err != nil {
		c.log.Errorf("failed to remove org-wide workload selection config: %v", err)
	}
	if err := c.removeConfig(configPathTargeted); err != nil {
		c.log.Errorf("failed to remove targeted workload selection config: %v", err)
	}
}

func (c *workloadselectionComponent) removeConfig(path string) error {
	// os.RemoveAll does not fail if the path doesn't exist, it returns nil
	c.log.Debugf("Removing workload selection config at %s", path)
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to remove workload selection binary policy at %s: %w", path, err)
	}
	return nil
}
