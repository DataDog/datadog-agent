// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

func TestDockerRecordsFromInspect(t *testing.T) {
	testCases := []struct {
		co                   *types.ContainerJSON
		toRecordEnvAsTags    map[string]string
		toRecordLabelsAsTags map[string]string
		expectedLow          []string
		expectedHigh         []string
	}{
		{
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v"},
					Labels: map[string]string{"labelKey": "labelValue"},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow:          []string{},
			expectedHigh:         []string{},
		},
		{
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v"},
					Labels: map[string]string{"labelKey": "labelKey"},
				},
			},
			toRecordEnvAsTags:    map[string]string{"k": "becomeK"},
			toRecordLabelsAsTags: map[string]string{"labelKey": "labelValue"},
			expectedLow:          []string{"becomeK:v"},
			expectedHigh:         []string{},
		},
		{
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v"},
					Labels: map[string]string{"labelKey": "labelKey"},
				},
			},
			toRecordEnvAsTags:    map[string]string{"k": "+becomeK"},
			toRecordLabelsAsTags: map[string]string{"labelKey": "labelValue"},
			expectedLow:          []string{},
			expectedHigh:         []string{"becomeK:v"},
		},
		{
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v", "l=t"},
					Labels: map[string]string{"labelKey": "labelValue"},
				},
			},
			toRecordEnvAsTags:    map[string]string{"k": "+becomeK", "l": "expectedLow"},
			toRecordLabelsAsTags: map[string]string{"labelkey": "labelKey"},
			expectedLow:          []string{"expectedLow:t", "labelKey:labelValue"},
			expectedHigh:         []string{"becomeK:v"},
		},
		{
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env:    []string{"k=v", "l=t"},
					Labels: map[string]string{"labelKey": "labelValue"},
				},
			},
			toRecordEnvAsTags:    map[string]string{"k": "+becomeK", "l": "expectedLow"},
			toRecordLabelsAsTags: map[string]string{"labelkey": "+labelKey"},
			expectedLow:          []string{"expectedLow:t"},
			expectedHigh:         []string{"becomeK:v", "labelKey:labelValue"},
		},
		{
			co: &types.ContainerJSON{
				Config: &container.Config{
					Env: []string{
						"MARATHON_APP_ID=1",
						"CHRONOS_JOB_NAME=2",
						"CHRONOS_JOB_OWNER=3",
						"MESOS_TASK_ID=4",
					},
					Labels: map[string]string{},
				},
			},
			toRecordEnvAsTags:    map[string]string{},
			toRecordLabelsAsTags: map[string]string{},
			expectedLow: []string{
				"marathon_app:1",
				"chronos_job:2",
				"chronos_job_owner:3",
			},
			expectedHigh: []string{"mesos_task:4"},
		},
	}

	dc := &DockerCollector{}
	for i, test := range testCases {
		dc.envAsTags = test.toRecordEnvAsTags
		dc.labelsAsTags = test.toRecordLabelsAsTags
		tags := utils.NewTagList()
		dc.recordEnvVariableFromInspect(tags, test.co.Config.Env)
		dc.recordLabelsFromInspect(tags, test.co.Config.Labels)
		low, high := tags.Compute()

		// Low card tags
		assert.Equal(t, len(test.expectedLow), len(low), "test case %d", i)
		for _, lt := range test.expectedLow {
			assert.Contains(t, low, lt, "test case %d", i)
		}

		// High card tags
		assert.True(t, len(test.expectedHigh) == len(high))
		for _, ht := range test.expectedHigh {
			assert.Contains(t, high, ht, "test case %d", i)
		}
	}
}
