package tests

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config instance contains ConfigParams and StackParams
type Config struct {
	ConfigParams ConfigParams                 `yaml:"configParams"`
	StackParams  map[string]map[string]string `yaml:"stackParams"`
}

// ConfigParams instance contains config relayed parameters
type ConfigParams struct {
	AWS       AWS    `yaml:"aws"`
	Agent     Agent  `yaml:"agent"`
	OutputDir string `yaml:"outputDir"`
	Pulumi    Pulumi `yaml:"pulumi"`
	DevMode   string `yaml:"devMode"`
}

// AWS instance contains AWS related parameters
type AWS struct {
	Account            string `yaml:"account"`
	KeyPairName        string `yaml:"keyPairName"`
	PublicKeyPath      string `yaml:"publicKeyPath"`
	PrivateKeyPath     string `yaml:"privateKeyPath"`
	PrivateKeyPassword string `yaml:"privateKeyPassword"`
	TeamTag            string `yaml:"teamTag"`
}

// Agent instance contains agent related parameters
type Agent struct {
	APIKey              string `yaml:"apiKey"`
	APPKey              string `yaml:"appKey"`
	VerifyCodeSignature string `yaml:"verifyCodeSignature"`
}

// Pulumi instance contains pulumi related parameters
type Pulumi struct {
	// Sets the log level for Pulumi operations
	// Be careful setting this value, as it can expose sensitive information in the logs.
	// https://www.pulumi.com/docs/support/troubleshooting/#verbose-logging
	LogLevel string `yaml:"logLevel"`
	// By default pulumi logs to /tmp, and creates symlinks to the most recent log, e.g. /tmp/pulumi.INFO
	// Set this option to true to log to stderr instead.
	// https://www.pulumi.com/docs/support/troubleshooting/#verbose-logging
	LogToStdErr string `yaml:"logToStdErr"`
	// To reduce logs noise in the CI, by default we display only the Pulumi error progress steam.
	// Set this option to true to display all the progress streams.
	VerboseProgressStreams string `yaml:"verboseProgressStreams"`
}

func LoadConfig(path string) (Config, error) {
	config := Config{}
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	err = yaml.Unmarshal(content, &config)
	return config, err
}
