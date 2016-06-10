package loader

import (
	"io/ioutil"
	"path/filepath"

	"github.com/op/go-logging"

	"gopkg.in/yaml.v2"
)

var log = logging.MustGetLogger("datadog-agent")

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
func (c *FileConfigProvider) Collect() ([]CheckConfig, error) {
	configs := []CheckConfig{}

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

// getCheckConfig returns an instance of CheckConfig if `fpath` points to a valid config file
func getCheckConfig(name, fpath string) (CheckConfig, error) {
	conf := CheckConfig{Name: name}

	// Read file contents
	// FIXME: ReadFile reads the entire file, possible security implications
	yamlFile, err := ioutil.ReadFile(fpath)
	if err != nil {
		return conf, err
	}

	// Parse configuration
	err = yaml.Unmarshal(yamlFile, &conf.Data)
	return conf, err
}
