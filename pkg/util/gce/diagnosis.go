// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package gce

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("GCE Metadata availability", diagnose)
}

// diagnose the GCE metadata API availability
func diagnose() error {
	_, err := GetHostname()
	if err != nil {
		log.Error(err)
	}
	return err
}
