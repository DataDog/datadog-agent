package appsec

import (
	"fmt"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/pkg/errors"
)

const (
	// defaultIntakeURLTemplate specifies the string template allowing to obtain
	// the intake URL from a given site.
	defaultIntakeURLTemplate = "https://appsecevts-http-intake.logs.%s/v1/input"
	// defaultSite is the intake site applied by default to the default intake
	// URL template defaultIntakeURLTemplate
	defaultSite = "datad0g.com"
)

// Config handles the interpretation of the configuration. It is also a simple
// structure to share across all the components, with 100% safe and reliable
// values.
type Config struct {
	Enabled bool

	// Intake API URL
	IntakeURL *url.URL
	APIKey    string
}

// newConfig creates and returns the AppSec config from the overall agent
// config.
func newConfig(cfg config.Config) (*Config, error) {
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
func intakeURL(cfg config.Config) (*url.URL, error) {
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
