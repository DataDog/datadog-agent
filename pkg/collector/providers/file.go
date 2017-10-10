// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package providers

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"

	"gopkg.in/yaml.v2"
)

type configFormat struct {
	ADIdentifiers []string    `yaml:"ad_identifiers"`
	DockerImages  []string    `yaml:"docker_images"`
	InitConfig    interface{} `yaml:"init_config"`
	Instances     []check.ConfigRawMap
}

// FileConfigProvider collect configuration files from disk
type FileConfigProvider struct {
	paths []string
}

// NewFileConfigProvider creates a new FileConfigProvider searching for
// configuration files on the given paths
func NewFileConfigProvider(paths []string) *FileConfigProvider {
	return &FileConfigProvider{paths: paths}
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
			if entry.IsDir() {
				dirConfigs := collectDir(path, entry)
				if len(dirConfigs) > 0 {
					configs = append(configs, dirConfigs...)
					configNames[dirConfigs[0].Name] = struct{}{}
				}
				continue
			}

			entryName := entry.Name()
			checkName := entryName
			ext := filepath.Ext(entryName)
			isDefault := false

			// skip config files that are not of type:
			//  * check.yaml, check.yml
			//  * check.yaml.default, check.yml.default
			if ext == ".default" {
				// trim the .default suffix but preserve the real filename
				checkName = entryName[:len(entryName)-8]
				ext = filepath.Ext(checkName)
				isDefault = true
			}

			if ext != ".yaml" && ext != ".yml" {
				log.Debugf("Skipping file: %s", entry.Name())
				continue
			}

			checkName = checkName[:len(checkName)-len(ext)]
			conf, err := GetCheckConfigFromFile(checkName, filepath.Join(path, entry.Name()))
			if err != nil {
				log.Warnf("%s is not a valid config file: %s", entry.Name(), err)
				continue
			}
			log.Debug("Found valid configuration in file:", entry.Name())
			// determine if a check has to be run by default by
			// searching for check.yaml.default files
			if isDefault {
				defaultConfigs = append(defaultConfigs, conf)
			} else {
				configNames[conf.Name] = struct{}{}
				configs = append(configs, conf)
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

func (c *FileConfigProvider) String() string {
	return "File Configuration Provider"
}

func collectDir(parentPath string, folder os.FileInfo) []check.Config {
	configs := []check.Config{}

	if filepath.Ext(folder.Name()) != ".d" {
		// the name of this directory isn't in the form `checkname.d`, skip it
		log.Debugf("Not a config folder, skipping directory: %s", folder.Name())
		return configs
	}

	dirPath := filepath.Join(parentPath, folder.Name())

	// search for yaml files within this directory
	subEntries, err := ioutil.ReadDir(dirPath)
	if err != nil {
		log.Warnf("Skipping config directory: %s", err)
		return configs
	}

	// strip the trailing `.d`
	checkName := folder.Name()[:len(folder.Name())-2]

	// try to load any config file in it
	for _, sEntry := range subEntries {
		if !sEntry.IsDir() {
			filePath := filepath.Join(dirPath, sEntry.Name())
			conf, err := GetCheckConfigFromFile(checkName, filePath)
			if err != nil {
				log.Warnf("%s is not a valid config file: %s", sEntry.Name(), err)
				continue
			}
			log.Debug("Found valid configuration in file:", filePath)
			configs = append(configs, conf)
		}
	}

	return configs
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

	// If no valid instances were found, this is not a valid configuration file
	if len(cf.Instances) < 1 {
		return config, errors.New("Configuration file contains no valid instances")
	}

	// at this point the Yaml was already parsed, no need to check the error
	rawInitConfig, _ := yaml.Marshal(cf.InitConfig)
	config.InitConfig = rawInitConfig

	// Go through instances and return corresponding []byte
	for _, instance := range cf.Instances {
		// at this point the Yaml was already parsed, no need to check the error
		rawConf, _ := yaml.Marshal(instance)
		config.Instances = append(config.Instances, rawConf)
	}

	// Read AutoDiscovery data, try to use the old `docker_image` settings
	// param first
	if len(cf.DockerImages) > 0 {
		log.Warnf("'docker_image' section in %s is deprecated and will be eventually removed, use 'ad_identifiers' instead",
			fpath)
		config.ADIdentifiers = cf.DockerImages
	}

	// Override the legacy param with the new one, `ad_identifiers`
	if len(cf.ADIdentifiers) > 0 {
		if len(config.ADIdentifiers) > 0 {
			log.Warnf("Overwriting the deprecated 'docker_image' section from %s in favor of the new 'ad_identifiers' one",
				fpath)
		}
		config.ADIdentifiers = cf.ADIdentifiers
	}

	return config, err
}
