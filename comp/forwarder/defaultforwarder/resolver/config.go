// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"fmt"
	"net/url"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// DomainSet represents a set of domains and their associated resolvers
type DomainSet = map[string]DomainResolver

// FromConfig creates a ResolverSet from the configuration.
//
// allowOPW specifies if observability_pipelines and vector config keys need to be honored.
func FromConfig(config config.Component, log log.Component, allowOPW bool) (DomainSet, error) {
	keysPerDomain, err := utils.GetMultipleEndpoints(config)
	if err != nil {
		return nil, fmt.Errorf("Misconfiguration of agent endpoints: %s", err)
	}

	resolvers, err := NewSingleDomainResolvers(keysPerDomain)
	if err != nil {
		return nil, err
	}

	if allowOPW {
		err := configureOPW(config, log, resolvers)
		if err != nil {
			return nil, err
		}
	}

	return resolvers, nil
}

func configureOPW(config config.Component, log log.Component, resolvers DomainSet) error {
	vectorMetricsURL, err := getObsPipelineURL(log, pkgconfigsetup.Metrics, config)
	if err != nil {
		log.Error("Misconfiguration of agent observability_pipelines_worker endpoint for metrics: ", err)
	}
	if r, ok := resolvers[utils.GetInfraEndpoint(config)]; ok && vectorMetricsURL != "" {
		log.Debugf("Configuring forwarder to send metrics to observability_pipelines_worker: %s", vectorMetricsURL)
		apiKeys, _ := r.GetAPIKeysInfo()
		resolver, err := NewDomainResolverWithMetricToVector(
			r.GetBaseDomain(),
			apiKeys,
			vectorMetricsURL,
		)
		if err != nil {
			return err
		}
		resolvers[utils.GetInfraEndpoint(config)] = resolver
	}

	return nil
}

// getObsPipelineURL returns the URL under the 'observability_pipelines_worker.' prefix for the given datatype
func getObsPipelineURL(log log.Component, datatype string, config config.Component) (string, error) {
	if config.GetBool(fmt.Sprintf("observability_pipelines_worker.%s.enabled", datatype)) {
		return getObsPipelineURLForPrefix(log, datatype, "observability_pipelines_worker", config)
	} else if config.GetBool(fmt.Sprintf("vector.%s.enabled", datatype)) {
		// Fallback to the `vector` config if observability_pipelines_worker is not set.
		return getObsPipelineURLForPrefix(log, datatype, "vector", config)
	}
	return "", nil
}

func getObsPipelineURLForPrefix(log log.Component, datatype string, prefix string, config config.Component) (string, error) {
	if config.GetBool(fmt.Sprintf("%s.%s.enabled", prefix, datatype)) {
		pipelineURL := config.GetString(fmt.Sprintf("%s.%s.url", prefix, datatype))
		if pipelineURL == "" {
			log.Errorf("%s.%s.enabled is set to true, but %s.%s.url is empty", prefix, datatype, prefix, datatype)
			return "", nil
		}
		_, err := url.Parse(pipelineURL)
		if err != nil {
			return "", fmt.Errorf("could not parse %s %s endpoint: %s", prefix, datatype, err)
		}
		return pipelineURL, nil
	}
	return "", nil
}
