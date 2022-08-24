// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"fmt"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/pkg/errors"
)

const (
	// defaultIntakeURLTemplate specifies the string template allowing to obtain
	// the intake URL from a given site.
	defaultIntakeURLTemplate = "https://appsecevts-intake.%s"
	// defaultSite is the default intake site.
	defaultSite = "datadoghq.com"
	// defaultPayloadSize is the default HTTP payload size (ie. the body size) the proxy can copy and forward.
	defaultPayloadSize = 5 * 1024 * 1024
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
func newConfig(cfg *config.AgentConfig) (*Config, error) {
	intakeURL, err := intakeURL(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "config")
	}
	maxPayloadSize := cfg.AppSec.MaxPayloadSize
	if maxPayloadSize <= 0 {
		maxPayloadSize = defaultPayloadSize
	}
	return &Config{
		Enabled:        cfg.AppSec.Enabled,
		IntakeURL:      intakeURL,
		APIKey:         cfg.AppSec.APIKey,
		MaxPayloadSize: maxPayloadSize,
	}, nil
}

// intakeURL returns the appsec intake URL.
func intakeURL(cfg *config.AgentConfig) (*url.URL, error) {
	var main string
	if url := cfg.AppSec.DDURL; url != "" {
		main = url
	} else {
		site := cfg.Site
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
