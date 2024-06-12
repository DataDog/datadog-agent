package impl

import (
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
)

type extractDebugEndpoint func(conf *confmap.Conf) (string, bool, error)

var (
	errHTTPEndpointRequired  = errors.New("http endpoint required")
	supportedDebugExtensions = map[string]extractDebugEndpoint{
		"health_check": healthExtractEndpoint,
		"zpages":       zPagesExtractEndpoint,
		"pprof":        pprofExtractEndpoint,
	}
)

// Config has the configuration for the extension enabling the health check
// extension, used to report the health status of the service.
type Config struct {
	HTTPConfig *confighttp.ServerConfig `mapstructure:",squash"`

	Converter confmap.Converter
}

var _ component.Config = (*Config)(nil)

// Validate checks if the extension configuration is valid
func (c *Config) Validate() error {

	if c.HTTPConfig == nil || c.HTTPConfig.Endpoint == "" {
		return errHTTPEndpointRequired
	}

	return nil
}

// Unmarshal a confmap.Conf into the config struct.
func (c *Config) Unmarshal(conf *confmap.Conf) error {
	err := conf.Unmarshal(c)
	if err != nil {
		return err
	}

	return nil
}

func zPagesExtractEndpoint(c *confmap.Conf) (string, bool, error) {
	endpoint, err := regularStringEndpointExtractor(c)
	return endpoint, true, err
}

func pprofExtractEndpoint(c *confmap.Conf) (string, bool, error) {
	endpoint, err := regularStringEndpointExtractor(c)
	return endpoint, false, err
}

func healthExtractEndpoint(c *confmap.Conf) (string, bool, error) {
	endpoint, err := regularStringEndpointExtractor(c)
	return endpoint, false, err
}

func regularStringEndpointExtractor(c *confmap.Conf) (string, error) {
	if c == nil {
		return "", fmt.Errorf("nil confmap - skipping")
	}

	element := c.Get("endpoint")
	if element == nil {
		return "", fmt.Errorf("Expected endpoint conf element, but none found")
	}

	endpoint, ok := element.(string)
	if !ok {
		return "", fmt.Errorf("endpoint conf element was unexpectedly not a string")
	}
	return endpoint, nil
}
