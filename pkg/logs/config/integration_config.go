// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/viper"
)

// Logs source types
const (
	TCPType      = "tcp"
	UDPType      = "udp"
	FileType     = "file"
	DockerType   = "docker"
	JournaldType = "journald"
	EventLogType = "windows_event"
)

// Logs rule types
const (
	ExcludeAtMatch = "exclude_at_match"
	IncludeAtMatch = "include_at_match"
	MaskSequences  = "mask_sequences"
	MultiLine      = "multi_line"
)

// Valid integration config extensions
const (
	directoryExtension = ".d"
	yamlExtension      = ".yaml"
	ymlExtension       = ".yml"
)

// LogsProcessingRule defines an exclusion or a masking rule to
// be applied on log lines
type LogsProcessingRule struct {
	Type               string
	Name               string
	ReplacePlaceholder string `mapstructure:"replace_placeholder"`
	Pattern            string
	// TODO: should be moved out
	Reg                     *regexp.Regexp
	ReplacePlaceholderBytes []byte
}

// LogsConfig represents a log source config, which can be for instance
// a file to tail or a port to listen to.
type LogsConfig struct {
	Type string

	Port int    // Network
	Path string // File, Journald

	IncludeUnits         []string `mapstructure:"include_units"`         // Journald
	ExcludeUnits         []string `mapstructure:"exclude_units"`         // Journald
	DisableNormalization bool     `mapstructure:"disable_normalization"` // Journald

	Image string // Docker
	Label string // Docker
	Name  string // Docker

	ChannelPath string `mapstructure:"channel_path"` // Windows Event
	Query       string // Windows Event

	Service         string
	Source          string
	SourceCategory  string
	Tags            []string
	ProcessingRules []LogsProcessingRule `mapstructure:"log_processing_rules"`
}

// IntegrationConfig represents a DataDog agent configuration file, which includes infra and logs parts.
type IntegrationConfig struct {
	Logs []LogsConfig
}

// buildLogSourcesFromDirectory looks for all yml configs in the ddconfdPath directory,
// and returns a list of all the valid logs sources along with their trackers
func buildLogSourcesFromDirectory(ddconfdPath string) []*LogSource {
	integrationConfigFiles := availableIntegrationConfigs(ddconfdPath)
	var sources []*LogSource
	for _, file := range integrationConfigFiles {
		var integrationConfig IntegrationConfig
		var viperCfg = viper.New()
		viperCfg.SetConfigFile(filepath.Join(ddconfdPath, file))
		err := viperCfg.ReadInConfig()
		if err != nil {
			log.Error(err)
			continue
		}
		err = viperCfg.Unmarshal(&integrationConfig)
		if err != nil {
			log.Error(err)
			continue
		}
		integrationName, err := buildIntegrationName(file)
		if err != nil {
			log.Error(err)
			continue
		}
		for _, logSourceConfigIterator := range integrationConfig.Logs {
			config := logSourceConfigIterator

			// Users can specify tags as comma separated string, or as YAML array. Handle the first case here
			if len(config.Tags) == 1 {
				newSlice := []string{}
				for _, splitted := range strings.Split(config.Tags[0], ",") {
					newSlice = append(newSlice, strings.TrimSpace(splitted))
				}
				config.Tags = newSlice
			}

			source := NewLogSource(integrationName, &config)
			sources = append(sources, source)
			// Mis-configured sources are also tracked to report configuration errors
			err = validateConfig(config)
			if err != nil {
				source.Status.Error(err)
				log.Error(err)
				continue
			}
			rules, err := validateProcessingRules(config.ProcessingRules)
			if err != nil {
				source.Status.Error(err)
				log.Error(err)
				continue
			}
			config.ProcessingRules = rules
		}
	}

	return sources
}

// buildIntegrationName returns the name of the integration
func buildIntegrationName(filePath string) (string, error) {
	validFileExtensions := []string{yamlExtension, ymlExtension}
	components := strings.Split(filePath, string(os.PathSeparator))
	if len(components) == 1 {
		for _, ext := range validFileExtensions {
			// check if file has a valid extension
			if strings.HasSuffix(components[0], ext) {
				return strings.TrimSuffix(components[0], ext), nil
			}
		}
	} else if len(components) == 2 {
		// check if directory has a valid extension
		if strings.HasSuffix(components[0], directoryExtension) {
			return strings.TrimSuffix(components[0], directoryExtension), nil
		}
	}
	return "", fmt.Errorf("Invalid file path: %s", filePath)
}

// availableIntegrationConfigs lists yaml files in ddconfdPath
func availableIntegrationConfigs(ddconfdPath string) []string {
	integrationConfigFiles := integrationConfigsFromDirectory(ddconfdPath, ".")
	dirs, _ := ioutil.ReadDir(ddconfdPath)
	for _, d := range dirs {
		if d.IsDir() {
			integrationConfigFiles = append(
				integrationConfigFiles,
				integrationConfigsFromDirectory(filepath.Join(ddconfdPath, d.Name()), d.Name())...,
			)
		}
	}
	return integrationConfigFiles
}

// integrationConfigsFromDirectory returns a list of yaml files in a directory
func integrationConfigsFromDirectory(dir string, prefix string) []string {
	var integrationConfigFiles []string
	files, _ := ioutil.ReadDir(dir)
	for _, f := range files {
		if !f.IsDir() {
			ext := filepath.Ext(f.Name())
			if ext == yamlExtension || ext == ymlExtension {
				integrationConfigFiles = append(integrationConfigFiles, filepath.Join(prefix, f.Name()))
			}
		}
	}
	return integrationConfigFiles
}

func validateConfig(config LogsConfig) error {
	switch config.Type {
	case FileType, DockerType, TCPType, UDPType, JournaldType, EventLogType:
	default:
		return fmt.Errorf("A source must have a valid type (got %s)", config.Type)
	}

	switch {
	case config.Type == FileType && config.Path == "":
		return fmt.Errorf("A file source must have a path")
	case config.Type == TCPType && config.Port == 0:
		return fmt.Errorf("A tcp source must have a port")
	case config.Type == UDPType && config.Port == 0:
		return fmt.Errorf("A udp source must have a port")
	default:
		return nil
	}
}

// validateProcessingRules checks the rules and raises errors if one is misconfigured
func validateProcessingRules(rules []LogsProcessingRule) ([]LogsProcessingRule, error) {
	for i, rule := range rules {
		if rule.Name == "" {
			return nil, fmt.Errorf("LogsAgent misconfigured: all log processing rules need a name")
		}
		switch rule.Type {
		case ExcludeAtMatch:
			rules[i].Reg = regexp.MustCompile(rule.Pattern)
		case IncludeAtMatch:
			rules[i].Reg = regexp.MustCompile(rule.Pattern)
		case MaskSequences:
			rules[i].Reg = regexp.MustCompile(rule.Pattern)
			rules[i].ReplacePlaceholderBytes = []byte(rule.ReplacePlaceholder)
		case MultiLine:
			rules[i].Reg = regexp.MustCompile("^" + rule.Pattern)
		default:
			if rule.Type == "" {
				return nil, fmt.Errorf("LogsAgent misconfigured: type must be set for log processing rule `%s`", rule.Name)
			}
			return nil, fmt.Errorf("LogsAgent misconfigured: type %s is unsupported for log processing rule `%s`", rule.Type, rule.Name)
		}
	}
	return rules, nil
}
