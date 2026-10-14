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

func TestCloudRunInventoryOffMetadataCaching(t *testing.T) {
	for _, isFunction := range []bool{false, true} {
		for _, available := range []bool{false, true} {
			t.Run(fmt.Sprintf("function=%t/metadata=%t", isFunction, available), func(t *testing.T) {
				conf := configmock.New(t)
				conf.Set("serverless.inventory_enabled", false, model.SourceAgentRuntime)
				t.Setenv(ServiceNameEnvVar, "service")
				t.Setenv(revisionNameEnvVar, "revision")
				t.Setenv(functionTargetEnvVar, "handler")
				service := &CloudRun{isFunction: isFunction}
				prefix := cloudRunServiceTagPrefix
				kind := CloudRunService
				if isFunction {
					prefix = cloudRunFunctionTagPrefix
					kind = CloudRunFunction
				}
				metadata := map[string]string{projectID: "", location: "", containerID: ""}
				if available {
					metadata = map[string]string{projectID: "project", location: "region", containerID: "instance"}
				}
				saved := metadataHelperFunc
				t.Cleanup(func() { metadataHelperFunc = saved })
				calls := 0
				metadataHelperFunc = func(_ *GCPConfig, gotKind CloudRunType) map[string]string {
					calls++
					assert.Equal(t, kind, gotKind)
					return metadata
				}

				first := service.GetTags()
				first[projectID] = "mutated-by-caller"
				second := service.GetTags()
				assert.Equal(t, metadata[projectID], second[projectID])
				assert.NotContains(t, metadata, "origin", "tag enrichment must not mutate cached metadata")
				assert.Equal(t, CloudRunOrigin, second["origin"])
				assert.Equal(t, "service", second[prefix+serviceName])
				assert.Equal(t, "revision", second[prefix+revisionName])
				if available {
					resource := "projects/project/locations/region/services/service"
					if isFunction {
						resource += "/functions/handler"
					}
					assert.Equal(t, resource, second[prefix+resourceName])
				} else {
					assert.NotContains(t, second, prefix+resourceName)
					assert.Equal(t, "unknown", service.GetEnhancedMetricTags(second).Base[projectID])
				}
				service.GetInventoryData()
				assert.Equal(t, 1, calls, "inventory and telemetry share the cached lookup, including failures")
				assert.False(t, conf.GetBool("serverless.inventory_enabled"))
			})
		}
	}
}
