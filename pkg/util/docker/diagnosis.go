// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() {
	diagnosis.Register("Docker availability", diagnose)
}

// diagnose the docker availability on the system
func diagnose() error {
	_, err := GetDockerUtil()
	if err != nil {
		log.Error(err)
	} else {
		log.Info("successfully connected to docker")
	}

	hostname, err := HostnameProvider()
	if err != nil {
		log.Errorf("returned hostname %q with error: %s", hostname, err)
	} else {
		log.Infof("successfully got hostname %q from docker", hostname)
	}
	return err
}
