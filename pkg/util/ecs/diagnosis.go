// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package ecs

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	log "github.com/cihub/seelog"
)

func init() {
	diagnosis.Register("ECS Metadata availability", diagnoseECS)
	diagnosis.Register("ECS Fargate Metadata availability", diagnoseFargate)
}

// diagnose the ECS metadata API availability
func diagnoseECS() error {
	_, err := GetTasks()
	if err != nil {
		log.Error(err)
	}
	return err
}

// diagnose the ECS Fargate metadata API availability
func diagnoseFargate() error {
	_, err := GetTaskMetadata()
	if err != nil {
		log.Error(err)
	}
	return err
}
