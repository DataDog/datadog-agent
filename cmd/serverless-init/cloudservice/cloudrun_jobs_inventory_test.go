// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestCloudRunJobsInventoryOffMetadataCaching(t *testing.T) {
	for _, available := range []bool{false, true} {
		t.Run(fmt.Sprintf("metadata=%t", available), func(t *testing.T) {
			conf := configmock.New(t)
			conf.Set("serverless.inventory_enabled", false, model.SourceAgentRuntime)
			t.Setenv(cloudRunJobNameEnvVar, "job")
			t.Setenv(cloudRunExecutionEnvVar, "execution")
			t.Setenv(cloudRunTaskIndexEnvVar, "1")
			t.Setenv(cloudRunTaskAttemptEnvVar, "2")
			t.Setenv(cloudRunTaskCountEnvVar, "3")
			service := &CloudRunJobs{}
			metadata := map[string]string{projectID: "", location: "", containerID: ""}
			if available {
				metadata = map[string]string{projectID: "project", location: "region", containerID: "instance"}
			}
			saved := metadataHelperFunc
			t.Cleanup(func() { metadataHelperFunc = saved })
			calls := 0
			metadataHelperFunc = func(_ *GCPConfig, kind CloudRunType) map[string]string {
				calls++
				assert.Equal(t, CloudRunJob, kind)
				return metadata
			}

			first := service.GetTags()
			first[projectID] = "mutated-by-caller"
			second := service.GetTags()
			assert.Equal(t, metadata[projectID], second[projectID])
			assert.NotContains(t, metadata, "origin", "tag enrichment must not mutate cached metadata")
			assert.Equal(t, CloudRunJobsOrigin, second["origin"])
			assert.Equal(t, "job", second[cloudRunJobTagPrefix+jobNameTag])
			assert.Equal(t, "execution", second[cloudRunJobTagPrefix+executionNameTag])
			assert.Equal(t, "1", second[cloudRunJobTagPrefix+taskIndexTag])
			assert.Equal(t, "2", second[cloudRunJobTagPrefix+taskAttemptTag])
			assert.Equal(t, "3", second[cloudRunJobTagPrefix+taskCountTag])
			if available {
				assert.Equal(t, "projects/project/locations/region/jobs/job", second[cloudRunJobTagPrefix+resourceNameTag])
			} else {
				assert.NotContains(t, second, cloudRunJobTagPrefix+resourceNameTag)
				assert.Equal(t, "unknown", service.GetEnhancedMetricTags(second).Base[projectID])
			}
			service.GetInventoryData()
			assert.Equal(t, 1, calls, "inventory and telemetry share the cached lookup, including failures")
			assert.False(t, conf.GetBool("serverless.inventory_enabled"))
		})
	}
}
