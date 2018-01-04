// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package ec2

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("EC2 Metadata availability", diagnose)
}

// diagnose the ec2 metadata API availability
func diagnose() error {
	_, err := GetHostname()
	if err != nil {
		log.Error(err)
	}
	return err
}
