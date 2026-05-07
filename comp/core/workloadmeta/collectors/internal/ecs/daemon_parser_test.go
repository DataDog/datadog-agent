// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

// taskParserName returns the name of the function backing the task parser for assertion.
func taskParserName(fn interface{}) string {
	if fn == nil {
		return ""
	}
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		return ""
	}
	pc := v.Pointer()
	if pc == 0 {
		return ""
	}
	f := runtime.FuncForPC(pc)
	if f == nil {
		return ""
	}
	return f.Name()
}

func TestSetTaskCollectionParserForDaemon(t *testing.T) {
	v1ParserSuffix := "parseTasksFromV1Endpoint"
	v4ParserSuffix := "parseTasksFromV4Endpoint"
	v4TasksParserSuffix := "parseTasksFromV4TasksEndpoint"

	tests := []struct {
		name                  string
		taskCollectionEnabled bool
		version               string
		setV4Env              bool
		actualLaunchType      workloadmeta.ECSLaunchType
		hasMetaV4             bool
		expectParserSuffix    string
	}{
		{
			name:                  "task collection disabled uses V1",
			taskCollectionEnabled: false,
			version:               "Amazon ECS Agent - v1.39.0 (abc1234)",
			expectParserSuffix:    v1ParserSuffix,
		},
		{
			// Use 1.54.0+ so the test passes on both Linux (min 1.39.0) and Windows (min 1.54.0)
			name:                  "task collection enabled with V4-capable version uses V4",
			taskCollectionEnabled: true,
			version:               "Amazon ECS Agent - v1.54.0 (abc1234)",
			actualLaunchType:      workloadmeta.ECSLaunchTypeEC2,
			expectParserSuffix:    v4ParserSuffix,
		},
		{
			name:                  "task collection enabled with version below V4 minimum uses V1",
			taskCollectionEnabled: true,
			version:               "Amazon ECS Agent - v1.30.0 (abc1234)",
			expectParserSuffix:    v1ParserSuffix,
		},
		{
			name:                  "task collection enabled with empty version and V4 env uses V4",
			taskCollectionEnabled: true,
			version:               "",
			setV4Env:              true,
			actualLaunchType:      workloadmeta.ECSLaunchTypeEC2,
			expectParserSuffix:    v4ParserSuffix,
		},
		{
			name:                  "task collection enabled with empty version and no V4 env uses V1",
			taskCollectionEnabled: true,
			version:               "",
			setV4Env:              false,
			expectParserSuffix:    v1ParserSuffix,
		},
		{
			name:                  "task collection enabled with invalid version and V4 env uses V4",
			taskCollectionEnabled: true,
			version:               "not-a-version",
			setV4Env:              true,
			actualLaunchType:      workloadmeta.ECSLaunchTypeEC2,
			expectParserSuffix:    v4ParserSuffix,
		},
		{
			name:                  "task collection enabled with invalid version and no V4 env uses V1",
			taskCollectionEnabled: true,
			version:               "not-a-version",
			setV4Env:              false,
			expectParserSuffix:    v1ParserSuffix,
		},
		{
			name:                  "managed instances with V4 env and metaV4 uses /tasks endpoint",
			taskCollectionEnabled: true,
			version:               "",
			setV4Env:              true,
			actualLaunchType:      workloadmeta.ECSLaunchTypeManagedInstances,
			hasMetaV4:             true,
			expectParserSuffix:    v4TasksParserSuffix,
		},
		{
			name:                  "managed instances with V4 env but no metaV4 falls back to per-task V4",
			taskCollectionEnabled: true,
			version:               "",
			setV4Env:              true,
			actualLaunchType:      workloadmeta.ECSLaunchTypeManagedInstances,
			hasMetaV4:             false,
			expectParserSuffix:    v4ParserSuffix,
		},
		{
			name:                  "managed instances with V4-capable version and metaV4 uses /tasks endpoint",
			taskCollectionEnabled: true,
			version:               "Amazon ECS Agent - v1.54.0 (abc1234)",
			actualLaunchType:      workloadmeta.ECSLaunchTypeManagedInstances,
			hasMetaV4:             true,
			expectParserSuffix:    v4TasksParserSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v4EnvVar := v3or4.DefaultMetadataURIv4EnvVariable
			if tt.setV4Env {
				t.Setenv(v4EnvVar, "http://169.254.170.2/v4")
			} else {
				oldVal, hadKey := os.LookupEnv(v4EnvVar)
				defer func() {
					if hadKey {
						os.Setenv(v4EnvVar, oldVal)
					} else {
						os.Unsetenv(v4EnvVar)
					}
				}()
				os.Unsetenv(v4EnvVar)
			}

			c := &collector{
				taskCollectionEnabled: tt.taskCollectionEnabled,
				actualLaunchType:      tt.actualLaunchType,
			}
			if tt.hasMetaV4 {
				c.metaV4 = &fakev3or4EcsClient{}
			}

			c.setTaskCollectionParserForDaemon(tt.version)

			require.NotNil(t, c.taskCollectionParser, "taskCollectionParser should be set")
			name := taskParserName(c.taskCollectionParser)
			require.NotEmpty(t, name, "parser function name should be resolvable")
			assert.Contains(t, name, tt.expectParserSuffix, "unexpected parser selected")
		})
	}
}
