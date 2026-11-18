// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestKindBinaryName(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		kindVersion string
		want        string
	}{
		{"v0.15.0", "kind015"},
		{"v0.17.0", "kind017"},
		{"v0.20.0", "kind020"},
		{"v0.22.0", "kind022"},
		{"v0.26.0", "kind026"},
		{"v0.27.0", "kind027"},
		{"v0.28.0", "kind028"},
		{"v0.31.0", "kind031"},
		{"v0.32.0", "kind032"},
		{"v0.33.0", "kind033"},
		{"v1.0.0", ""},
		{"v0.32.0-rc.0", ""},
		{"v0.32.1", ""},
		{"not-a-version", ""},
	}
	for _, tt := range tests {
		assert.Equal(tt.want, KindBinaryName(tt.kindVersion), "kind version %q", tt.kindVersion)
	}
}

// The rendered configuration triggers the replacement of the kind cluster,
// so these shapes must keep rendering byte for byte.
func TestGenerateKindConfigGolden(t *testing.T) {
	tests := []struct {
		name  string
		flags KindConfigFlags
		want  string
	}{
		{
			name:  "control plane",
			flags: KindConfigFlags{},
			want: `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - hostPath: /proc
    containerPath: /host/proc
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
      endpoint = ["https://mirror.gcr.io", "https://registry-1.docker.io"]
`,
		},
		{
			name: "worker with labels and taints",
			flags: KindConfigFlags{WorkerNodes: []KindWorkerNode{{
				Labels: []Label{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
				Taints: []Taint{{Key: "spot", Value: "true", Effect: "NoSchedule"}},
			}}},
			want: `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - hostPath: /proc
    containerPath: /host/proc
- role: worker
  extraMounts:
  - hostPath: /proc
    containerPath: /host/proc
  kubeadmConfigPatches:
    - |
      kind: JoinConfiguration
      nodeRegistration:
        kubeletExtraArgs:
          node-labels: "a=1,b=2"
        taints:
          - key: "spot"
            value: "true"
            effect: "NoSchedule"
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
      endpoint = ["https://mirror.gcr.io", "https://registry-1.docker.io"]
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateKindConfig(kindClusterConfig, tt.flags)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Kubelet flags join the node labels in the kubeletExtraArgs of the join
// configuration, and extra mounts follow the default ones.
func TestGenerateKindConfigKubeletExtraArgs(t *testing.T) {
	got, err := generateKindConfig(kindClusterConfig, KindConfigFlags{WorkerNodes: []KindWorkerNode{{
		Labels:           []Label{{Key: "a", Value: "1"}},
		KubeletExtraArgs: map[string]string{"config-dir": "/etc/kubernetes/kubelet.conf.d", "v": "2"},
		ExtraMounts:      []Mount{{HostPath: "/var/lib/kubelet.conf.d", ContainerPath: "/etc/kubernetes/kubelet.conf.d"}},
	}}})
	require.NoError(t, err)

	type mount struct {
		HostPath      string `yaml:"hostPath"`
		ContainerPath string `yaml:"containerPath"`
	}
	var cluster struct {
		Nodes []struct {
			Role                 string   `yaml:"role"`
			ExtraMounts          []mount  `yaml:"extraMounts"`
			KubeadmConfigPatches []string `yaml:"kubeadmConfigPatches"`
		} `yaml:"nodes"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(got), &cluster), got)
	require.Len(t, cluster.Nodes, 2)
	worker := cluster.Nodes[1]
	assert.Equal(t, "worker", worker.Role)
	assert.Equal(t, []mount{
		{"/proc", "/host/proc"},
		{"/var/lib/kubelet.conf.d", "/etc/kubernetes/kubelet.conf.d"},
	}, worker.ExtraMounts)

	require.Len(t, worker.KubeadmConfigPatches, 1)
	var join struct {
		Kind             string `yaml:"kind"`
		NodeRegistration struct {
			KubeletExtraArgs map[string]string `yaml:"kubeletExtraArgs"`
		} `yaml:"nodeRegistration"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(worker.KubeadmConfigPatches[0]), &join), worker.KubeadmConfigPatches[0])
	assert.Equal(t, "JoinConfiguration", join.Kind)
	assert.Equal(t, map[string]string{
		"node-labels": "a=1",
		"config-dir":  "/etc/kubernetes/kubelet.conf.d",
		"v":           "2",
	}, join.NodeRegistration.KubeletExtraArgs)
}

// Kubelet flags and mount paths are quoted, so any value survives the YAML.
func TestGenerateKindConfigQuoting(t *testing.T) {
	const value = `a "b" \c: #d` + "\n e"
	got, err := generateKindConfig(kindClusterConfig, KindConfigFlags{WorkerNodes: []KindWorkerNode{{
		KubeletExtraArgs: map[string]string{"v": value},
		ExtraMounts:      []Mount{{HostPath: value, ContainerPath: "/c"}},
	}}})
	require.NoError(t, err)

	var cluster struct {
		Nodes []struct {
			ExtraMounts []struct {
				HostPath string `yaml:"hostPath"`
			} `yaml:"extraMounts"`
			KubeadmConfigPatches []string `yaml:"kubeadmConfigPatches"`
		} `yaml:"nodes"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(got), &cluster), got)
	require.Len(t, cluster.Nodes, 2)
	worker := cluster.Nodes[1]
	require.Len(t, worker.ExtraMounts, 2)
	assert.Equal(t, value, worker.ExtraMounts[1].HostPath)

	require.Len(t, worker.KubeadmConfigPatches, 1)
	var join struct {
		NodeRegistration struct {
			KubeletExtraArgs map[string]string `yaml:"kubeletExtraArgs"`
		} `yaml:"nodeRegistration"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(worker.KubeadmConfigPatches[0]), &join), worker.KubeadmConfigPatches[0])
	assert.Equal(t, map[string]string{"v": value}, join.NodeRegistration.KubeletExtraArgs)
}

func TestGenerateKindConfigNodeLabelsArg(t *testing.T) {
	_, err := generateKindConfig(kindClusterConfig, KindConfigFlags{WorkerNodes: []KindWorkerNode{{
		KubeletExtraArgs: map[string]string{"node-labels": "a=1"},
	}}})
	assert.Error(t, err)
}
