// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/configresolver"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	cache "github.com/patrickmn/go-cache"
	"gopkg.in/yaml.v2"
)

type configFormat struct {
	ADIdentifiers           []string                           `yaml:"ad_identifiers"`
	AdvancedADIdentifiers   []integration.AdvancedADIdentifier `yaml:"advanced_ad_identifiers"`
	ClusterCheck            bool                               `yaml:"cluster_check"`
	InitConfig              interface{}                        `yaml:"init_config"`
	MetricConfig            interface{}                        `yaml:"jmx_metrics"`
	LogsConfig              interface{}                        `yaml:"logs"`
	Instances               []integration.RawMap
	DockerImages            []string `yaml:"docker_images"`             // Only imported for deprecation warning
	IgnoreAutodiscoveryTags bool     `yaml:"ignore_autodiscovery_tags"` // Use to ignore tags coming from autodiscovery
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

var reader *configFilesReader

type configFilesReader struct {
	paths []string
	cache *cache.Cache
	sync.Mutex
}

var doOnce sync.Once

// InitConfigFilesReader initializes the config files reader.
// It reads all configs and caches them in memory for 5 minutes.
// InitConfigFilesReader should be called at agent startup.
func InitConfigFilesReader(paths []string) {
	doOnce.Do(func() {
		if reader == nil {
			reader = &configFilesReader{
				paths: paths,
				cache: cache.New(5*time.Minute, 30*time.Second),
			}
		}

		reader.readAndCacheAll()
	})
}

// FilterFunc is used by ReadConfigFiles to filter integration configs.
type FilterFunc func(integration.Config) bool

// GetAll makes ReadConfigFiles return all the configurations found.
var GetAll FilterFunc = func(c integration.Config) bool { return true }

// WithAdvancedADOnly makes ReadConfigFiles return the configurations with AdvancedADIdentifiers only.
var WithAdvancedADOnly FilterFunc = func(c integration.Config) bool { return len(c.AdvancedADIdentifiers) > 0 }

// WithoutAdvancedAD makes ReadConfigFiles return the all configurations except the ones with AdvancedADIdentifiers.
var WithoutAdvancedAD FilterFunc = func(c integration.Config) bool { return len(c.AdvancedADIdentifiers) == 0 }

// ReadConfigFiles returns integration configs read from config files, a mapping integration config error strings and an error.
// The filter argument allows returing a subset of configs depending on the caller preferences.
// InitConfigFilesReader should be called at agent startup before this function
// to setup the config paths and cache the configs.
func ReadConfigFiles(keep FilterFunc) ([]integration.Config, map[string]string, error) {
	if reader == nil {
		return nil, nil, errors.New("cannot read config files: reader not initialized")
	}

	reader.Lock()
	defer reader.Unlock()

	cachedConfigs, foundConfigs := reader.cache.Get("configs")
	cachedErrors, foundErrors := reader.cache.Get("errors")
	if !foundConfigs || !foundErrors {
		// Cache expired
		reader.readAndCacheAll()
	}

	configs, ok := cachedConfigs.([]integration.Config)
	if !ok {
		return nil, nil, errors.New("couldn't cast cached configs from cache")
	}

	errs, ok := cachedErrors.(map[string]string)
	if !ok {
		return nil, nil, errors.New("couldn't cast cached config errors from cache")
	}

	return filterConfigs(configs, keep), errs, nil
}

func filterConfigs(configs []integration.Config, keep FilterFunc) []integration.Config {
	filteredConfigs := []integration.Config{}
	for _, config := range configs {
		if keep(config) {
			filteredConfigs = append(filteredConfigs, config)
		}
	}

	return filteredConfigs
}

func (r *configFilesReader) readAndCacheAll() {
	configs, errors := r.read(GetAll)
	reader.cache.SetDefault("configs", configs)
	reader.cache.SetDefault("errors", errors)
}

// read scans paths searching for configuration files. When found,
// it parses the files and try to unmarshall Yaml contents into integration.Config instances.
func (r *configFilesReader) read(keep FilterFunc) ([]integration.Config, map[string]string) {
	integrationErrors := map[string]string{}
	configs := []integration.Config{}
	configNames := make(map[string]struct{}) // use this map as a python set
	defaultConfigs := []integration.Config{}

	for _, path := range r.paths {
		log.Infof("Searching for configuration files at: %s", path)

		entries, err := readDirPtr(path)
		if err != nil {
			log.Warnf("Skipping, %s", err)
			continue
		}

		for _, fileEntry := range entries {
			// We support only one level of nesting for check configs
			if fileEntry.IsDir() {
				var dirConfigs configPkg
				dirConfigs, integrationErrors = collectDir(path, fileEntry, integrationErrors)
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
			var entry configEntry
			entry, integrationErrors = collectEntry(fileEntry, path, "", integrationErrors)
			// we don't collect metric files from the root dir (which check is it for? that's nonsensical!)
			if entry.err != nil || entry.isMetric {
				// logging is handled in collectEntry
				continue
			}

			if !keep(entry.conf) {
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

	return configs, integrationErrors
}

// collectEntry collects a file entry and return it's configuration if valid
// the integrationName can be manually provided else it'll use the filename
func collectEntry(file os.FileInfo, path string, integrationName string, integrationErrors map[string]string) (configEntry, map[string]string) {
	const defaultExt string = ".default"
	fileName := file.Name()
	ext := filepath.Ext(fileName)
	entry := configEntry{}
	absPath := filepath.Join(path, fileName)

	// skip auto conf files based on the agent configuration
	if fileName == "auto_conf.yaml" && containsString(config.Datadog.GetStringSlice("ignore_autoconf"), integrationName) {
		log.Infof("Skipping 'auto_conf.yaml' for integration '%s'", integrationName)
		entry.err = fmt.Errorf("'auto_conf.yaml' for integration '%s' is skipped", integrationName)
		return entry, integrationErrors
	}

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
		return entry, integrationErrors
	}

	var err error
	entry.conf, err = GetIntegrationConfigFromFile(integrationName, absPath)
	if err != nil {
		log.Warnf("%s is not a valid config file: %s", absPath, err)
		integrationErrors[integrationName] = err.Error()
		entry.err = errors.New("Invalid config file format")
		return entry, integrationErrors
	}

	// if logs is the only integration, set isLogsOnly to true
	if entry.conf.LogsConfig != nil && entry.conf.MetricConfig == nil && len(entry.conf.Instances) == 0 && len(entry.conf.ADIdentifiers) == 0 {
		entry.isLogsOnly = true
	}

	delete(integrationErrors, integrationName) // noop if entry is nonexistant
	log.Debug("Found valid configuration in file:", absPath)
	return entry, integrationErrors
}

func collectDir(parentPath string, folder os.FileInfo, integrationErrors map[string]string) (configPkg, map[string]string) {
	configs := []integration.Config{}
	defaultConfigs := []integration.Config{}
	otherConfigs := []integration.Config{}
	const dirExt string = ".d"
	dirPath := filepath.Join(parentPath, folder.Name())

	if filepath.Ext(folder.Name()) != dirExt {
		// the name of this directory isn't in the form `integrationName.d`, skip it
		log.Debugf("Not a config folder, skipping directory: %s", dirPath)
		return configPkg{configs, defaultConfigs, otherConfigs}, integrationErrors
	}

	// search for yaml files within this directory
	subEntries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		log.Warnf("Skipping config directory %s: %s", dirPath, err)
		return configPkg{configs, defaultConfigs, otherConfigs}, integrationErrors
	}

	// strip the trailing `.d`
	integrationName := strings.TrimSuffix(folder.Name(), dirExt)

	// try to load any config file in it
	for _, sEntry := range subEntries {
		if !sEntry.IsDir() {
			var entry configEntry
			entry, integrationErrors = collectEntry(sEntry, dirPath, integrationName, integrationErrors)
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

	return configPkg{confs: configs, defaults: defaultConfigs, others: otherConfigs}, integrationErrors
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
	config.AdvancedADIdentifiers = cf.AdvancedADIdentifiers

	// Copy cluster_check status
	config.ClusterCheck = cf.ClusterCheck

	// Copy ignore_autodiscovery_tags parameter
	config.IgnoreAutodiscoveryTags = cf.IgnoreAutodiscoveryTags

	// DockerImages entry was found: we ignore it if no ADIdentifiers has been found
	if len(cf.DockerImages) > 0 && len(cf.ADIdentifiers) == 0 {
		return config, errors.New("the 'docker_images' section is deprecated, please use 'ad_identifiers' instead")
	}

	// Interpolate env vars. Returns an error a variable wasn't subsituted, ignore it.
	_ = configresolver.SubstituteTemplateEnvVars(&config)

	config.Source = "file:" + fpath

	return config, err
}

func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// ResetReader is only for unit tests
func ResetReader(paths []string) {
	reader = &configFilesReader{
		paths: paths,
		cache: cache.New(5*time.Minute, 30*time.Second),
	}

	reader.readAndCacheAll()
}
