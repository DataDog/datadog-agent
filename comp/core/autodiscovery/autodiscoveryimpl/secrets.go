// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func decryptConfig(conf integration.Config, secretResolver secrets.Component) (integration.Config, error) {
	if config.Datadog.GetBool("secret_backend_skip_checks") {
		log.Tracef("'secret_backend_skip_checks' is enabled, not decrypting configuration %q", conf.Name)
		return conf, nil
	}

	var err error

	// init_config
	conf.InitConfig, err = secretResolver.Resolve(conf.InitConfig, conf.Name)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets in 'init_config': %s", err)
	}

	// instances
	// we cannot update in place as, being a slice, it would modify the input config as well
	instances := make([]integration.Data, 0, len(conf.Instances))
	for _, inputInstance := range conf.Instances {
		decryptedInstance, err := secretResolver.Resolve(inputInstance, conf.Name)
		if err != nil {
			return conf, fmt.Errorf("error while decrypting secrets in an instance: %s", err)
		}
		instances = append(instances, decryptedInstance)
	}
	conf.Instances = instances

	// metrics
	conf.MetricConfig, err = secretResolver.Resolve(conf.MetricConfig, conf.Name)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets in 'metrics': %s", err)
	}

	// logs
	conf.LogsConfig, err = secretResolver.Resolve(conf.LogsConfig, conf.Name)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets 'logs': %s", err)
	}

	return conf, nil
}
