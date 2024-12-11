// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMeta(t *testing.T) {
	rawData := []byte(`kubelet_runtime_operations_latency_microseconds{operation_type="version",quantile="0.9"} 761
kubelet_runtime_operations_latency_microseconds{operation_type="version",quantile="0.99"} 1372
kubelet_runtime_operations_latency_microseconds_sum{operation_type="version"} 3.02124601e+08
kubelet_runtime_operations_latency_microseconds_count{operation_type="version"} 361825
# HELP kubernetes_build_info A metric with a constant '1' value labeled by major, minor, git version, git commit, git tree state, build date, Go version, and compiler from which Kubernetes was built, and platform on which it is running.
# TYPE kubernetes_build_info gauge
kubernetes_build_info{buildDate="2018-03-21T19:01:20Z",compiler="gc",gitCommit="cb151369f60073317da686a6ce7de36abe2bda8d",gitTreeState="clean",gitVersion="v1.9.6-gke.0",goVersion="go1.9.3b4",major="1",minor="9+",platform="linux/amd64"} 1
# HELP process_cpu_seconds_total Total user and system CPU time spent in seconds.
# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 127923.04
# HELP process_max_fds Maximum number of open file descriptors.
# TYPE process_max_fds gauge`)
	metric, err := ParseMetricFromRaw(rawData, "kubernetes_build_info")
	require.Empty(t, err)
	assert.Equal(t, `kubernetes_build_info{buildDate="2018-03-21T19:01:20Z",compiler="gc",gitCommit="cb151369f60073317da686a6ce7de36abe2bda8d",gitTreeState="clean",gitVersion="v1.9.6-gke.0",goVersion="go1.9.3b4",major="1",minor="9+",platform="linux/amd64"} 1`, metric)

	metric, err = ParseMetricFromRaw(rawData, "process_cpu_seconds_total")
	assert.Empty(t, err)
	assert.Equal(t, "process_cpu_seconds_total 127923.04", metric)
}

func loadPodsFixture(path string) ([]*Pod, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var podList PodList
	err = json.Unmarshal(raw, &podList)
	if err != nil {
		return nil, err
	}
	for _, pod := range podList.Items {
		allContainers := make([]ContainerStatus, 0, len(pod.Status.InitContainers)+len(pod.Status.Containers))
		allContainers = append(allContainers, pod.Status.InitContainers...)
		allContainers = append(allContainers, pod.Status.Containers...)
		pod.Status.AllContainers = allContainers
	}
	return podList.Items, nil
}

func TestKubeContainerIDToTaggerEntityID(t *testing.T) {
	for in, out := range map[string]string{
		"container_id://deadbeef": "container_id://deadbeef",
		"containerd://deadbeef":   "container_id://deadbeef",
		"cri-o://deadbeef":        "container_id://deadbeef",
		"cri-o://d":               "container_id://d",
		"runtime://deadbeef":      "container_id://deadbeef",
		"container_id://":         "",
		"deadbeef":                "",
		"/deadbeef":               "",
		"runtime://foo/bar":       "container_id://foo/bar",
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			res, _ := KubeContainerIDToTaggerEntityID(in)
			assert.Equal(t, out, res.String())
		})
	}
}

func TestKubePodUIDToTaggerEntityID(t *testing.T) {
	for in, out := range map[string]string{
		"kubernetes_pod_uid://deadbeef": "kubernetes_pod_uid://deadbeef",
		"pod://deadbeef":                "kubernetes_pod_uid://deadbeef",
		"kubernetes_pod://deadbeef":     "kubernetes_pod_uid://deadbeef",
		"kubernetes_pod://d":            "kubernetes_pod_uid://d",
		"kubernetes_pod_uid://":         "",
		"deadbeef":                      "",
		"/deadbeef":                     "",
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			res, _ := KubePodUIDToTaggerEntityID(in)
			assert.Equal(t, out, res.String())
		})
	}
}

func TestKubeIDToTaggerEntityID(t *testing.T) {
	for in, out := range map[string]string{
		"kubernetes_pod://deadbeef": "kubernetes_pod_uid://deadbeef",
		"kubernetes_pod://d":        "kubernetes_pod_uid://d",
		"docker://deadbeef":         "container_id://deadbeef",
		"crio://deadbeef":           "container_id://deadbeef",
		"kubernetes_pod://":         "",
		"deadbeef":                  "",
		"/deadbeef":                 "",
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			res, _ := KubeIDToTaggerEntityID(in)
			assert.Equal(t, out, res.String())
		})
	}
}
