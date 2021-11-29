package telemetry

import (
	"fmt"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultSite = "datadoghq.com"
const telemetryURLTemplate = "https://instrumentation-telemetry-intake.%s/"

// IsEnabled checks if telmeetry proxy is enabled
func IsEnabled() bool {
	return config.Datadog.GetBool("apm_config.telemetry.enabled")
}

// BuildBaseTarget returns the URL of the main proxy target
func BuildBaseTarget(baseAPIKey string) (*Target, error) {
	site := config.Datadog.GetString("site")
	if site == "" {
		site = defaultSite
	}
	main := fmt.Sprintf(telemetryURLTemplate, site)
	if v := config.Datadog.GetString("apm_config.telemetry.dd_url"); v != "" {
		main = v
	}
	u, err := url.Parse(main)
	if err != nil {
		// if the main intake URL is invalid we don't use additional endpoints
		return nil, fmt.Errorf("error parsing main telemetry intake URL %s: %v", main, err)
	}

	return &Target{
		url:    u,
		apiKey: baseAPIKey,
	}, nil
}

// BuildAdditionalTargets returns a list of additional targets to send payloads to
// on best effort basis. In order to duplicate the data across many instances of DD.
func BuildAdditionalTargets() (targets []Target) {
	additionalEndpoitnsCfg := "apm_config.telemetry.additional_endpoints"

	if config.Datadog.IsSet(additionalEndpoitnsCfg) {
		extra := config.Datadog.GetStringMapStringSlice(additionalEndpoitnsCfg)
		for endpoint, keys := range extra {
			u, err := url.Parse(endpoint)
			if err != nil {
				log.Errorf("Error parsing additional telemetry intake URL %s: %v", endpoint, err)
				continue
			}
			for _, key := range keys {
				targets = append(targets, Target{
					url:    u,
					apiKey: key,
				})
			}
		}
	}
	return targets
}
