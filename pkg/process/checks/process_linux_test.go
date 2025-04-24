// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"fmt"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/stretchr/testify/assert"
)

func TestProcessIsECSFargatePidModeSetToTaskLinux(t *testing.T) {
	for _, tc := range []struct {
		description    string
		containers     []*model.Container
		fargateEnabled bool
		expected       bool
	}{
		{
			description: "ecs linux fargate pidMode not set to task",
			containers: []*model.Container{
				{
					Tags: []string{
						fmt.Sprintf("%s:%s", tags.EcsContainerName, "other container"),
						fmt.Sprintf("%s:%s", tags.ContainerName, "aws-fargate-pause"),
						fmt.Sprintf("%s:%s", tags.ImageName, "aws-fargate-pause"),
					},
				},
			},
			fargateEnabled: true,
			expected:       false,
		},
		{
			description: "ecs linux fargate pidMode set to task",
			containers: []*model.Container{
				{
					Tags: []string{
						fmt.Sprintf("%s:%s", tags.EcsContainerName, "aws-fargate-pause"),
						fmt.Sprintf("%s:%s", tags.ContainerName, "some container name"),
						fmt.Sprintf("%s:%s", tags.ImageName, "some image name"),
					},
				},
			},
			fargateEnabled: true,
			expected:       true,
		},
		{
			description: "ecs linux fargate pidMode task container exists but not on fargate",
			containers: []*model.Container{
				{
					Tags: []string{
						fmt.Sprintf("%s:%s", tags.EcsContainerName, "aws-fargate-pause"),
						fmt.Sprintf("%s:%s", tags.ContainerName, "some container name"),
						fmt.Sprintf("%s:%s", tags.ImageName, "some image name"),
					},
				},
			},
			fargateEnabled: false,
			expected:       false,
		},
		{
			description: "ecs linux fargate pidMode task container does not exist and not on fargate",
			containers: []*model.Container{
				{
					Tags: []string{
						fmt.Sprintf("%s:%s", tags.EcsContainerName, "some container name"),
						fmt.Sprintf("%s:%s", tags.ContainerName, "some container name"),
						fmt.Sprintf("%s:%s", tags.ImageName, "some image name"),
					},
				},
			},
			fargateEnabled: false,
			expected:       false,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			if tc.fargateEnabled {
				env.SetFeatures(t, env.ECSFargate)
			} else {
				env.ClearFeatures()
			}
			assert.Equal(t, tc.expected, isECSLinuxFargatePidModeSetToTask(tc.containers))
		})
	}
}
