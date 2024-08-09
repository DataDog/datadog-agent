// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

// Package ecs provides information about the ECS Agent Version when running in ECS
package ecs

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
)

type ecsProvider struct{}

var metaV1 v1.Client
var err error

func GetECSAgentVersion(ctx context.Context) string {
	metaV1, err = metadata.V1()
	if err != nil {
		return ""
	}
	ecsInstance, err := metaV1.GetInstance(ctx)
	if err != nil {
		return ""
	}
	return ecsInstance.Version
}
