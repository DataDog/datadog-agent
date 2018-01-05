// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/spf13/viper"

	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

// LogsAgent is the global configuration object
var LogsAgent = ddconfig.Datadog

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
	yamlExtension = ".yaml"
	ymlExtension  = ".yml"
)

const logsRules = "LogsRules"
const logsIntegrationsStatus = "LogsIntegrationsStatus"

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
}

// IntegrationConfig represents a dd agent config, which includes infra and logs parts
type IntegrationConfig struct {
	Logs []IntegrationConfigLogSource
}

// GetLogsSources returns a list of integration sources
func GetLogsSources() []*IntegrationConfigLogSource {
	return getLogsSources(LogsAgent)
}

func getLogsSources(config *viper.Viper) []*IntegrationConfigLogSource {
	return config.Get(logsRules).([]*IntegrationConfigLogSource)
}

// GetIntegrationsStatus returns all the integrations status
// if not set returns nil
func GetIntegrationsStatus() []status.Integration {
	return getIntegrationsStatus(LogsAgent)
}

func getIntegrationsStatus(config *viper.Viper) []status.Integration {
	if config.Get(logsIntegrationsStatus) != nil {
		return config.Get(logsIntegrationsStatus).([]status.Integration)
	}
	return nil
}

// BuildLogsAgentIntegrationsConfigs looks for all yml configs in the ddconfdPath directory,
// and initializes the LogsAgent integrations configs
func BuildLogsAgentIntegrationsConfigs() error {
	return buildLogsAgentIntegrationsConfig(LogsAgent, LogsAgent.GetString("confd_path"))
}

func buildLogsAgentIntegrationsConfig(config *viper.Viper, ddconfdPath string) error {

	integrationConfigFiles := availableIntegrationConfigs(ddconfdPath)
	logsSourceConfigs := []*IntegrationConfigLogSource{}
	integrationsStatus := []status.Integration{}

	for _, file := range integrationConfigFiles {
		var integrationConfig IntegrationConfig
		var viperCfg = viper.New()

		integrationName := strings.TrimSuffix(file, filepath.Ext(file))
		integrationSources := []status.Source{}
		integrationErrors := []status.Error{}

		viperCfg.SetConfigFile(filepath.Join(ddconfdPath, file))
		err := viperCfg.ReadInConfig()
		if err != nil {
			log.Error(err)
			integrationErrors = append(integrationErrors, status.Error{err.Error()})
			integrationsStatus = append(integrationsStatus, status.Integration{Name: integrationName, Errors: integrationErrors})
			continue
		}
		err = viperCfg.Unmarshal(&integrationConfig)
		if err != nil {
			log.Error(err)
			integrationErrors = append(integrationErrors, status.Error{err.Error()})
			integrationsStatus = append(integrationsStatus, status.Integration{Name: integrationName, Errors: integrationErrors})
			continue
		}

		for _, logSourceConfigIterator := range integrationConfig.Logs {
			logSourceConfig := logSourceConfigIterator
			err = validateSource(logSourceConfig)
			if err != nil {
				log.Error(err)
				integrationErrors = append(integrationErrors, status.Error{err.Error()})
				continue
			}

			rules, err := validateProcessingRules(logSourceConfig.ProcessingRules)
			if err != nil {
				log.Error(err)
				integrationErrors = append(integrationErrors, status.Error{err.Error()})
				continue
			}
			logSourceConfig.ProcessingRules = rules

			logSourceConfig.TagsPayload = BuildTagsPayload(logSourceConfig.Tags, logSourceConfig.Source, logSourceConfig.SourceCategory)

			logsSourceConfigs = append(logsSourceConfigs, &logSourceConfig)
			integrationSources = append(integrationSources, buildSourceStatus(logSourceConfig))
		}

		integrationsStatus = append(integrationsStatus, status.Integration{Name: integrationName, Sources: integrationSources, Errors: integrationErrors})
	}

	// save the integrations status for further status queries
	config.Set(logsIntegrationsStatus, integrationsStatus)

	if len(logsSourceConfigs) == 0 {
		return fmt.Errorf("Could not find any valid logs configuration file in %s", ddconfdPath)
	}

	config.Set(logsRules, logsSourceConfigs)

	return nil
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
			return nil, fmt.Errorf("logs-agent misconfigured: all logs processing rules must have a name")
		}
		if rule.Pattern == "" {
			return nil, fmt.Errorf("logs-agent misconfigured: all logs processing rules must have a pattern")
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
				return nil, fmt.Errorf("logs-agent misconfigured: type must be set for logs processing rule `%s`", rule.Name)
			}
			return nil, fmt.Errorf("logs-agent misconfigured: type %s is not supported for logs processing rule `%s`", rule.Type, rule.Name)
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

// buildSourceStatus returns the status of source
func buildSourceStatus(source IntegrationConfigLogSource) status.Source {
	var info string
	switch source.Type {
	case FileType:
		info = fmt.Sprintf("tailing file(s) matching %s", source.Path)
	case DockerType:
		info = fmt.Sprintf("tailing docker image %s", source.Image)
	case TCPType, UDPType:
		info = fmt.Sprintf("listenning on port %d", source.Port)
	}
	return status.Source{Type: source.Type, Info: info}
}
