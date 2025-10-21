// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package catalog

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl/parse"
)

// FilterConfig holds all configuration values needed for filter initialization
type FilterConfig struct {
	// Legacy container filters
	ContainerInclude        []string
	ContainerExclude        []string
	ContainerIncludeMetrics []string
	ContainerExcludeMetrics []string
	ContainerIncludeLogs    []string
	ContainerExcludeLogs    []string

	// Legacy AC filters
	ACInclude []string
	ACExclude []string

	// Pause container settings
	ExcludePauseContainer     bool
	SBOMExcludePauseContainer bool

	// SBOM container filtering
	SBOMContainerInclude []string
	SBOMContainerExclude []string

	// Process filtering settings
	ProcessBlacklistPatterns []string

	// CEL workload filter rules (pre-parsed)
	CELProductRules map[workloadfilter.Product]map[workloadfilter.ResourceType][]string
}

// NewFilterConfig creates a FilterConfig from the agent config
func NewFilterConfig(cfg config.Component) (*FilterConfig, error) {
	rawCfg, loadErr := loadCELConfig(cfg)
	if loadErr != nil {
		return nil, loadErr
	}

	celProductRules, celParseErrors := parse.GetProductConfigs(rawCfg)
	if celParseErrors != nil {
		return nil, errors.Join(celParseErrors...)
	}

	var processBlacklistPatterns []string
	if cfg.IsConfigured("process_config.blacklist_patterns") {
		processBlacklistPatterns = cfg.GetStringSlice("process_config.blacklist_patterns")
	}

	return &FilterConfig{
		// Legacy container filters
		ContainerInclude:        cfg.GetStringSlice("container_include"),
		ContainerExclude:        cfg.GetStringSlice("container_exclude"),
		ContainerIncludeMetrics: cfg.GetStringSlice("container_include_metrics"),
		ContainerExcludeMetrics: cfg.GetStringSlice("container_exclude_metrics"),
		ContainerIncludeLogs:    cfg.GetStringSlice("container_include_logs"),
		ContainerExcludeLogs:    cfg.GetStringSlice("container_exclude_logs"),

		// Legacy AC filters
		ACInclude: cfg.GetStringSlice("ac_include"),
		ACExclude: cfg.GetStringSlice("ac_exclude"),

		// Pause container settings
		ExcludePauseContainer:     cfg.GetBool("exclude_pause_container"),
		SBOMExcludePauseContainer: cfg.GetBool("sbom.container_image.exclude_pause_container"),

		// SBOM container filtering
		SBOMContainerInclude: cfg.GetStringSlice("sbom.container_image.container_include"),
		SBOMContainerExclude: cfg.GetStringSlice("sbom.container_image.container_exclude"),

		// Process filtering settings
		ProcessBlacklistPatterns: processBlacklistPatterns,

		// CEL workload filter rules (pre-parsed)
		CELProductRules: celProductRules,
	}, nil
}

// GetCELRulesForProduct returns the CEL rules for a specific product and resource type
func (fc *FilterConfig) GetCELRulesForProduct(product workloadfilter.Product, resourceType workloadfilter.ResourceType) string {
	if fc.CELProductRules == nil {
		return ""
	}

	if productMap, exists := fc.CELProductRules[product]; exists {
		if ruleSlice, exists := productMap[resourceType]; exists {
			return strings.Join(ruleSlice, " || ")
		}
	}

	return ""
}

// GetLegacyContainerInclude returns the appropriate container include list with fallback to AC include
func (fc *FilterConfig) GetLegacyContainerInclude() []string {
	if len(fc.ContainerInclude) > 0 {
		return fc.ContainerInclude
	}
	return fc.ACInclude
}

// GetLegacyContainerExclude returns the appropriate container exclude list with fallback to AC exclude
func (fc *FilterConfig) GetLegacyContainerExclude() []string {
	if len(fc.ContainerExclude) > 0 {
		return fc.ContainerExclude
	}
	return fc.ACExclude
}

// loadCELConfig loads CEL workload exclude configuration
func loadCELConfig(cfg config.Component) ([]workloadfilter.RuleBundle, error) {
	var celConfig []workloadfilter.RuleBundle

	// First try the standard UnmarshalKey method (input defined in datadog.yaml)
	err := cfg.UnmarshalKey("cel_workload_exclude", &celConfig)
	if err == nil {
		return celConfig, nil
	}

	// Fallback: try to get raw value and unmarshal manually
	rawValue := cfg.GetString("cel_workload_exclude")
	if rawValue == "" {
		return nil, nil
	}

	// handles both yaml and json input
	err = yaml.Unmarshal([]byte(rawValue), &celConfig)
	if err == nil {
		return celConfig, nil
	}

	return nil, err
}
