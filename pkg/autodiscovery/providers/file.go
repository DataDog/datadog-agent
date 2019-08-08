// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package providers

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/configresolver"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type configFormat struct {
	ADIdentifiers []string    `yaml:"ad_identifiers"`
	ClusterCheck  bool        `yaml:"cluster_check"`
	InitConfig    interface{} `yaml:"init_config"`
	MetricConfig  interface{} `yaml:"jmx_metrics"`
	LogsConfig    interface{} `yaml:"logs"`
	Instances     []integration.RawMap
	DockerImages  []string `yaml:"docker_images"` // Only imported for deprecation warning
}

type configPkg struct {
	confs    []integration.Config
	defaults []integration.Config
	others   []integration.Config
}

type configEntry struct {
	conf       integration.Config
	name       string
	isDefault  bool
	isMetric   bool
	isLogsOnly bool
	err        error
}

// FileConfigProvider collect configuration files from disk
type FileConfigProvider struct {
	paths  []string
	Errors map[string]string
}

// NewFileConfigProvider creates a new FileConfigProvider searching for
// configuration files on the given paths
func NewFileConfigProvider(paths []string) *FileConfigProvider {
	return &FileConfigProvider{
		paths:  paths,
		Errors: make(map[string]string),
	}
}

// Collect scans provided paths searching for configuration files. When found,
// it parses the files and try to unmarshall Yaml contents into a CheckConfig
// instance
func (c *FileConfigProvider) Collect() ([]integration.Config, error) {
	configs := []integration.Config{}
	configNames := make(map[string]struct{}) // use this map as a python set
	defaultConfigs := []integration.Config{}

	for _, path := range c.paths {
		log.Infof("%v: searching for configuration files at: %s", c, path)

		entries, err := readDirPtr(path)
		if err != nil {
			log.Warnf("Skipping, %s", err)
			continue
		}

		for _, entry := range entries {
			// We support only one level of nesting for check configs
			if entry.IsDir() {
				dirConfigs := c.collectDir(path, entry)
				if len(dirConfigs.defaults) > 0 {
					defaultConfigs = append(defaultConfigs, dirConfigs.defaults...)
				}
				if len(dirConfigs.others) > 0 {
					// don't save file names for others configs in the configNames maps so they don't override defaults
					configs = append(configs, dirConfigs.others...)
				}
				if len(dirConfigs.confs) > 0 {
					configs = append(configs, dirConfigs.confs...)
					configNames[dirConfigs.confs[0].Name] = struct{}{}
				}
				continue
			}

			entry := c.collectEntry(entry, path, "")
			// we don't collect metric files from the root dir (which check is it for? that's nonsensical!)
			if entry.err != nil || entry.isMetric {
				// logging is handled in collectEntry
				continue
			}

			// determine if a check has to be run by default by
			// searching for check.yaml.default files
			if entry.isDefault {
				defaultConfigs = append(defaultConfigs, entry.conf)
			} else if entry.isLogsOnly {
				// don't save file names for logs only configs in the configNames maps so they don't override defaults
				configs = append(configs, entry.conf)
			} else {
				configs = append(configs, entry.conf)
				configNames[entry.name] = struct{}{}
			}
		}
	}

	// add all the default enabled checks unless another regular
	// configuration file was already provided for the same check
	for _, conf := range defaultConfigs {
		if _, isThere := configNames[conf.Name]; !isThere {
			configs = append(configs, conf)
		} else {
			log.Debugf("Ignoring default config file '%s' because non-default config was found", conf.Name)
		}
	}

	return configs, nil
}

// IsUpToDate is not implemented for the file Providers as the files are not meant to change very often.
func (c *FileConfigProvider) IsUpToDate() (bool, error) {
	return false, nil
}

// String returns a string representation of the FileConfigProvider
func (c *FileConfigProvider) String() string {
	return File
}

// collectEntry collects a file entry and return it's configuration if valid
// the integrationName can be manually provided else it'll use the filename
func (c *FileConfigProvider) collectEntry(file os.FileInfo, path string, integrationName string) configEntry {
	const defaultExt string = ".default"
	fileName := file.Name()
	ext := filepath.Ext(fileName)
	entry := configEntry{}
	absPath := filepath.Join(path, fileName)

	// skip config files that are not of type:
	//  * integration.yaml, integration.yml
	//  * integration.yaml.default, integration.yml.default

	if fileName == "metrics.yaml" || fileName == "metrics.yml" {
		entry.isMetric = true
	}

	if ext == defaultExt {
		entry.isDefault = true
		ext = filepath.Ext(strings.TrimSuffix(fileName, defaultExt))
	}

	if integrationName == "" {
		integrationName = fileName
		if entry.isDefault {
			integrationName = strings.TrimSuffix(integrationName, defaultExt)
		}
		integrationName = strings.TrimSuffix(integrationName, ext)
	}
	entry.name = integrationName

	if ext != ".yaml" && ext != ".yml" {
		log.Tracef("Skipping file: %s", absPath)
		entry.err = errors.New("Invalid config file extension")
		return entry
	}

	var err error
	entry.conf, err = GetIntegrationConfigFromFile(integrationName, absPath)
	if err != nil {
		log.Warnf("%s is not a valid config file: %s", absPath, err)
		c.Errors[integrationName] = err.Error()
		entry.err = errors.New("Invalid config file format")
		return entry
	}

	// if logs is the only integration, set isLogsOnly to true
	if entry.conf.LogsConfig != nil && entry.conf.MetricConfig == nil && len(entry.conf.Instances) == 0 && len(entry.conf.ADIdentifiers) == 0 {
		entry.isLogsOnly = true
	}

	delete(c.Errors, integrationName) // noop if entry is nonexistant
	log.Debug("Found valid configuration in file:", absPath)
	return entry
}

// collectDir collects entries in subdirectories of the main conf folder
func (c *FileConfigProvider) collectDir(parentPath string, folder os.FileInfo) configPkg {
	configs := []integration.Config{}
	defaultConfigs := []integration.Config{}
	otherConfigs := []integration.Config{}
	const dirExt string = ".d"
	dirPath := filepath.Join(parentPath, folder.Name())

	if filepath.Ext(folder.Name()) != dirExt {
		// the name of this directory isn't in the form `integrationName.d`, skip it
		log.Debugf("Not a config folder, skipping directory: %s", dirPath)
		return configPkg{configs, defaultConfigs, otherConfigs}
	}

	// search for yaml files within this directory
	subEntries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		log.Warnf("Skipping config directory %s: %s", dirPath, err)
		return configPkg{configs, defaultConfigs, otherConfigs}
	}

	// strip the trailing `.d`
	integrationName := strings.TrimSuffix(folder.Name(), dirExt)

	// try to load any config file in it
	for _, sEntry := range subEntries {
		if !sEntry.IsDir() {

			entry := c.collectEntry(sEntry, dirPath, integrationName)
			if entry.err != nil {
				// logging already done in collectEntry
				continue
			}
			// determine if a check has to be run by default by
			// searching for integration.yaml.default files
			if entry.isDefault {
				defaultConfigs = append(defaultConfigs, entry.conf)
			} else if entry.isMetric || entry.isLogsOnly {
				otherConfigs = append(otherConfigs, entry.conf)
			} else {
				configs = append(configs, entry.conf)
			}
		}
	}

	return configPkg{confs: configs, defaults: defaultConfigs, others: otherConfigs}
}

// GetIntegrationConfigFromFile returns an instance of integration.Config if `fpath` points to a valid config file
func GetIntegrationConfigFromFile(name, fpath string) (integration.Config, error) {
	cf := configFormat{}
	config := integration.Config{Name: name}

	// Read file contents
	// FIXME: ReadFile reads the entire file, possible security implications
	yamlFile, err := readFilePtr(fpath)
	if err != nil {
		return config, err
	}

	// Parse configuration
	// Try UnmarshalStrict first, so we can warn about duplicated keys
	if strictErr := yaml.UnmarshalStrict(yamlFile, &cf); strictErr != nil {
		if err := yaml.Unmarshal(yamlFile, &cf); err != nil {
			return config, err
		}
		log.Warnf("reading config file %v: %v\n", fpath, strictErr)
	}

	// If no valid instances were found & this is neither a metrics file, nor a logs file
	// this is not a valid configuration file
	if cf.MetricConfig == nil && cf.LogsConfig == nil && len(cf.Instances) < 1 {
		return config, errors.New("Configuration file contains no valid instances")
	}

	// at this point the Yaml was already parsed, no need to check the error
	if cf.InitConfig != nil {
		rawInitConfig, _ := yaml.Marshal(cf.InitConfig)
		config.InitConfig = rawInitConfig
	}

	// Go through instances and return corresponding []byte
	for _, instance := range cf.Instances {
		// at this point the Yaml was already parsed, no need to check the error
		rawConf, _ := yaml.Marshal(instance)
		config.Instances = append(config.Instances, rawConf)
	}

	// If JMX metrics were found, add them to the config
	if cf.MetricConfig != nil {
		rawMetricConfig, _ := yaml.Marshal(cf.MetricConfig)
		config.MetricConfig = rawMetricConfig
	}

	// If logs was found, add it to the config
	if cf.LogsConfig != nil {
		logsConfig := make(map[string]interface{})
		logsConfig["logs"] = cf.LogsConfig
		config.LogsConfig, _ = yaml.Marshal(logsConfig)
	}

	// Copy auto discovery identifiers
	config.ADIdentifiers = cf.ADIdentifiers

	// Copy cluster_check status
	config.ClusterCheck = cf.ClusterCheck

	// DockerImages entry was found: we ignore it if no ADIdentifiers has been found
	if len(cf.DockerImages) > 0 && len(cf.ADIdentifiers) == 0 {
		return config, errors.New("the 'docker_images' section is deprecated, please use 'ad_identifiers' instead")
	}

	// Interpolate env vars. Returns an error a variable wasn't subsituted, ignore it.
	_ = configresolver.SubstituteTemplateEnvVars(&config)

	config.Source = "file:" + fpath

	return config, err
}
