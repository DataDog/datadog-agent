package loader

import (
	"io/ioutil"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/check"
	"github.com/op/go-logging"

	"gopkg.in/yaml.v2"
)

var log = logging.MustGetLogger("datadog-agent")

type configFormat struct {
	Instances []check.ConfigRawMap
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

	for _, path := range c.paths {
		log.Debug("Searching for yaml files at:", path)

		files, err := ioutil.ReadDir(path)
		if err != nil {
			log.Warningf("Unable to access dir: %s, skipping...", err)
			continue
		}

		for _, f := range files {
			if f.IsDir() {
				log.Warningf("%s is a dir, skipping...", f.Name())
				continue
			}

			fName := f.Name()
			extName := filepath.Ext(fName)
			bName := fName[:len(f.Name())-len(extName)]
			conf, err := getCheckConfig(bName, filepath.Join(path, fName))
			if err != nil {
				log.Warningf("%s is not a valid config file: %s", f.Name(), err)
				continue
			}

			log.Debug("Found valid configuration in file:", f.Name())
			configs = append(configs, conf)
		}
	}

	return configs, nil
}

// getCheckConfig returns an instance of check.Config if `fpath` points to a valid config file
func getCheckConfig(name, fpath string) (check.Config, error) {
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

	// Go through instances and return corresponding []byte
	for _, instance := range cf.Instances {
		rawConf, e := yaml.Marshal(instance)
		if e != nil {
			log.Warningf("Unable to unmarshal config: %v", e)
			continue
		}
		config.Instances = append(config.Instances, rawConf)
	}

	return config, err
}
