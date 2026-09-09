// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
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

// TestInitializeDaemonModeV1Unavailable covers daemon-mode startup when the metadata v1
// introspection endpoint cannot be used. ECS Managed Instances must still start by routing
// to the v4 /tasks endpoint, while EC2 must keep failing because every EC2 daemon parser
// reads the task list from v1.
func TestInitializeDaemonModeV1Unavailable(t *testing.T) {
	getInstanceErr := errors.New("connection refused")

	tests := []struct {
		name                  string
		launchType            workloadmeta.ECSLaunchType
		taskCollectionEnabled bool
		hasMetaV4             bool
		// v1Client is returned by the ecsMetaV1 seam; nil means v1 client init fails.
		v1Client           *fakev1EcsClient
		expectErr          bool
		expectParserSuffix string
	}{
		{
			name:                  "managed instances without v1 client uses v4 /tasks",
			launchType:            workloadmeta.ECSLaunchTypeManagedInstances,
			taskCollectionEnabled: true,
			hasMetaV4:             true,
			v1Client:              nil,
			expectParserSuffix:    "parseTasksFromV4TasksEndpoint",
		},
		{
			name:                  "managed instances with failing GetInstance uses v4 /tasks",
			launchType:            workloadmeta.ECSLaunchTypeManagedInstances,
			taskCollectionEnabled: true,
			hasMetaV4:             true,
			v1Client: &fakev1EcsClient{
				mockGetInstance: func(context.Context) (*v1.Instance, error) { return nil, getInstanceErr },
			},
			expectParserSuffix: "parseTasksFromV4TasksEndpoint",
		},
		{
			name:                  "managed instances without v1 and without metaV4 fails",
			launchType:            workloadmeta.ECSLaunchTypeManagedInstances,
			taskCollectionEnabled: true,
			hasMetaV4:             false,
			v1Client:              nil,
			expectErr:             true,
		},
		{
			name:                  "managed instances without v1 and task collection disabled fails",
			launchType:            workloadmeta.ECSLaunchTypeManagedInstances,
			taskCollectionEnabled: false,
			hasMetaV4:             true,
			v1Client:              nil,
			expectErr:             true,
		},
		{
			name:                  "ec2 without v1 client fails",
			launchType:            workloadmeta.ECSLaunchTypeEC2,
			taskCollectionEnabled: true,
			hasMetaV4:             true,
			v1Client:              nil,
			expectErr:             true,
		},
		{
			name:                  "ec2 with failing GetInstance fails instead of leaving a nil parser",
			launchType:            workloadmeta.ECSLaunchTypeEC2,
			taskCollectionEnabled: true,
			hasMetaV4:             true,
			v1Client: &fakev1EcsClient{
				mockGetInstance: func(context.Context) (*v1.Instance, error) { return nil, getInstanceErr },
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The v4 /tasks route additionally requires the v4 env var to be present.
			t.Setenv(v3or4.DefaultMetadataURIv4EnvVariable, "http://169.254.170.2/v4")

			restore := stubDaemonMetadataSeams(t, tt.v1Client, tt.hasMetaV4)
			defer restore()

			c := &collector{
				config:                config.NewMockWithOverrides(t, map[string]interface{}{}),
				taskCollectionEnabled: tt.taskCollectionEnabled,
				actualLaunchType:      tt.launchType,
			}

			err := c.initializeDaemonMode(context.Background())

			if tt.expectErr {
				require.Error(t, err)
				// workloadmeta only keeps a collector in its candidate set when
				// retry.IsErrWillRetry matches, and that helper type-asserts instead of
				// unwrapping. A non-retriable error here would drop the ECS collector
				// permanently instead of retrying Start.
				assert.True(t, retry.IsErrWillRetry(err),
					"startup error must be retriable, got %T: %v", err, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, c.taskCollectionParser, "a nil parser would panic on the first Pull")
			assert.Contains(t, taskParserName(c.taskCollectionParser), tt.expectParserSuffix)
		})
	}

}

// TestInitializeDaemonModeV1Available verifies that a reachable v1 endpoint still drives
// parser selection and populates the instance-derived fields, unchanged for EC2.
func TestInitializeDaemonModeV1Available(t *testing.T) {
	t.Setenv(v3or4.DefaultMetadataURIv4EnvVariable, "http://169.254.170.2/v4")

	v1Client := &fakev1EcsClient{
		mockGetInstance: func(context.Context) (*v1.Instance, error) {
			return &v1.Instance{
				Cluster:              "my-cluster",
				ContainerInstanceARN: "arn:aws:ecs:us-east-1:123456789012:container-instance/my-cluster/abc123",
				Version:              "Amazon ECS Agent - v1.54.0 (abc1234)",
			}, nil
		},
	}

	restore := stubDaemonMetadataSeams(t, v1Client, false)
	defer restore()

	c := &collector{
		config:                config.NewMockWithOverrides(t, map[string]interface{}{}),
		taskCollectionEnabled: true,
		actualLaunchType:      workloadmeta.ECSLaunchTypeEC2,
	}

	require.NoError(t, c.initializeDaemonMode(context.Background()))
	assert.Equal(t, "my-cluster", c.clusterName)
	assert.Equal(t, "arn:aws:ecs:us-east-1:123456789012:container-instance/my-cluster/abc123", c.containerInstanceARN)
	require.NotNil(t, c.taskCollectionParser)
	assert.Contains(t, taskParserName(c.taskCollectionParser), "parseTasksFromV4Endpoint")
}

// stubDaemonMetadataSeams replaces the metadata helpers that initializeDaemonMode calls so
// tests do not depend on memoized global clients or network I/O. A nil v1Client makes the
// v1 client initialization fail.
func stubDaemonMetadataSeams(t *testing.T, v1Client *fakev1EcsClient, hasMetaV4 bool) func() {
	t.Helper()

	origV1, origV4, origTags := ecsMetaV1, ecsMetaV4FromCurrentTask, ecsHasEC2ResourceTags

	ecsMetaV1 = func() (v1.Client, error) {
		if v1Client == nil {
			return nil, errors.New("temporary failure in ecsutil-meta-v1")
		}
		return v1Client, nil
	}
	ecsMetaV4FromCurrentTask = func() (v3or4.Client, error) {
		if !hasMetaV4 {
			return nil, errors.New("v4 metadata endpoint not available")
		}
		return &fakev3or4EcsClient{}, nil
	}
	ecsHasEC2ResourceTags = func() bool { return false }

	return func() {
		ecsMetaV1, ecsMetaV4FromCurrentTask, ecsHasEC2ResourceTags = origV1, origV4, origTags
	}
}
