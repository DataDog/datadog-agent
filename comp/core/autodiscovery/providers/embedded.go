// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/configresolver"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup" //nolint:pkgconfigusage
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// embeddedCheckConfig represents a check configuration embedded in code.
type embeddedCheckConfig struct {
	// InitConfig is the init_config section as a map
	InitConfig map[string]interface{}
	// Instances is the list of instances, each as a map
	Instances []integration.RawMap
}

// embeddedCheckConfigs contains check configurations embedded in the agent code.
// These are equivalent to the .yaml.default files but embedded in code.
// Checks not defined here will use a default empty config (nil InitConfig, single empty instance).
var embeddedCheckConfigs = map[string]embeddedCheckConfig{}

// EmbeddedConfigProvider provides check configurations embedded in the agent code.
// It is primarily used for infrastructure modes (basic, end_user_device) to load
// exclusive checks without requiring configuration files on disk.
type EmbeddedConfigProvider struct {
	config    pkgconfigmodel.Reader
	collected bool
}

// NewEmbeddedConfigProvider creates a new EmbeddedConfigProvider.
func NewEmbeddedConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, telemetryStore *telemetry.Store) (types.ConfigProvider, error) {
	return newEmbeddedConfigProvider(pkgconfigsetup.Datadog(), telemetryStore), nil
}

func newEmbeddedConfigProvider(config pkgconfigmodel.Reader, _ *telemetry.Store) *EmbeddedConfigProvider {
	return &EmbeddedConfigProvider{
		config:    config,
		collected: false,
	}
}

// Collect returns the check configurations for the current infrastructure mode.
// If the mode is "full", no configs are returned (default behavior applies).
// If a user has defined their own config file for a check, the embedded config is skipped.
func (p *EmbeddedConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	p.collected = true

	mode := p.config.GetString("infrastructure_mode")
	if mode == "full" {
		log.Debug("Infrastructure mode is 'full', embedded provider will not load any checks")
		return nil, nil
	}

	exclusiveChecks := p.config.GetStringSlice("integration.infrastructure_mode_exclusive_checks." + mode)
	if len(exclusiveChecks) == 0 {
		log.Debugf("No exclusive checks configured for infrastructure mode '%s'", mode)
		return nil, nil
	}

	log.Debugf("Infrastructure mode '%s' detected, loading embedded check configurations", mode)

	configs := make([]integration.Config, 0, len(exclusiveChecks))
	for _, checkName := range exclusiveChecks {
		// Skip if user has defined their own config file for this check
		if hasUserDefinedConfig(checkName, p.config) {
			log.Debugf("Skipping embedded config for check '%s': user-defined config found", checkName)
			continue
		}

		cfg, err := getIntegrationConfigFromEmbedded(checkName)
		if err != nil {
			log.Warnf("Failed to create config for check '%s': %v", checkName, err)
			continue
		}
		configs = append(configs, cfg)
	}

	return configs, nil
}

// hasUserDefinedConfig checks if a user-defined config file exists for the given check.
// This checks for conf.yaml or conf.yml in the check's conf.d directory,
// excluding .yaml.default files which are just defaults.
func hasUserDefinedConfig(checkName string, config pkgconfigmodel.Reader) bool {
	// Get the conf.d paths to search
	confPaths := []string{
		filepath.Join(defaultpaths.GetDistPath(), "conf.d"),
		config.GetString("confd_path"),
	}

	checkDir := checkName + ".d"

	for _, confPath := range confPaths {
		if confPath == "" {
			continue
		}

		checkDirPath := filepath.Join(confPath, checkDir)

		// Check if the check directory exists
		entries, err := os.ReadDir(checkDirPath)
		if err != nil {
			continue // Directory doesn't exist or can't be read
		}

		// Look for user-defined config files (not .default files)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			// Check for conf.yaml or conf.yml (not .default)
			if name == "conf.yaml" || name == "conf.yml" {
				return true
			}
			// Also check for any .yaml/.yml file that's not a .default or metrics file
			ext := filepath.Ext(name)
			if (ext == ".yaml" || ext == ".yml") &&
				!strings.HasSuffix(name, ".default") &&
				name != "metrics.yaml" && name != "metrics.yml" &&
				name != "auto_conf.yaml" && name != "auto_conf.yml" {
				return true
			}
		}
	}

	return false
}

// getIntegrationConfigFromEmbedded creates an integration.Config from embedded configuration.
// This mirrors the behavior of GetIntegrationConfigFromFile but uses embedded config data.
func getIntegrationConfigFromEmbedded(checkName string) (integration.Config, error) {
	conf := integration.Config{
		Name:     checkName,
		Provider: names.Embedded,
		Source:   "embedded:" + checkName,
	}

	// Get the embedded config for this check, or use a default empty config
	checkConfig, exists := embeddedCheckConfigs[checkName]
	if !exists {
		// Use default empty config for checks not explicitly defined
		checkConfig = embeddedCheckConfig{
			InitConfig: nil,
			Instances:  []integration.RawMap{{}},
		}
	}

	// Marshal InitConfig to YAML bytes (same as GetIntegrationConfigFromFile)
	if checkConfig.InitConfig != nil {
		rawInitConfig, err := yaml.Marshal(checkConfig.InitConfig)
		if err != nil {
			return conf, fmt.Errorf("failed to marshal init_config for check '%s': %w", checkName, err)
		}
		conf.InitConfig = rawInitConfig
	}

	// Marshal each instance to YAML bytes (same as GetIntegrationConfigFromFile)
	for _, instance := range checkConfig.Instances {
		rawConf, err := yaml.Marshal(instance)
		if err != nil {
			return conf, fmt.Errorf("failed to marshal instance for check '%s': %w", checkName, err)
		}
		conf.Instances = append(conf.Instances, integration.Data(rawConf))
	}

	// Substitute template env vars (same as GetIntegrationConfigFromFile)
	if err := configresolver.SubstituteTemplateEnvVars(&conf); err != nil {
		// Ignore NoServiceError since service is always nil for integration configs
		if _, ok := err.(*configresolver.NoServiceError); !ok {
			log.Warnf("Failed to substitute template var for check '%s': %v", checkName, err)
		}
	}

	return conf, nil
}

// IsUpToDate returns whether the provider's configurations are still up to date.
// The infrastructure mode is not expected to change at runtime, so once collected, it's up to date.
func (p *EmbeddedConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	return p.collected, nil
}

// String returns a string representation of the EmbeddedConfigProvider.
func (p *EmbeddedConfigProvider) String() string {
	return names.Embedded
}

// GetConfigErrors returns a map of configuration errors.
func (p *EmbeddedConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}
