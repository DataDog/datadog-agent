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

	log "github.com/cihub/seelog"
	"github.com/spf13/viper"

	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

// Logs source types
const (
	TCPType    = "tcp"
	UDPType    = "udp"
	FileType   = "file"
	DockerType = "docker"
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
	Type                    string
	Name                    string
	ReplacePlaceholder      string `mapstructure:"replace_placeholder"`
	Pattern                 string
	Reg                     *regexp.Regexp
	ReplacePlaceholderBytes []byte
}

// IntegrationConfigLogSource represents a log source config, which can be for instance
// a file to tail or a port to listen to
type IntegrationConfigLogSource struct {
	Type string

	Port int    // Network
	Path string // File

	Image string // Docker
	Label string // Docker

	Service         string
	Logset          string
	Source          string
	SourceCategory  string
	Tags            string
	TagsPayload     []byte
	ProcessingRules []LogsProcessingRule `mapstructure:"log_processing_rules"`

	Tracker *status.Tracker
}

// IntegrationConfig represents a dd agent config, which includes infra and logs parts
type IntegrationConfig struct {
	Logs []IntegrationConfigLogSource
}

// buildLogsSources looks for all yml configs in the ddconfdPath directory,
// and returns a list of all the valid logs sources along with their trackers
func buildLogsSources(ddconfdPath string) ([]*IntegrationConfigLogSource, []*status.SourceToTrack, error) {
	integrationConfigFiles := availableIntegrationConfigs(ddconfdPath)
	logsSourceConfigs := []*IntegrationConfigLogSource{}
	sourcesToTrack := []*status.SourceToTrack{}
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
			logSourceConfig := logSourceConfigIterator
			tracker := status.NewTracker(logSourceConfig.Type)
			// misconfigured sources are also tracked to repport configuration errors
			sourcesToTrack = append(sourcesToTrack, status.NewSourceToTrack(integrationName, tracker))
			err = validateSource(logSourceConfig)
			if err != nil {
				tracker.TrackError(err)
				log.Error(err)
				continue
			}
			rules, err := validateProcessingRules(logSourceConfig.ProcessingRules)
			if err != nil {
				tracker.TrackError(err)
				log.Error(err)
				continue
			}
			logSourceConfig.ProcessingRules = rules
			logSourceConfig.TagsPayload = BuildTagsPayload(logSourceConfig.Tags, logSourceConfig.Source, logSourceConfig.SourceCategory)
			logSourceConfig.Tracker = tracker
			logsSourceConfigs = append(logsSourceConfigs, &logSourceConfig)
		}
	}

	if len(logsSourceConfigs) == 0 {
		return nil, nil, fmt.Errorf("Could not find any valid logs integration configuration file in %s", ddconfdPath)
	}

	return logsSourceConfigs, sourcesToTrack, nil
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

func validateSource(config IntegrationConfigLogSource) error {

	switch config.Type {
	case FileType,
		DockerType,
		TCPType,
		UDPType:
	default:
		return fmt.Errorf("A source must have a valid type (got %s)", config.Type)
	}

	if config.Type == FileType && config.Path == "" {
		return fmt.Errorf("A file source must have a path")
	}

	if config.Type == TCPType && config.Port == 0 {
		return fmt.Errorf("A tcp source must have a port")
	}

	if config.Type == UDPType && config.Port == 0 {
		return fmt.Errorf("A udp source must have a port")
	}

	return nil
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

// BuildTagsPayload generates the bytes array that will be inserted
// into messages given a list of tags
func BuildTagsPayload(configTags, source, sourceCategory string) []byte {

	tagsPayload := []byte{}
	if source != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddsource=\"")...)
		tagsPayload = append(tagsPayload, []byte(source)...)
		tagsPayload = append(tagsPayload, []byte("\"]")...)
	}

	if sourceCategory != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddsourcecategory=\"")...)
		tagsPayload = append(tagsPayload, []byte(sourceCategory)...)
		tagsPayload = append(tagsPayload, []byte("\"]")...)
	}

	if configTags != "" {
		tagsPayload = append(tagsPayload, []byte("[dd ddtags=\"")...)
		tagsPayload = append(tagsPayload, []byte(configTags)...)
		tagsPayload = append(tagsPayload, []byte("\"]")...)
	}

	if len(tagsPayload) == 0 {
		return []byte{'-'}
	}

	return tagsPayload
}
