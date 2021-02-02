// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker

package ecs

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
)

func init() {
	diagnosis.Register("ECS Metadata availability", diagnoseECS)
	diagnosis.Register("ECS Metadata with tags availability", diagnoseECSTags)
	diagnosis.Register("ECS Fargate Metadata availability", diagnoseFargate)
}

// diagnose the ECS metadata API availability
func diagnoseECS() error {
	client, err := ecsmeta.V1()
	if err != nil {
		log.Error(err)
		return err
	}
	log.Info("successfully detected ECS metadata server endpoint")

	if _, err = client.GetTasks(); err != nil {
		log.Error(err)
		return err
	}
	log.Info("successfully retrieved task list from ECS metadata server")

	return nil
}

// diagnose the ECS metadata with tags API availability
func diagnoseECSTags() error {
	client, err := ecsmeta.V3FromCurrentTask()
	if err != nil {
		log.Error(err)
		return err
	}
	log.Info("successfully detected ECS metadata server endpoint for resource tags")

	if _, err = client.GetTaskWithTags(); err != nil {
		log.Error(err)
		return err
	}
	log.Info("successfully retrieved task with potential tags from ECS metadata server")

	return nil
}

// diagnose the ECS Fargate metadata API availability
func diagnoseFargate() error {
	client, err := ecsmeta.V2()
	if err != nil {
		log.Debugf("error while initializing ECS metadata V2 client: %s", err)
		return err
	}

	if _, err := client.GetTask(); err != nil {
		log.Error(err)
		return err
	}
	log.Info("successfully retrieved task from Fargate metadata endpoint")

	return nil
}
