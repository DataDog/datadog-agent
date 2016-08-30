package config

import (
	"io/ioutil"
	"path/filepath"

	"github.com/op/go-logging"
)

const configFileName = "datadog.conf"

var log = logging.MustGetLogger("datadog-agent")

// FileProvider retrieves configuration data from text files on the
// filesystem containing YAML code.
type FileProvider struct {
	searchPath string
}

// NewFileProvider returns a provider for configuration files. It will
// search for `configFileName` within the given path.
func NewFileProvider(path string) *FileProvider {
	return &FileProvider{searchPath: path}
}

// Configure tries to open the configuration file, read YAML data and
// unmarshal it into the Config instance.
func (p *FileProvider) Configure(config *Config) error {
	configFilePath := filepath.Join(p.searchPath, configFileName)
	yamlData, err := ioutil.ReadFile(configFilePath)

	if err != nil {
		log.Errorf("Unable to read config from file %s, skipping...", configFilePath)
		return err
	}
	return config.FromYAML(yamlData)
}
