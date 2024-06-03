package impl

import (
	"errors"
	"strings"

	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
)

const (
	httpConfigKey = "http"
)

var (
	errMissingProtocol      = errors.New("must specify at least one protocol")
	errHTTPEndpointRequired = errors.New("http endpoint required")
	errInvalidPath          = errors.New("path must start with /")
)

// Config has the configuration for the extension enabling the health check
// extension, used to report the health status of the service.
type Config struct {
	// HTTPConfig is v2 config for the http healthcheck service.
	HTTPConfig *confighttp.ServerConfig `mapstructure:"http,squash"`
}

// Validate checks if the extension configuration is valid
func (c *Config) Validate() error {

	if c.HTTPConfig != nil {
		if c.HTTPConfig.Endpoint == "" {
			return errHTTPEndpointRequired
		}
		if c.HTTPConfig.Status.Enabled && !strings.HasPrefix(c.HTTPConfig.Status.Path, "/") {
			return errInvalidPath
		}
		if c.HTTPConfig.Config.Enabled && !strings.HasPrefix(c.HTTPConfig.Config.Path, "/") {
			return errInvalidPath
		}
	}

	return nil
}

// Unmarshal a confmap.Conf into the config struct.
func (c *Config) Unmarshal(conf *confmap.Conf) error {
	err := conf.Unmarshal(c)
	if err != nil {
		return err
	}

	if !conf.IsSet(httpConfigKey) {
		c.HTTPConfig = nil
	}

	if !conf.IsSet(grpcConfigKey) {
		c.GRPCConfig = nil
	}

	return nil
}
