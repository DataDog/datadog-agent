// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestFilter(t *testing.T) {
	containers := []struct {
		c  Container
		ns string
	}{
		{
			c: Container{
				ID:    "1",
				Name:  "secret-container-dd",
				Image: "docker-dd-agent",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "2",
				Name:  "webapp1-dd",
				Image: "apache:2.2",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "3",
				Name:  "mysql-dd",
				Image: "mysql:5.3",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "4",
				Name:  "linux-dd",
				Image: "alpine:latest",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "5",
				Name:  "k8s_superpause_kube-apiserver-mega-node_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "gcr.io/random-project/superpause:1.0",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "6",
				Name:  "k8s_superpause_kube-apiserver-mega-node_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "gcr.io/random-project/pause:1.0",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "7",
				Name:  "k8s_POD.f8120f_kube-proxy-gke-pool-1-2890-pv0",
				Image: "gcr.io/google_containers/pause-amd64:3.0",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "8",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "k8s.gcr.io/pause-amd64:3.1",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "9",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "kubernetes/pause:latest",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "10",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "asia.gcr.io/google_containers/pause-amd64:3.0",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "11",
				Name:  "k8s_POD_AZURE_pause",
				Image: "k8s-gcrio.azureedge.net/pause-amd64:3.0",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "12",
				Name:  "k8s_POD_AZURE_pause",
				Image: "gcrio.azureedge.net/google_containers/pause-amd64",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "13",
				Name:  "k8s_POD_rancher_pause",
				Image: "rancher/pause-amd64:3.0",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "14",
				Name:  "foo-dd",
				Image: "foo:1.0",
			},
			ns: "foo",
		},
		{
			c: Container{
				ID:    "15",
				Name:  "bar-dd",
				Image: "bar:1.0",
			},
			ns: "bar",
		},
		{
			c: Container{
				ID:    "16",
				Name:  "foo",
				Image: "gcr.io/gke-release/pause-win:1.1.0",
			},
			ns: "bar",
		},
		{
			c: Container{
				ID:    "17",
				Name:  "foo",
				Image: "mcr.microsoft.com/k8s/core/pause:1.2.0",
			},
			ns: "bar",
		},
		{
			c: Container{
				ID:    "18",
				Name:  "foo",
				Image: "ecr.us-east-1.amazonaws.com/pause",
			},
			ns: "bar",
		},
		{
			c: Container{
				ID:    "19",
				Name:  "k8s_POD_AKS_pause",
				Image: "aksrepos.azurecr.io/mirror/pause-amd64:3.1",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "20",
				Name:  "k8s_POD_OSE3",
				Image: "registry.access.redhat.com/rhel7/pod-infrastructure:latest",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "23",
				Name:  "k8s_POD_EKS_Win",
				Image: "amazonaws.com/eks/pause-windows:latest",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "24",
				Name:  "k8s_POD_AKS_Win",
				Image: "kubeletwin/pause:latest",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "25",
				Name:  "eu_gcr",
				Image: "eu.gcr.io/k8s-artifacts-prod/pause:3.3",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "26",
				Name:  "private_jfrog",
				Image: "foo.jfrog.io/google_containers/pause",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "27",
				Name:  "private_ecr_upstream",
				Image: "2342834325.ecr.us-east-1.amazonaws.com/upstream/pause",
			},
			ns: "default",
		},
		{
			c: Container{
				ID:    "28",
				Name:  "cdk",
				Image: "cdk/pause-amd64:3.1",
			},
			ns: "default",
		},
	}

	for i, tc := range []struct {
		includeList []string
		excludeList []string
		expectedIDs []string
	}{
		{
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28"},
		},
		{
			excludeList: []string{"name:secret"},
			expectedIDs: []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28"},
		},
		{
			excludeList: []string{"image:secret"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28"},
		},
		{
			includeList: []string{},
			excludeList: []string{"image:apache", "image:alpine"},
			expectedIDs: []string{"1", "3", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28"},
		},
		{
			includeList: []string{"name:mysql"},
			excludeList: []string{"name:dd"},
			expectedIDs: []string{"3", "5", "6", "7", "8", "9", "10", "11", "12", "13", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28"},
		},
		{
			excludeList: []string{"kube_namespace:.*"},
			includeList: []string{"kube_namespace:foo"},
			expectedIDs: []string{"14"},
		},
		{
			excludeList: []string{"kube_namespace:bar"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "19", "20", "23", "24", "25", "26", "27", "28"},
		},
		// Test kubernetes defaults
		{
			excludeList: []string{
				pauseContainerGCR,
				pauseContainerOpenshift,
				pauseContainerOpenshift3,
				pauseContainerKubernetes,
				pauseContainerGoogle,
				pauseContainerAzure,
				pauseContainerECS,
				pauseContainerEKS,
				pauseContainerRancher,
				pauseContainerMCR,
				pauseContainerWin,
				pauseContainerAKS,
				pauseContainerECR,
				pauseContainerUpstream,
				pauseContainerCDK,
			},
			expectedIDs: []string{"1", "2", "3", "4", "5", "14", "15"},
		},
	} {
		t.Run("", func(t *testing.T) {
			f, err := NewFilter(tc.includeList, tc.excludeList)
			require.Nil(t, err, "case %d", i)

			var allowed []string
			for _, c := range containers {
				if !f.IsExcluded(c.c.Name, c.c.Image, c.ns) {
					allowed = append(allowed, c.c.ID)
				}
			}
			assert.Equal(t, tc.expectedIDs, allowed, "case %d", i)
		})
	}
}

func TestNewMetricFilterFromConfig(t *testing.T) {
	config.Datadog.SetDefault("exclude_pause_container", true)
	config.Datadog.SetDefault("ac_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd-.*"})

	f, err := newMetricFilterFromConfig()
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded("dummy", "dummy", ""))
	assert.True(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.True(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", ""))

	config.Datadog.SetDefault("exclude_pause_container", false)
	f, err = newMetricFilterFromConfig()
	require.NoError(t, err)
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", ""))

	config.Datadog.SetDefault("exclude_pause_container", true)
	config.Datadog.SetDefault("ac_include", []string{})
	config.Datadog.SetDefault("ac_exclude", []string{})

	config.Datadog.SetDefault("exclude_pause_container", false)
	config.Datadog.SetDefault("container_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude", []string{"name:dd-.*"})
	config.Datadog.SetDefault("container_include_metrics", []string{"image:nginx.*"})
	config.Datadog.SetDefault("container_exclude_metrics", []string{"name:ddmetric-.*"})

	f, err = newMetricFilterFromConfig()
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", ""))
	assert.True(t, f.IsExcluded("ddmetric-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded("ddmetric-152462", "nginx:latest", ""))
	assert.False(t, f.IsExcluded("dummy", "dummy", ""))
}

func TestNewAutodiscoveryFilter(t *testing.T) {
	resetConfig()

	// Global - legacy config
	config.Datadog.SetDefault("ac_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd-.*"})

	f, err := NewAutodiscoveryFilter(GlobalFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded("dummy", "dummy", ""))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", ""))
	resetConfig()

	// Global - new config - legacy config ignored
	config.Datadog.SetDefault("container_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude", []string{"name:dd-.*"})
	config.Datadog.SetDefault("ac_include", []string{"image:apache/legacy.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd/legacy-.*"})

	f, err = NewAutodiscoveryFilter(GlobalFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded("dd/legacy-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded("dummy", "dummy", ""))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", ""))
	resetConfig()

	// Metrics
	config.Datadog.SetDefault("container_include_metrics", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude_metrics", []string{"name:dd-.*"})

	f, err = NewAutodiscoveryFilter(MetricsFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded("dummy", "dummy", ""))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", ""))
	resetConfig()

	// Logs
	config.Datadog.SetDefault("container_include_logs", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude_logs", []string{"name:dd-.*"})

	f, err = NewAutodiscoveryFilter(LogsFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded("dummy", "dummy", ""))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", ""))
	resetConfig()
}

func resetConfig() {
	config.Datadog.SetDefault("exclude_pause_container", true)
	config.Datadog.SetDefault("container_include", []string{})
	config.Datadog.SetDefault("container_exclude", []string{})
	config.Datadog.SetDefault("container_include_metrics", []string{})
	config.Datadog.SetDefault("container_exclude_metrics", []string{})
	config.Datadog.SetDefault("container_include_logs", []string{})
	config.Datadog.SetDefault("container_exclude_logs", []string{})
	config.Datadog.SetDefault("ac_include", []string{})
	config.Datadog.SetDefault("ac_exclude", []string{})
}
