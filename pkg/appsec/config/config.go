package config

import (
	"fmt"
	"net/url"

	"github.com/pkg/errors"
)

// AgentConfig represents the agent configuration API.
type AgentConfig interface {
	SetKnown(s string)
	BindEnvAndSetDefault(key string, v interface{}, env ...string)
	GetBool(key string) bool
	GetString(key string) string
	GetInt(key string) int
}

const (
	// intakeURLTemplate specifies the string template allowing to obtain the
	// intake URL from a given site.
	defaultIntakeURLTemplate = "https://appsecevts-http-intake.logs.%s/v1/input"
	defaultSite              = "datad0g.com"
)

// InitConfig initializes the config defaults on a config
func InitConfig(cfg AgentConfig) {
	cfg.SetKnown("appsec_config.enabled")
	cfg.SetKnown("appsec_config.appsec_dd_url")

	cfg.BindEnvAndSetDefault("appsec_config.enabled", true, "DD_APPSEC_ENABLED")
	cfg.BindEnvAndSetDefault("appsec_config.appsec_dd_url", "", "DD_APPSEC_DD_URL")
}

// Config handles the interpretation of the configuration. It is also a simple
// structure to share across all the components, with 100% safe and reliable
// values.
type Config struct {
	Enabled bool

	// Intake API URL
	IntakeURL *url.URL
	APIKey    string
}

// FromAgentConfig creates and returns the AppSec config from the overall agent
// config.
func FromAgentConfig(cfg AgentConfig) (*Config, error) {
	intakeURL, err := intakeURL(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "config")
	}
	return &Config{
		Enabled:   cfg.GetBool("appsec_config.enabled"),
		IntakeURL: intakeURL,
		APIKey:    cfg.GetString("api_key"),
	}, nil
}

// intakeURL returns the appsec intake URL.
func intakeURL(cfg AgentConfig) (*url.URL, error) {
	var main string
	if url := cfg.GetString("appsec_config.appsec_dd_url"); url != "" {
		main = url
	} else {
		site := cfg.GetString("site")
		if site == "" {
			site = defaultSite
		}
		main = fmt.Sprintf(defaultIntakeURLTemplate, site)
	}
	u, err := url.Parse(main)
	if err != nil {
		return nil, errors.Wrapf(err, "error while parsing the appsec intake API URL %s", main)
	}
	return u, nil
}
