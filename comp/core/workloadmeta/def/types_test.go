// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

func TestNewContainerImage(t *testing.T) {
	tests := []struct {
		name                      string
		imageName                 string
		expectedWorkloadmetaImage ContainerImage
		expectsErr                bool
	}{
		{
			name:      "image with tag",
			imageName: "datadog/agent:7",
			expectedWorkloadmetaImage: ContainerImage{
				RawName:   "datadog/agent:7",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "7",
				ID:        "0",
			},
		}, {
			name:      "image without tag",
			imageName: "datadog/agent",
			expectedWorkloadmetaImage: ContainerImage{
				RawName:   "datadog/agent",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "latest", // Default to latest when there's no tag
				ID:        "1",
			},
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			image, err := NewContainerImage(strconv.Itoa(i), test.imageName)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedWorkloadmetaImage, image)
		})
	}
}

func TestECSTaskString(t *testing.T) {
	task := ECSTask{
		EntityID: EntityID{
			Kind: KindECSTask,
			ID:   "task-1-id",
		},
		EntityMeta: EntityMeta{
			Name: "task-1",
		},
		Containers: []OrchestratorContainer{
			{
				ID:   "container-1-id",
				Name: "container-1",
				Image: ContainerImage{
					RawName:   "datadog/agent:7",
					Name:      "datadog/agent",
					ShortName: "agent",
					Tag:       "7",
					ID:        "0",
				},
			},
		},
		Family:  "family-1",
		Version: "revision-1",
		EphemeralStorageMetrics: map[string]int64{
			"memory": 100,
			"cpu":    200,
		},
	}
	expected := `----------- Entity ID -----------
Kind: ecs_task ID: task-1-id
----------- Entity Meta -----------
Name: task-1
Namespace:
Annotations:
Labels:
----------- Containers -----------
Name: container-1
ID: container-1-id
Image: datadog/agent
----------- Resources -----------
----------- Task Info -----------
Tags:
Container Instance Tags:
Cluster Name:
Region:
Availability Zone:
Family: family-1
Version: revision-1
Launch Type:
AWS Account ID:
Desired Status:
Known Status:
VPC ID:
Ephemeral Storage Metrics: map[cpu:200 memory:100]
Limits: map[]
`
	compareTestOutput(t, expected, task.String(true))
}

func TestProcessString(t *testing.T) {
	creationTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		process  Process
		verbose  bool
		expected string
	}{
		{
			name: "basic process with minimal fields non-verbose",
			process: Process{
				EntityID: EntityID{
					Kind: KindProcess,
					ID:   "12345",
				},
				Pid:          12345,
				NsPid:        12345,
				Ppid:         1,
				Name:         "test-process",
				Cwd:          "/tmp",
				Exe:          "/usr/bin/test-process",
				Comm:         "test-process",
				Cmdline:      []string{"/usr/bin/test-process", "--flag"},
				Uids:         []int32{1000, 1001},
				Gids:         []int32{1002, 1003},
				ContainerID:  "container-123",
				CreationTime: creationTime,
			},
			verbose: false,
			expected: `----------- Entity ID -----------
PID: 12345
Name: test-process
Exe: /usr/bin/test-process
Cmdline: /usr/bin/test-process --flag
Namespace PID: 12345
Container ID: container-123
Creation time: 2023-01-01 12:00:00 +0000 UTC
APM Injection Status: unknown
`,
		},
		{
			name: "basic process with minimal fields verbose",
			process: Process{
				EntityID: EntityID{
					Kind: KindProcess,
					ID:   "12345",
				},
				Pid:          12345,
				NsPid:        12345,
				Ppid:         1,
				Name:         "test-process",
				Cwd:          "/tmp",
				Exe:          "/usr/bin/test-process",
				Comm:         "test-process",
				Cmdline:      []string{"/usr/bin/test-process", "--flag"},
				Uids:         []int32{1000, 1001},
				Gids:         []int32{1002, 1003},
				ContainerID:  "container-123",
				CreationTime: creationTime,
			},
			verbose: true,
			expected: `----------- Entity ID -----------
PID: 12345
Name: test-process
Exe: /usr/bin/test-process
Cmdline: /usr/bin/test-process --flag
Namespace PID: 12345
Container ID: container-123
Creation time: 2023-01-01 12:00:00 +0000 UTC
APM Injection Status: unknown
Comm: test-process
Cwd: /tmp
Uids: [1000 1001]
Gids: [1002 1003]
`,
		},
		{
			name: "process with language and service non-verbose",
			process: Process{
				EntityID: EntityID{
					Kind: KindProcess,
					ID:   "12345",
				},
				Pid:          12345,
				NsPid:        12345,
				Ppid:         1,
				Name:         "java-app",
				Cwd:          "/app",
				Exe:          "/usr/bin/java",
				Comm:         "java",
				Cmdline:      []string{"/usr/bin/java", "-jar", "app.jar"},
				Uids:         []int32{1000, 2, 3},
				Gids:         []int32{1001, 4, 5},
				ContainerID:  "container-999",
				CreationTime: creationTime,
				Language: &languagemodels.Language{
					Name:    languagemodels.Java,
					Version: "11.0.0",
				},
				Service: &Service{
					GeneratedName:            "java-app",
					GeneratedNameSource:      "binary_name",
					AdditionalGeneratedNames: []string{"java", "app"},
					TracerMetadata:           []tracermetadata.TracerMetadata{},
					TCPPorts:                 []uint16{8080, 8081},
					UDPPorts:                 []uint16{8082, 8083},
					APMInstrumentation:       true,
					Type:                     "web_service",
					LogFiles: []string{
						"/var/log/app_access.log",
						"/var/log/app_error.log",
					},
				},
			},
			verbose: false,
			expected: `----------- Entity ID -----------
PID: 12345
Name: java-app
Exe: /usr/bin/java
Cmdline: /usr/bin/java -jar app.jar
Namespace PID: 12345
Container ID: container-999
Creation time: 2023-01-01 12:00:00 +0000 UTC
Language: java
APM Injection Status: unknown
----------- Service Discovery -----------
Service Generated Name: java-app
`,
		},
		{
			name: "process with language and service verbose",
			process: Process{
				EntityID: EntityID{
					Kind: KindProcess,
					ID:   "12345",
				},
				Pid:          12345,
				NsPid:        12345,
				Ppid:         1,
				Name:         "java-app",
				Cwd:          "/app",
				Exe:          "/usr/bin/java",
				Comm:         "java",
				Cmdline:      []string{"/usr/bin/java", "-jar", "app.jar"},
				Uids:         []int32{1000, 2, 3},
				Gids:         []int32{1001, 4, 5},
				ContainerID:  "container-999",
				CreationTime: creationTime,
				Language: &languagemodels.Language{
					Name:    languagemodels.Java,
					Version: "11.0.0",
				},
				Service: &Service{
					GeneratedName:            "java-app",
					GeneratedNameSource:      "binary_name",
					AdditionalGeneratedNames: []string{"java", "app"},
					TracerMetadata:           []tracermetadata.TracerMetadata{},
					UST: UST{
						Service: "java-app",
						Env:     "production",
						Version: "1.2.3",
					},
					TCPPorts:           []uint16{8080, 8081},
					UDPPorts:           []uint16{8082, 8083},
					APMInstrumentation: true,
					Type:               "web_service",
					LogFiles: []string{
						"/var/log/app_access.log",
						"/var/log/app_error.log",
					},
				},
			},
			verbose: true,
			expected: `----------- Entity ID -----------
PID: 12345
Name: java-app
Exe: /usr/bin/java
Cmdline: /usr/bin/java -jar app.jar
Namespace PID: 12345
Container ID: container-999
Creation time: 2023-01-01 12:00:00 +0000 UTC
Language: java
APM Injection Status: unknown
Comm: java
Cwd: /app
Uids: [1000 2 3]
Gids: [1001 4 5]
----------- Service Discovery -----------
Service Generated Name: java-app
Service Generated Name Source: binary_name
Service Additional Generated Names: [java app]
Service Tracer Metadata: []
Service TCP Ports: [8080 8081]
Service UDP Ports: [8082 8083]
Service APM Instrumentation: true
Service Type: web_service
---- Unified Service Tagging ----
Service: java-app
Env: production
Version: 1.2.3
----------- Log Files -----------
/var/log/app_access.log
/var/log/app_error.log
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := test.process.String(test.verbose)
			compareTestOutput(t, test.expected, actual)
		})
	}
}

func compareTestOutput(t *testing.T, expected, actual string) {
	assert.Equal(t, strings.ReplaceAll(expected, " ", ""), strings.ReplaceAll(actual, " ", ""))
}

func TestMergeECSContainer(t *testing.T) {
	container1 := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "container-1-id",
		},
		EntityMeta: EntityMeta{
			Name: "container-1",
		},
		PID: 123,
		ECSContainer: &ECSContainer{
			DisplayName: "ecs-container-1",
		},
	}
	container2 := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "container-1-id",
		},
		EntityMeta: EntityMeta{
			Name: "container-1",
		},
	}

	err := container2.Merge(&container1)
	assert.NoError(t, err)
	assert.NotSame(t, container1.ECSContainer, container2.ECSContainer, "pointers of ECSContainer should not be equal")
	assert.EqualValues(t, container1.ECSContainer, container2.ECSContainer)

	container2.ECSContainer = nil
	err = container1.Merge(&container2)
	assert.NoError(t, err)
	assert.NotSame(t, container1.ECSContainer, container2.ECSContainer, "pointers of ECSContainer should not be equal")
	assert.Nil(t, container2.ECSContainer)
	assert.EqualValues(t, container1.ECSContainer.DisplayName, "ecs-container-1")
}

func TestMergeGPU(t *testing.T) {
	gpu1 := GPU{
		EntityID: EntityID{
			Kind: KindGPU,
			ID:   "gpu-1-id",
		},
		EntityMeta: EntityMeta{
			Name: "gpu-1",
		},
		Vendor:           "nvidia",
		DriverVersion:    "460.32.03",
		Device:           "",
		ActivePIDs:       []int{123, 456},
		ChildrenGPUUUIDs: []string{"gpu-2-id", "gpu-3-id"},
	}
	gpu2 := GPU{
		EntityID: EntityID{
			Kind: KindGPU,
			ID:   "gpu-1-id",
		},
		EntityMeta: EntityMeta{
			Name: "gpu-1",
		},
		Vendor:           "nvidia",
		DriverVersion:    "460.32.03",
		Device:           "tesla",
		ActivePIDs:       []int{654},
		ChildrenGPUUUIDs: []string{"gpu-4-id", "gpu-5-id"},
	}

	err := gpu1.Merge(&gpu2)
	assert.NoError(t, err)
	assert.Equal(t, gpu1.Device, "tesla")
	assert.ElementsMatch(t, gpu1.ActivePIDs, []int{654})
	assert.Equal(t, gpu1.Vendor, "nvidia")
	assert.ElementsMatch(t, gpu1.ChildrenGPUUUIDs, []string{"gpu-4-id", "gpu-5-id"})
}
