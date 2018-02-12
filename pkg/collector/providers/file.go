// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package providers

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"

	"gopkg.in/yaml.v2"
)

type configFormat struct {
	ADIdentifiers []string    `yaml:"ad_identifiers"`
	InitConfig    interface{} `yaml:"init_config"`
	MetricConfig  interface{} `yaml:"jmx_metrics"`
	LogsConfig    interface{} `yaml:"logs"`
	Instances     []check.ConfigRawMap
}

type configPkg struct {
	confs    []check.Config
	defaults []check.Config
	metrics  []check.Config
}

type configEntry struct {
	conf       check.Config
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
func (c *FileConfigProvider) Collect() ([]check.Config, error) {
	configs := []check.Config{}
	configNames := make(map[string]struct{}) // use this map as a python set
	defaultConfigs := []check.Config{}

	for _, path := range c.paths {
		log.Infof("%v: searching for configuration files at: %s", c, path)

		entries, err := ioutil.ReadDir(path)
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
				if len(dirConfigs.metrics) > 0 {
					// don't save metric file names in the configNames maps so they don't override defaults
					configs = append(configs, dirConfigs.metrics...)
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

			if entry.isLogsOnly {
				// skip logs-only configs for now as they are not processed by autodiscovery
				continue
			}

			// determine if a check has to be run by default by
			// searching for check.yaml.default files
			if entry.isDefault {
				defaultConfigs = append(defaultConfigs, entry.conf)
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
	return "File Configuration Provider"
}

// collectEntry collects a file entry and return it's configuration if valid
// the checkName can be manually provided else it'll use the filename
func (c *FileConfigProvider) collectEntry(file os.FileInfo, path string, checkName string) configEntry {
	const defaultExt string = ".default"
	fileName := file.Name()
	ext := filepath.Ext(fileName)
	entry := configEntry{}
	absPath := filepath.Join(path, fileName)

	// skip config files that are not of type:
	//  * check.yaml, check.yml
	//  * check.yaml.default, check.yml.default

	if fileName == "metrics.yaml" || fileName == "metrics.yml" {
		entry.isMetric = true
	}

	if ext == defaultExt {
		entry.isDefault = true
		ext = filepath.Ext(strings.TrimSuffix(fileName, defaultExt))
	}

	if checkName == "" {
		checkName = fileName
		if entry.isDefault {
			checkName = strings.TrimSuffix(checkName, defaultExt)
		}
		checkName = strings.TrimSuffix(checkName, ext)
	}
	entry.name = checkName

	if ext != ".yaml" && ext != ".yml" {
		log.Tracef("Skipping file: %s", absPath)
		entry.err = errors.New("Invalid config file extension")
		return entry
	}

	var err error
	entry.conf, err = GetCheckConfigFromFile(checkName, absPath)
	if err != nil {
		log.Warnf("%s is not a valid config file: %s", absPath, err)
		c.Errors[checkName] = err.Error()
		entry.err = errors.New("Invalid config file format")
		return entry
	}

	// if logs is the only integration, set isLogsOnly to true
	if entry.conf.LogsConfig != nil && entry.conf.MetricConfig == nil && len(entry.conf.Instances) == 0 && len(entry.conf.ADIdentifiers) == 0 {
		entry.isLogsOnly = true
	}

	delete(c.Errors, checkName) // noop if entry is nonexistant
	log.Debug("Found valid configuration in file:", absPath)
	return entry
}

// collectDir collects entries in subdirectories of the main conf folder
func (c *FileConfigProvider) collectDir(parentPath string, folder os.FileInfo) configPkg {
	configs := []check.Config{}
	defaultConfigs := []check.Config{}
	metricConfigs := []check.Config{}
	const dirExt string = ".d"
	dirPath := filepath.Join(parentPath, folder.Name())

	if filepath.Ext(folder.Name()) != dirExt {
		// the name of this directory isn't in the form `checkname.d`, skip it
		log.Debugf("Not a config folder, skipping directory: %s", dirPath)
		return configPkg{configs, defaultConfigs, metricConfigs}
	}

	// search for yaml files within this directory
	subEntries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		log.Warnf("Skipping config directory %s: %s", dirPath, err)
		return configPkg{configs, defaultConfigs, metricConfigs}
	}

	// strip the trailing `.d`
	checkName := strings.TrimSuffix(folder.Name(), dirExt)

	// try to load any config file in it
	for _, sEntry := range subEntries {
		if !sEntry.IsDir() {

			entry := c.collectEntry(sEntry, dirPath, checkName)
			if entry.err != nil {
				// logging already done in collectEntry
				continue
			}
			// determine if a check has to be run by default by
			// searching for check.yaml.default files
			if entry.isDefault {
				defaultConfigs = append(defaultConfigs, entry.conf)
			} else if entry.isMetric {
				metricConfigs = append(metricConfigs, entry.conf)
			} else if entry.isLogsOnly {
				// skip logs-only configs for now as they are not processed by autodiscovery
			} else {
				configs = append(configs, entry.conf)
			}
		}
	}

	return configPkg{confs: configs, defaults: defaultConfigs, metrics: metricConfigs}
}

// GetCheckConfigFromFile returns an instance of check.Config if `fpath` points to a valid config file
func GetCheckConfigFromFile(name, fpath string) (check.Config, error) {
	cf := configFormat{}
	config := check.Config{Name: name}

	// Read file contents
	// FIXME: ReadFile reads the entire file, possible security implications
	yamlFile, err := ioutil.ReadFile(fpath)
	if err != nil {
		return config, err
	}

	// Parse configuration
	err = yaml.Unmarshal(yamlFile, &cf)
	if err != nil {
		return config, err
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

	// Copy auto discovery identifiers
	config.ADIdentifiers = cf.ADIdentifiers

	// If logs was found, add it to the config
	if cf.LogsConfig != nil {
		rawLogsConfig, _ := yaml.Marshal(cf.LogsConfig)
		config.LogsConfig = rawLogsConfig
	}

	return config, err
}
