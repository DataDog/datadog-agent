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
	defaultIntakeURLTemplate = "https://appsecevts-http-intake.logs.%s"
	// defaultSite is the default intake site.
	defaultSite = "datadoghq.com"
)

// Config handles the interpretation of the configuration. It is also a simple
// structure to share across all the components, with 100% safe and reliable
// values.
type Config struct {
	Enabled bool

	// API configuration group
	IntakeURL      *url.URL
	APIKey         string
	MaxPayloadSize int64
}

// newConfig creates and returns the AppSec config from the overall agent
// config.
func newConfig(cfg config.Config) (*Config, error) {
	intakeURL, err := intakeURL(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "config")
	}
	return &Config{
		Enabled:        cfg.GetBool("appsec_config.enabled"),
		IntakeURL:      intakeURL,
		APIKey:         cfg.GetString("api_key"),
		MaxPayloadSize: cfg.GetInt64("appsec_config.max_payload_size"),
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
