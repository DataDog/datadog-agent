// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v2"
)

// datadogYAMLConfig holds the subset of datadog.yaml fields that feed into Env.
type datadogYAMLConfig struct {
	APIKey    string `yaml:"api_key"`
	Site      string `yaml:"site"`
	Installer struct {
		Registry struct {
			URL        string                                           `yaml:"url"`
			Auth       string                                           `yaml:"auth"`
			Username   string                                           `yaml:"username"`
			Password   string                                           `yaml:"password"`
			Extensions map[string]map[string]extensionRegistryYAMLEntry `yaml:"extensions"`
		} `yaml:"registry"`
	} `yaml:"installer"`
}

// extensionRegistryYAMLEntry maps one extension override in datadog.yaml.
type extensionRegistryYAMLEntry struct {
	URL      string `yaml:"url,omitempty"`
	Auth     string `yaml:"auth,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// readDatadogYAML reads the relevant subset of datadog.yaml from configDir.
// Returns nil if the file doesn't exist or can't be parsed (best-effort).
func readDatadogYAML(configDir string) *datadogYAMLConfig {
	if configDir == "" {
		return nil
	}
	configPath := filepath.Join(configDir, "datadog.yaml")
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	var config datadogYAMLConfig
	if err = yaml.Unmarshal(rawConfig, &config); err != nil {
		return nil
	}
	return &config
}

// applyConfig overlays datadog.yaml values onto env for fields that have a
// non-empty value in the config. This is called before env var processing,
// so env vars naturally take precedence by overwriting these values later.
func applyConfig(env *Env, cfg *datadogYAMLConfig) {
	if cfg.APIKey != "" {
		env.APIKey = cfg.APIKey
	}
	if cfg.Site != "" {
		env.Site = cfg.Site
	}

	r := cfg.Installer.Registry
	if r.URL != "" {
		env.RegistryOverride = r.URL
	}
	if r.Auth != "" {
		env.RegistryAuthOverride = r.Auth
	}
	if r.Username != "" {
		env.RegistryUsername = r.Username
	}
	if r.Password != "" {
		env.RegistryPassword = r.Password
	}

	// Extension registry overrides: installer.registry.extensions.<pkg>.<ext>
	if len(r.Extensions) > 0 {
		if env.ExtensionRegistryOverrides == nil {
			env.ExtensionRegistryOverrides = make(map[string]map[string]ExtensionRegistryOverride, len(r.Extensions))
		}
		for pkg, extMap := range r.Extensions {
			if len(extMap) == 0 {
				continue
			}
			if env.ExtensionRegistryOverrides[pkg] == nil {
				env.ExtensionRegistryOverrides[pkg] = make(map[string]ExtensionRegistryOverride, len(extMap))
			}
			for extName, extCfg := range extMap {
				override := env.ExtensionRegistryOverrides[pkg][extName]
				if extCfg.URL != "" {
					override.URL = extCfg.URL
				}
				if extCfg.Auth != "" {
					override.Auth = extCfg.Auth
				}
				if extCfg.Username != "" {
					override.Username = extCfg.Username
				}
				if extCfg.Password != "" {
					override.Password = extCfg.Password
				}
				env.ExtensionRegistryOverrides[pkg][extName] = override
			}
		}
	}
}
