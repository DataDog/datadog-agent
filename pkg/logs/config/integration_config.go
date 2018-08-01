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
	TCPType          = "tcp"
	UDPType          = "udp"
	FileType         = "file"
	DockerType       = "docker"
	JournaldType     = "journald"
	WindowsEventType = "windows_event"
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
	ReplacePlaceholder string `mapstructure:"replace_placeholder" json:"replace_placeholder"`
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

	IncludeUnits []string `mapstructure:"include_units" json:"include_units"` // Journald
	ExcludeUnits []string `mapstructure:"exclude_units" json:"exclude_units"` // Journald

	Image string // Docker
	Label string // Docker
	Name  string // Docker

	ChannelPath string `mapstructure:"channel_path" json:"channel_path"` // Windows Event
	Query       string // Windows Event

	Service         string
	Source          string
	SourceCategory  string
	Tags            []string
	ProcessingRules []LogsProcessingRule `mapstructure:"log_processing_rules" json:"log_processing_rules"`
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
			if len(config.Tags) > 1 {
				newSlice := []string{}
				for _, splitted := range config.Tags {
					newSlice = append(newSlice, strings.TrimSpace(splitted))
				}
				config.Tags = newSlice
			}

			source := NewLogSource(integrationName, &config)
			sources = append(sources, source)
			// Mis-configured sources are also tracked to report configuration errors
			if isValid, err := Validate(&config); !isValid {
				source.Status.Error(err)
				log.Error(err)
				continue
			}
			if err := Compile(&config); err != nil {
				source.Status.Error(err)
				log.Errorf("could not compile config: %v", err)
				continue
			}
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

// Validate returns an error if the config is misconfigured
func Validate(config *LogsConfig) (bool, error) {
	switch {
	case config.Type == FileType && config.Path == "":
		return false, fmt.Errorf("A file source must have a path")
	case config.Type == TCPType && config.Port == 0:
		return false, fmt.Errorf("A tcp source must have a port")
	case config.Type == UDPType && config.Port == 0:
		return false, fmt.Errorf("A udp source must have a port")
	default:
		return validateProcessingRules(config.ProcessingRules)
	}
}

// validateProcessingRules checks the rules and raises errors if one is misconfigured
func validateProcessingRules(rules []LogsProcessingRule) (bool, error) {
	for _, rule := range rules {
		if rule.Name == "" {
			return false, fmt.Errorf("LogsAgent misconfigured: all log processing rules need a name")
		}
		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch, MaskSequences, MultiLine:
			continue
		case "":
			return false, fmt.Errorf("LogsAgent misconfigured: type must be set for log processing rule `%s`", rule.Name)
		default:
			return false, fmt.Errorf("LogsAgent misconfigured: type %s is unsupported for log processing rule `%s`", rule.Type, rule.Name)
		}
	}
	return true, nil
}

// Compile compiles all processing rules regular expression
func Compile(config *LogsConfig) error {
	rules := config.ProcessingRules
	for i, rule := range rules {
		if rule.Pattern == "" {
			return fmt.Errorf("no pattern provided for processing rule: %s", rule.Name)
		}
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return err
		}
		switch rule.Type {
		case ExcludeAtMatch, IncludeAtMatch:
			rules[i].Reg = re
		case MaskSequences:
			rules[i].Reg = re
			rules[i].ReplacePlaceholderBytes = []byte(rule.ReplacePlaceholder)
		case MultiLine:
			rules[i].Reg, err = regexp.Compile("^" + rule.Pattern)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("invalid type for rule %s: %s", rule.Name, rule.Type)
		}

	}
	return nil
}
