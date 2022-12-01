// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

import (
	"fmt"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func buildKeysPerDomains(conf config.Config) (map[string][]string, error) {
	mainURL := config.GetMainEndpointWithConfig(conf, "https://contimage-intake.", "container_image.dd_url")
	if _, err := url.Parse(mainURL); err != nil {
		return nil, fmt.Errorf("could not parse contimage main endpoint: %w", err)
	}

	keysPerDomain := map[string][]string{
		mainURL: {
			conf.GetString("api_key"),
		},
	}

	if !conf.IsSet("container_image.additional_endpoints") {
		return keysPerDomain, nil
	}

	additionalEndpoints := conf.GetStringMapStringSlice("container_image.additional_endpoints")

	return config.MergeAdditionalEndpoints(keysPerDomain, additionalEndpoints)
}

// NewForwarder returns a forwarder for container lifecycle events
func NewForwarder() *forwarder.DefaultForwarder {
	if !config.Datadog.GetBool("container_image.enabled") {
		return nil
	}

	if flavor.GetFlavor() != flavor.DefaultAgent {
		return nil
	}

	keysPerDomain, err := buildKeysPerDomains(config.Datadog)
	if err != nil {
		log.Errorf("Cannot build keys per domains: %v", err)
		return nil
	}

	options := forwarder.NewOptionsWithResolvers(resolver.NewSingleDomainResolvers(keysPerDomain))

	return forwarder.NewDefaultForwarder(options)
}
