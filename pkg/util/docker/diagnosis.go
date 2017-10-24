// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"

	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("Docker availability", new(dockerAvailabilityDiagnosis))
}

type dockerAvailabilityDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *dockerAvailabilityDiagnosis) Diagnose() error {
	_, err := ConnectToDocker()
	if err != nil {
		log.Error(err)
	}
	return err
}
