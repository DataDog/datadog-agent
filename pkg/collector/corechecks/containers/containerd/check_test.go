// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	taggerUtils "github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
)

func TestToSnakeCaseConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Full",
			input:    "SomeMetricLabel",
			expected: "some_metric_label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := toSnakeCase(tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestContainerdCheckGenericPart(t *testing.T) {
	// Creating mocks
	containersMeta := []*workloadmeta.Container{
		// Container with full stats
		generic.CreateContainerMeta("containerd", "cID100"),
		// Should never been called as we are in the Docker check
		generic.CreateContainerMeta("docker", "cID101"),
	}
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	containersStats := map[string]mock.ContainerEntry{
		"cID100": mock.GetFullSampleContainerEntry(),
		"cID101": mock.GetFullSampleContainerEntry(),
	}

	// Inject mock processor in check
	mockSender, processor, _ := generic.CreateTestProcessor(containersMeta, containersStats, metricsAdapter{}, getProcessorFilter(nil, nil), fakeTagger)
	processor.RegisterExtension("containerd-custom-metrics", &containerdCustomMetricsExtension{})

	// Mock the containerd endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL)
		if r.URL.Path == "/v1/metrics" && r.Method == http.MethodGet {
			response := `
				grpc_server_handled_total{grpc_code="InvalidArgument",grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 0
				grpc_server_handled_total{grpc_code="NotFound",grpc_method="PullImage",grpc_service="runtime.v1.ImageService",grpc_type="unary"} 0
				grpc_server_handled_total{grpc_code="NotFound",grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 16559
				grpc_server_handled_total{grpc_code="OK",grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 72
			`
			w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	// Create Docker check
	check := ContainerdCheck{
		instance: &ContainerdConfig{
			CollectEvents:       true,
			OpenmetricsEndpoint: mockServer.URL,
		},
		processor: *processor,
	}

	err := check.runProcessor(mockSender)
	assert.NoError(t, err)

	expectedTags := []string{"runtime:containerd"}
	mockSender.AssertNumberOfCalls(t, "Rate", 13)
	mockSender.AssertNumberOfCalls(t, "Gauge", 11)

	mockSender.AssertMetricInRange(t, "Gauge", "containerd.uptime", 0, 600, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.total", 100, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.user", 300, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.system", 200, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.throttled.time", 100, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.throttled.periods", 0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.cpu.limit", 5e8, "", expectedTags)

	mockSender.AssertMetric(t, "Gauge", "containerd.mem.current.usage", 42000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.kernel.usage", 40, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.current.limit", 42000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.rss", 300, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.working_set", 350, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.cache", 200, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.swap.usage", 0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.current.failcnt", 10, "", expectedTags)

	fooReadTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/foo", "device_name:/dev/foo", "operation:read")
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.service_recursive_bytes", 100, "", fooReadTags)
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.serviced_recursive", 10, "", fooReadTags)
	fooWriteTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/foo", "device_name:/dev/foo", "operation:write")
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.service_recursive_bytes", 200, "", fooWriteTags)
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.serviced_recursive", 20, "", fooWriteTags)

	barReadTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/bar", "device_name:/dev/bar", "operation:read")
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.service_recursive_bytes", 100, "", barReadTags)
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.serviced_recursive", 10, "", barReadTags)
	barWriteTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/bar", "device_name:/dev/bar", "operation:write")
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.service_recursive_bytes", 200, "", barWriteTags)
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.serviced_recursive", 20, "", barWriteTags)

	check.scrapeOpenmetricsEndpoint(mockSender)
	mockSender.AssertMetric(t, "MonotonicCount", "containerd.image.pull", 72, "", []string{"grpc_service:runtime.v1alpha2.ImageService", "grpc_code:ok"})
	mockSender.AssertMetric(t, "MonotonicCount", "containerd.image.pull", 0, "", []string{"grpc_service:runtime.v1alpha2.ImageService", "grpc_code:invalid_argument"})
	mockSender.AssertMetric(t, "MonotonicCount", "containerd.image.pull", 0, "", []string{"grpc_service:runtime.v1.ImageService", "grpc_code:not_found"})
	mockSender.AssertMetric(t, "MonotonicCount", "containerd.image.pull", 16559, "", []string{"grpc_service:runtime.v1alpha2.ImageService", "grpc_code:not_found"})

	mockSender.AssertMetric(t, "Gauge", "containerd.proc.open_fds", 200, "", expectedTags)
}
