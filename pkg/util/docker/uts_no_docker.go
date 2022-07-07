// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker
// +build !docker

package docker

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// GetAgentUTSMode retrieves from Docker the UTS mode of the Agent container
func GetAgentUTSMode(context.Context) (containers.UTSMode, error) {
	return containers.UnknownUTSMode, nil
}
