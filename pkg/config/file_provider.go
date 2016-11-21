package config

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	log "github.com/cihub/seelog"
)

const configFileName = "datadog.yaml"

// FileProvider retrieves configuration data from text files on the
// filesystem containing YAML code.
type FileProvider struct {
	searchPaths []string
}

// NewFileProvider returns a provider for configuration files. It will
// search for `configFileName` within the given path.
func NewFileProvider(paths []string) *FileProvider {
	return &FileProvider{searchPaths: paths}
}

// Configure tries to open the configuration file, read YAML data and
// unmarshal it into the Config instance. It tries from different
// locations and exits at the first occurence.
func (p *FileProvider) Configure(config *Config) error {
	for _, path := range p.searchPaths {
		configFilePath := filepath.Join(path, configFileName)
		yamlData, err := ioutil.ReadFile(configFilePath)
		if err == nil {
			log.Infof("Found configuration file: %s", configFilePath)
			return config.FromYAML(yamlData)
		}
	}
	return fmt.Errorf("Unable to find a valid config file in any of the paths: %v", p.searchPaths)
}

func (p *FileProvider) String() string {
	return "File Provider"
}
