// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common contains the HostInstaller struct which is used to write the agent agentConfiguration to disk
package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/paths"
	"gopkg.in/yaml.v2"
)

var (
	configDir              = "/etc/datadog-agent"
	datadogConfFile        = filepath.Join(configDir, "datadog.yaml")
	logsConfFile           = filepath.Join(configDir, "conf.d/configured_at_install_logs.yaml")
	sparkConfigFile        = filepath.Join(configDir, "conf.d/spark.d/spark.yaml")
	injectTracerConfigFile = filepath.Join(configDir, "/etc/datadog-agent/inject/tracer.yaml")
)

// HostInstaller is a struct that represents the agent agentConfiguration
// used to write the agentConfiguration to disk in datadog-installer custom setup scenarios
type HostInstaller struct {
	env *env.Env

	agentConfig    map[string]interface{}
	logsConfig     logsConfig
	sparkConfig    sparkConfig
	injectorConfig injectorConfig
	hostTags       []tag
	ddUID          int
	ddGID          int

	injectorVersion string
	javaVersion     string
	agentVersion    string
}

type tag struct {
	key   string `yaml:"key"`
	value string `yaml:"value"`
}

type logsConfig struct {
	Logs []LogConfig `yaml:"logs"`
}

// LogConfig is a struct that represents a single log agentConfiguration
type LogConfig struct {
	Type    string `yaml:"type"`
	Path    string `yaml:"path"`
	Service string `yaml:"service"`
	Source  string `yaml:"source"`
}

type sparkConfig struct {
	InitConfig interface{}     `yaml:"init_config"`
	Instances  []SparkInstance `yaml:"instances"`
}

// SparkInstance is a struct that represents a single spark instance
type SparkInstance struct {
	SparkURL         string `yaml:"spark_url"`
	SparkClusterMode string `yaml:"spark_cluster_mode"`
	ClusterName      string `yaml:"cluster_name"`
	StreamingMetrics bool   `yaml:"streaming_metrics"`
}

type injectorConfig struct {
	Version       int      `yaml:"version"`
	ConfigSources string   `yaml:"config_sources"`
	EnvsToInject  []EnvVar `yaml:"additional_environment_variables"`
}

// EnvVar is a struct that represents an environment variable
type EnvVar struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

// NewHostInstaller creates a new HostInstaller struct and loads the existing agentConfiguration from disk
func NewHostInstaller(env *env.Env) (*HostInstaller, error) {
	ddUID, ddGID, err := packages.GetAgentIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get agent user and group IDs: %v", err)
	}
	return newHostInstaller(env, ddUID, ddGID)
}

func newHostInstaller(env *env.Env, ddUID, ddGID int) (*HostInstaller, error) {
	i := &HostInstaller{}
	if env.APIKey == "" {
		return nil, fmt.Errorf("DD_API key is required")
	}
	i.AddAgentConfig("api_key", env.APIKey)

	if env.Site != "" {
		i.AddAgentConfig("site", env.Site)
	}
	i.ddUID = ddUID
	i.ddGID = ddGID
	i.env = env
	return i, nil
}

// SetAgentVersion sets the agent version to install
func (i *HostInstaller) SetAgentVersion(version string) {
	i.agentVersion = version
}

// SetInjectorVersion sets the injector version to install
func (i *HostInstaller) SetInjectorVersion(version string) {
	i.injectorVersion = version
}

// SetJavaTracerVersion sets the java tracer version to install
func (i *HostInstaller) SetJavaTracerVersion(version string) {
	i.javaVersion = version
}

// AddTracerEnv adds an environment variable to the list of environment variables to inject
func (i *HostInstaller) AddTracerEnv(key, value string) {
	i.injectorConfig.EnvsToInject = append(i.injectorConfig.EnvsToInject, EnvVar{Key: key, Value: value})
}

// AddAgentConfig adds a key value pair to the agent agentConfiguration
func (i *HostInstaller) AddAgentConfig(key string, value interface{}) {
	i.agentConfig[key] = value
}

// AddLogConfig adds a log agentConfiguration to the agent configuration
func (i *HostInstaller) AddLogConfig(log LogConfig) {
	i.logsConfig.Logs = append(i.logsConfig.Logs, log)
	if len(i.logsConfig.Logs) == 1 {
		i.AddAgentConfig("logs_enabled", true)
	}
}

// AddSparkInstance adds a spark instance to the agent agentConfiguration
func (i *HostInstaller) AddSparkInstance(spark SparkInstance) {
	i.sparkConfig.Instances = append(i.sparkConfig.Instances, spark)
}

// AddHostTag adds a host tag to the agent agentConfiguration
func (i *HostInstaller) AddHostTag(key, value string) {
	i.hostTags = append(i.hostTags, tag{key, value})
}

func (i *HostInstaller) writeYamlConfig(path string, yml interface{}, perm os.FileMode, agentOwner bool) error {
	data, err := yaml.Marshal(yml)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}
	if err = os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("failed to write to %s: %v", path, err)
	}
	// Change ownership of the file to the agent user
	// ddUID=0 happens in local test environments
	if agentOwner && i.ddUID != 0 {
		if err := os.Chown(path, i.ddUID, i.ddGID); err != nil {
			return fmt.Errorf("failed to change ownership of %s: %v", path, err)
		}
	}
	return nil
}

func convertTagsToYaml(tags []tag) []interface{} {
	result := make([]interface{}, 0, len(tags))
	for _, tag := range tags {
		result = append(result, fmt.Sprintf("%s:%s", tag.key, tag.value))
	}
	return result
}

func (i *HostInstaller) writeConfigs() error {
	if len(i.hostTags) > 0 {
		i.AddAgentConfig("tags", convertTagsToYaml(i.hostTags))
	}

	if err := i.writeYamlConfig(datadogConfFile, i.agentConfig, 0640, true); err != nil {
		return err
	}
	if len(i.logsConfig.Logs) > 0 {
		if err := i.writeYamlConfig(logsConfFile, i.logsConfig, 0644, true); err != nil {
			return err
		}
	}
	if len(i.sparkConfig.Instances) > 0 {
		if err := i.writeYamlConfig(sparkConfigFile, i.sparkConfig, 0644, true); err != nil {
			return err
		}
	}
	if len(i.injectorConfig.EnvsToInject) > 0 {
		if err := i.writeYamlConfig(injectTracerConfigFile, i.injectorConfig, 0644, false); err != nil {
			return err
		}
	}
	return nil
}

// ConfigureAndInstall writes configurations to disk and installs desired packages
func (i *HostInstaller) ConfigureAndInstall(ctx context.Context) error {
	if err := i.writeConfigs(); err != nil {
		return fmt.Errorf("failed to write configurations: %w", err)
	}

	cmd := exec.NewInstallerExec(i.env, paths.StableInstallerPath)

	if i.injectorVersion != "" {
		if err := cmd.Install(ctx, oci.PackageURL(i.env, "datadog-apm-inject", i.injectorVersion), nil); err != nil {
			return fmt.Errorf("failed to install injector: %w", err)
		}
	}
	if i.javaVersion != "" {
		if err := cmd.Install(ctx, oci.PackageURL(i.env, "datadog-apm-library-java", i.javaVersion), nil); err != nil {
			return fmt.Errorf("failed to install java library: %w", err)
		}
	}
	if i.agentVersion != "" {
		if err := cmd.Install(ctx, oci.PackageURL(i.env, "datadog-agent", i.agentVersion), nil); err != nil {
			return fmt.Errorf("failed to install Databricks agent: %w", err)
		}
	}
	return nil
}
