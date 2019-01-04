// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// start various subservices (apm, logs, process) based on the config file settings

// IsEnabled checks to see if a given service should be started
func (s *Servicedef) IsEnabled() bool {
	return config.Datadog.GetBool(s.configKey)
}

func startDependentServices() {
	for _, svc := range subservices {
		if svc.IsEnabled() {
			log.Debugf("Attempting to start service: %s", svc.name)
			err := svc.Start()
			if err != nil {
				log.Warnf("Failed to start services %s: %s", svc.name, err.Error())
			} else {
				log.Debugf("Started service %s", svc.name)
			}
		} else {
			log.Infof("Service %s is disabled, not starting", svc.name)
		}
	}
}
