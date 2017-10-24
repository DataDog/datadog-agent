// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package gce

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("GCE Metadata availability", new(gceMetadataAvailabilityDiagnosis))
}

type gceMetadataAvailabilityDiagnosis struct{}

// Diagnose the docker availability on the system
func (dd *gceMetadataAvailabilityDiagnosis) Diagnose() error {
	_, err := GetHostname()
	if err != nil {
		log.Error(err)
	}
	return err
}
