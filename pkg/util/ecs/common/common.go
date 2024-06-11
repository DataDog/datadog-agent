// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

// Package common provides common functionality for the different ECS clients.
package common

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// CloudProviderName contains the inventory name of for ECS
const CloudProviderName = "AWS"

// MetadataTimeout defines timeout for ECS metadata endpoints
func MetadataTimeout() time.Duration {
	return config.Datadog().GetDuration("ecs_metadata_timeout") * time.Millisecond
}
