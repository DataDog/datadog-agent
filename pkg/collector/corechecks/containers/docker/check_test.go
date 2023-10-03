// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	taggerUtils "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	dockerUtil "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func TestDockerCheckGenericPart(t *testing.T) {
	// Creating mocks
	containersMeta := []*workloadmeta.Container{
		// Container with full stats
		generic.CreateContainerMeta("docker", "cID100"),
		// Should never been called as we are in the Docker check
		generic.CreateContainerMeta("containerd", "cID101"),
	}

	containersStats := map[string]mock.ContainerEntry{
		"cID100": mock.GetFullSampleContainerEntry(),
		"cID101": mock.GetFullSampleContainerEntry(),
	}

	// Inject mock processor in check
	mockSender, processor, _ := generic.CreateTestProcessor(containersMeta, containersStats, metricsAdapter{}, getProcessorFilter(nil))
	processor.RegisterExtension("docker-custom-metrics", &dockerCustomMetricsExtension{})

	// Create Docker check
	check := DockerCheck{
		instance: &DockerConfig{
			CollectExitCodes:   true,
			CollectImagesStats: true,
			CollectImageSize:   true,
			CollectDiskStats:   true,
			CollectVolumeCount: true,
			CollectEvent:       true,
		},
		processor:      *processor,
		dockerHostname: "testhostname",
	}

	err := check.runProcessor(mockSender)
	assert.NoError(t, err)

	expectedTags := []string{"runtime:docker"}
	mockSender.AssertNumberOfCalls(t, "Rate", 13)
	mockSender.AssertNumberOfCalls(t, "Gauge", 16)

	mockSender.AssertMetricInRange(t, "Gauge", "docker.uptime", 0, 600, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "docker.cpu.usage", 1e-5, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "docker.cpu.user", 3e-5, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "docker.cpu.system", 2e-5, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "docker.cpu.throttled.time", 1e-5, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "docker.cpu.throttled", 0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.cpu.limit", 50, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.cpu.shares", 400, "", expectedTags)

	mockSender.AssertMetric(t, "Gauge", "docker.kmem.usage", 40, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.mem.limit", 42000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.mem.soft_limit", 40000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.mem.rss", 300, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.mem.cache", 200, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.mem.working_set", 350, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.mem.swap", 0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.mem.failed_count", 10, "", expectedTags)
	mockSender.AssertMetricInRange(t, "Gauge", "docker.mem.in_use", 0, 1, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.mem.sw_limit", 500, "", expectedTags)

	expectedFooTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/foo", "device_name:/dev/foo")
	mockSender.AssertMetric(t, "Rate", "docker.io.read_bytes", 100, "", expectedFooTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_operations", 10, "", expectedFooTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_bytes", 200, "", expectedFooTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_operations", 20, "", expectedFooTags)
	expectedBarTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/bar", "device_name:/dev/bar")
	mockSender.AssertMetric(t, "Rate", "docker.io.read_bytes", 100, "", expectedBarTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.read_operations", 10, "", expectedBarTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_bytes", 200, "", expectedBarTags)
	mockSender.AssertMetric(t, "Rate", "docker.io.write_operations", 20, "", expectedBarTags)

	mockSender.AssertMetric(t, "Gauge", "docker.thread.count", 10, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.thread.limit", 20, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.container.open_fds", 200, "", expectedTags)
}

func TestDockerCustomPart(t *testing.T) {
	// Mocksender
	mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
	mockSender.SetupAcceptAll()

	fakeTagger := local.NewFakeTagger()
	fakeTagger.SetTags("container_id://e2d5394a5321d4a59497f53552a0131b2aafe64faba37f4738e78c531289fc45", "foo", []string{"image_name:datadog/agent", "short:agent", "tag:latest"}, nil, nil, nil)
	fakeTagger.SetTags("container_id://b781900d227cf8d63a0922705018b66610f789644bf236cb72c8698b31383074", "foo", []string{"image_name:datadog/agent", "short:agent", "tag:7.32.0-rc.1"}, nil, nil, nil)
	fakeTagger.SetTags("container_id://be2584a7d1a2a3ae9f9c688e9ce7a88991c028507fec7c70a660b705bd2a5b90", "foo", []string{"app:foo"}, nil, nil, nil)
	fakeTagger.SetTags("container_id://be2584a7d1a2a3ae9f9c688e9ce7a88991c028507fec7c70a660b705bd2a5b91", "foo", []string{"excluded:true"}, nil, nil, nil)
	tagger.SetDefaultTagger(fakeTagger)

	// Mock client + fake data
	dockerClient := dockerUtil.MockClient{}
	dockerClient.FakeContainerList = []dockerTypes.Container{
		{
			ID:      "e2d5394a5321d4a59497f53552a0131b2aafe64faba37f4738e78c531289fc45",
			Names:   []string{"agent"},
			Image:   "datadog/agent",
			ImageID: "sha256:7e813d42985b2e5a0269f868aaf238ffc952a877fba964f55aa1ff35fd0bf5f6",
			Labels: map[string]string{
				"io.kubernetes.pod.namespace": "kubens",
			},
			State:      string(workloadmeta.ContainerStatusRunning),
			SizeRw:     100,
			SizeRootFs: 200,
		},
		{
			ID:      "b781900d227cf8d63a0922705018b66610f789644bf236cb72c8698b31383074",
			Names:   []string{"agent2"},
			Image:   "datadog/agent:7.32.0-rc.1",
			ImageID: "sha256:c7e83cf0566432c24ed909f52ea16c29281f6f30d0a27855e15ff79376efaed0", // Image missing in mapping
			State:   string(workloadmeta.ContainerStatusRunning),
		},
		{
			ID:      "be2584a7d1a2a3ae9f9c688e9ce7a88991c028507fec7c70a660b705bd2a5b90",
			Names:   []string{"agent3"},
			Image:   "sha256:e575decbf7f4b920edabf5c86f948da776ffa26b5ceed591668ad6086c08a87f",
			ImageID: "sha256:e575decbf7f4b920edabf5c86f948da776ffa26b5ceed591668ad6086c08a87f",
			State:   string(workloadmeta.ContainerStatusRunning),
		},
		{
			ID:      "be2584a7d1a2a3ae9f9c688e9ce7a88991c028507fec7c70a660b705bd2a5b91",
			Names:   []string{"agent-excluded"},
			Image:   "sha256:e575decbf7f4b920edabf5c86f948da776ffa26b5ceed591668ad6086c08a87f",
			ImageID: "sha256:e575decbf7f4b920edabf5c86f948da776ffa26b5ceed591668ad6086c08a87f",
			State:   string(workloadmeta.ContainerStatusRunning),
		},
		{
			ID:      "e2d5394a5321d4a59497f53552a0131b2aafe64faba37f4738e78c531289fc45",
			Names:   []string{"agent-dead"},
			Image:   "datadog/agent",
			ImageID: "sha256:7e813d42985b2e5a0269f868aaf238ffc952a877fba964f55aa1ff35fd0bf5f6",
			Labels: map[string]string{
				"io.kubernetes.pod.namespace": "kubens",
			},
			State:  "dead",
			SizeRw: 100,
		},
	}
	dockerClient.FakeAttachedVolumes = 10
	dockerClient.FakeDandlingVolumes = 2
	dockerClient.FakeImageNameMapping = map[string]string{
		"sha256:7e813d42985b2e5a0269f868aaf238ffc952a877fba964f55aa1ff35fd0bf5f6": "datadog/agent:latest",
		"sha256:e575decbf7f4b920edabf5c86f948da776ffa26b5ceed591668ad6086c08a87f": "sha256:e575decbf7f4b920edabf5c86f948da776ffa26b5ceed591668ad6086c08a87f",
	}
	dockerClient.FakeImages = []dockerTypes.ImageSummary{
		{
			ID:          "sha256:7e813d42985b2e5a0269f868aaf238ffc952a877fba964f55aa1ff35fd0bf5f6",
			Size:        50,
			VirtualSize: 100,
		},
		{
			ID:          "sha256:e575decbf7f4b920edabf5c86f948da776ffa26b5ceed591668ad6086c08a87f",
			Size:        100,
			VirtualSize: 200,
		},
		{
			ID:          "sha256:c7e83cf0566432c24ed909f52ea16c29281f6f30d0a27855e15ff79376efaed0",
			Size:        200,
			VirtualSize: 400,
		},
	}

	// Create Docker check
	check := DockerCheck{
		instance: &DockerConfig{
			CollectExitCodes:   true,
			CollectImagesStats: true,
			CollectImageSize:   true,
			CollectDiskStats:   true,
			CollectVolumeCount: true,
			CollectEvent:       true,
		},
		eventTransformer: newBundledTransformer("testhostname", []string{}),
		dockerHostname:   "testhostname",
		containerFilter: &containers.Filter{
			Enabled:         true,
			NameExcludeList: []*regexp.Regexp{regexp.MustCompile("agent-excluded")},
		},
	}

	err := check.runDockerCustom(mockSender, &dockerClient, dockerClient.FakeContainerList)
	assert.NoError(t, err)

	mockSender.AssertNumberOfCalls(t, "Gauge", 14)

	mockSender.AssertMetric(t, "Gauge", "docker.container.size_rw", 100, "", []string{"image_name:datadog/agent", "short:agent", "tag:latest"})
	mockSender.AssertMetric(t, "Gauge", "docker.container.size_rootfs", 200, "", []string{"image_name:datadog/agent", "short:agent", "tag:latest"})

	mockSender.AssertMetric(t, "Gauge", "docker.containers.running", 1, "", []string{"image_name:datadog/agent", "short:agent", "tag:latest"})
	mockSender.AssertMetric(t, "Gauge", "docker.containers.running", 1, "", []string{"image_name:datadog/agent", "short:agent", "tag:7.32.0-rc.1"})
	mockSender.AssertMetric(t, "Gauge", "docker.containers.running", 1, "", []string{"app:foo"})
	mockSender.AssertMetric(t, "Gauge", "docker.containers.stopped", 1, "", []string{"docker_image:datadog/agent:latest", "image_name:datadog/agent", "image_tag:latest", "short_image:agent"})

	mockSender.AssertMetric(t, "Gauge", "docker.containers.running.total", 4, "", nil)
	mockSender.AssertMetric(t, "Gauge", "docker.containers.stopped.total", 1, "", nil)

	// Tags between `docker.containers.running` and `docker.image.*` may be different because `docker.image.*` never uses the tagger
	// while `docker.container.*` may use the tagger if the container is running.
	mockSender.AssertMetric(t, "Gauge", "docker.image.virtual_size", 100, "", []string{"docker_image:datadog/agent:latest", "image_name:datadog/agent", "image_tag:latest", "short_image:agent"})
	mockSender.AssertMetric(t, "Gauge", "docker.image.size", 50, "", []string{"docker_image:datadog/agent:latest", "image_name:datadog/agent", "image_tag:latest", "short_image:agent"})
	mockSender.AssertMetric(t, "Gauge", "docker.images.available", 3, "", nil)
	mockSender.AssertMetric(t, "Gauge", "docker.images.intermediate", 0, "", nil)

	mockSender.AssertMetric(t, "Gauge", "docker.volume.count", 10, "", []string{"volume_state:attached"})
	mockSender.AssertMetric(t, "Gauge", "docker.volume.count", 2, "", []string{"volume_state:dangling"})

	mockSender.AssertServiceCheck(t, DockerServiceUp, servicecheck.ServiceCheckOK, "", nil, "")
}

func TestContainersRunning(t *testing.T) {
	mockSender := mocksender.NewMockSender(checkid.ID(t.Name()))
	mockSender.SetupAcceptAll()

	// Define tags for 3 different containers. The first 2 have the same tags.
	// The third one shares the image-related tags, but has a different
	// "service" tag.
	fakeTagger := local.NewFakeTagger()
	fakeTagger.SetTags("container_id://e2d5394a5321d4a59497f53552a0131b2aafe64faba37f4738e78c531289fc45", "foo", []string{"image_name:datadog/agent", "short:agent", "tag:latest", "service:s1"}, nil, nil, nil)
	fakeTagger.SetTags("container_id://b781900d227cf8d63a0922705018b66610f789644bf236cb72c8698b31383074", "foo", []string{"image_name:datadog/agent", "short:agent", "tag:latest", "service:s1"}, nil, nil, nil)
	fakeTagger.SetTags("container_id://be2584a7d1a2a3ae9f9c688e9ce7a88991c028507fec7c70a660b705bd2a5b90", "foo", []string{"image_name:datadog/agent", "short:agent", "tag:latest", "service:s2"}, nil, nil, nil)
	tagger.SetDefaultTagger(fakeTagger)

	// Image ID is shared by the 3 containers
	imageID := "sha256:7e813d42985b2e5a0269f868aaf238ffc952a877fba964f55aa1ff35fd0bf5f6"

	// Mock client + fake data
	dockerClient := dockerUtil.MockClient{}
	dockerClient.FakeContainerList = []dockerTypes.Container{
		{
			ID:      "e2d5394a5321d4a59497f53552a0131b2aafe64faba37f4738e78c531289fc45",
			Names:   []string{"agent"},
			Image:   "datadog/agent",
			ImageID: imageID,
			State:   string(workloadmeta.ContainerStatusRunning),
		},
		{
			ID:      "b781900d227cf8d63a0922705018b66610f789644bf236cb72c8698b31383074",
			Names:   []string{"agent"},
			Image:   "datadog/agent",
			ImageID: imageID,
			State:   string(workloadmeta.ContainerStatusRunning),
		},
		{
			ID:      "be2584a7d1a2a3ae9f9c688e9ce7a88991c028507fec7c70a660b705bd2a5b90",
			Names:   []string{"agent"},
			Image:   "datadog/agent",
			ImageID: imageID,
			State:   string(workloadmeta.ContainerStatusRunning),
		},
	}

	// Create Docker check
	check := DockerCheck{
		instance:        &DockerConfig{},
		dockerHostname:  "testhostname",
		containerFilter: &containers.Filter{},
	}

	err := check.runDockerCustom(mockSender, &dockerClient, dockerClient.FakeContainerList)
	assert.NoError(t, err)

	// Containers that share the same set of tags should be reported together,
	// but containers that only share some tags should not be reported together

	mockSender.AssertMetric(t, "Gauge", "docker.containers.running", 2, "", []string{"image_name:datadog/agent", "short:agent", "tag:latest", "service:s1"})
	mockSender.AssertMetric(t, "Gauge", "docker.containers.running", 1, "", []string{"image_name:datadog/agent", "short:agent", "tag:latest", "service:s2"})
}

func TestProcess_CPUSharesMetric(t *testing.T) {
	containersMeta := []*workloadmeta.Container{
		generic.CreateContainerMeta("docker", "cID100"),
		generic.CreateContainerMeta("docker", "cID101"),
		generic.CreateContainerMeta("docker", "cID102"),
	}

	containersStats := map[string]mock.ContainerEntry{
		"cID100": { // container with CPU shares (cgroups v1)
			ContainerStats: &metrics.ContainerStats{
				CPU: &metrics.ContainerCPUStats{
					Shares: pointer.Ptr(1024.0),
				},
			},
		},
		"cID101": { // container with CPU weight (cgroups v2)
			ContainerStats: &metrics.ContainerStats{
				CPU: &metrics.ContainerCPUStats{
					Weight: pointer.Ptr(100.0), // 2597 shares
				},
			},
		},
		"cID102": { // shares/weight not available
			ContainerStats: &metrics.ContainerStats{
				CPU: &metrics.ContainerCPUStats{
					Total: pointer.Ptr(100.0),
				},
			},
		},
	}

	// Inject mock processor in check
	mockSender, processor, _ := generic.CreateTestProcessor(containersMeta, containersStats, metricsAdapter{}, getProcessorFilter(nil))
	processor.RegisterExtension("docker-custom-metrics", &dockerCustomMetricsExtension{})

	// Create Docker check
	check := DockerCheck{
		instance:       &DockerConfig{},
		processor:      *processor,
		dockerHostname: "testhostname",
	}

	err := check.runProcessor(mockSender)
	assert.NoError(t, err)

	expectedTags := []string{"runtime:docker"}

	mockSender.AssertMetricInRange(t, "Gauge", "docker.uptime", 0, 600, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.cpu.shares", 1024, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "docker.cpu.shares", 2597, "", expectedTags)
	mockSender.AssertNotCalled(t, "Gauge", "docker.cpu.shares", 0.0, "", mocksender.MatchTagsContains(expectedTags))
}
