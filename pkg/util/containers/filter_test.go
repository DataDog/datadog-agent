// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

type ctnDef struct {
	ID    string
	Name  string
	Image string
}

func TestFilter(t *testing.T) {
	containers := []struct {
		c           ctnDef
		ns          string
		annotations map[string]string
		labels      map[string]string
	}{
		{
			c: ctnDef{
				ID:    "1",
				Name:  "secret-container-dd",
				Image: "docker-dd-agent",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "2",
				Name:  "webapp1-dd",
				Image: "apache:2.2",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "3",
				Name:  "mysql-dd",
				Image: "mysql:5.3",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "4",
				Name:  "linux-dd",
				Image: "alpine:latest",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "5",
				Name:  "k8s_superpause_kube-apiserver-mega-node_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "gcr.io/random-project/superpause:1.0",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "6",
				Name:  "k8s_superpause_kube-apiserver-mega-node_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "gcr.io/random-project/pause:1.0",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "7",
				Name:  "k8s_POD.f8120f_kube-proxy-gke-pool-1-2890-pv0",
				Image: "gcr.io/google_containers/pause-amd64:3.0",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "8",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "k8s.gcr.io/pause-amd64:3.1",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "9",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "kubernetes/pause:latest",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "10",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "asia.gcr.io/google_containers/pause-amd64:3.0",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "11",
				Name:  "k8s_POD_AZURE_pause",
				Image: "k8s-gcrio.azureedge.net/pause-amd64:3.0",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "12",
				Name:  "k8s_POD_AZURE_pause",
				Image: "gcrio.azureedge.net/google_containers/pause-amd64",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "13",
				Name:  "k8s_POD_rancher_pause",
				Image: "rancher/pause-amd64:3.0",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "14",
				Name:  "foo-dd",
				Image: "foo:1.0",
			},
			ns:          "foo",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "15",
				Name:  "bar-dd",
				Image: "bar:1.0",
			},
			ns:          "bar",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "16",
				Name:  "foo",
				Image: "gcr.io/gke-release/pause-win:1.1.0",
			},
			ns:          "bar",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "17",
				Name:  "foo",
				Image: "mcr.microsoft.com/k8s/core/pause:1.2.0",
			},
			ns:          "bar",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "18",
				Name:  "foo",
				Image: "ecr.us-east-1.amazonaws.com/pause",
			},
			ns:          "bar",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "19",
				Name:  "k8s_POD_AKS_pause",
				Image: "aksrepos.azurecr.io/mirror/pause-amd64:3.1",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "20",
				Name:  "k8s_POD_OSE3",
				Image: "registry.access.redhat.com/rhel7/pod-infrastructure:latest",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "23",
				Name:  "k8s_POD_EKS_Win",
				Image: "amazonaws.com/eks/pause-windows:latest",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "24",
				Name:  "k8s_POD_AKS_Win",
				Image: "kubeletwin/pause:latest",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "25",
				Name:  "eu_gcr",
				Image: "eu.gcr.io/k8s-artifacts-prod/pause:3.3",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "26",
				Name:  "private_jfrog",
				Image: "foo.jfrog.io/google_containers/pause",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "27",
				Name:  "private_ecr_upstream",
				Image: "2342834325.ecr.us-east-1.amazonaws.com/upstream/pause",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "28",
				Name:  "cdk",
				Image: "cdk/pause-amd64:3.1",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{ // Empty name
				ID:    "29",
				Name:  "",
				Image: "redis",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{ // Empty image
				ID:    "30",
				Name:  "empty_image",
				Image: "",
			},
			ns:          "default",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{ // Empty namespace
				ID:    "31",
				Name:  "empty_namespace",
				Image: "redis",
			},
			ns:          "",
			annotations: nil,
			labels:      nil,
		},
		{
			c: ctnDef{
				ID:    "32",
				Name:  "pdcsi-node-8dp72",
				Image: "gke.gcr.io/csi-node-driver-registrar:v2.5.1-gke.0",
			},
			ns:          "kube-system",
			annotations: map[string]string{"components.foo.io/component-version": "0.13.3", "kubernetes.io/config.source": "api", "foo.bar/exclude": "true", "seccomp.security.alpha.kubernetes.io/pod": "runtime/default", "components.gke.io/component-name": "pdcsi"},
			labels:      map[string]string{"k8s-app": "gcp-compute-persistent-disk-csi-driver", "pod-template-generation": "10", "controller-revision-hash": "7d96cb4b8"},
		},
		{
			c: ctnDef{
				ID:    "33",
				Name:  "busybox",
				Image: "busybox",
			},
			ns:          "default",
			annotations: map[string]string{"components.foo.io/component-version": "1.18.3", "foo.bar/exclude": "false"},
			labels:      map[string]string{"k8s-app": "busybox", "pod-template-generation": "14"},
		},
		{
			c: ctnDef{
				ID:    "34",
				Name:  "busybox",
				Image: "busybox",
			},
			ns:          "default",
			annotations: map[string]string{"components.foo.io/component-version": "1.18.3", "foo.bar/exclude": "true", "foo.bar/include": "true"},
			labels:      map[string]string{"foo.bar/exclude": "true", "k8s-app": "busybox", "pod-template-generation": "14"},
		},
		{
			c: ctnDef{
				ID:    "35",
				Name:  "busybox",
				Image: "busybox",
			},
			ns:          "default",
			annotations: map[string]string{"foo.bar/include": "true"},
			labels:      map[string]string{"exclude": "true", "app": "buxybox", "foo": "bar"},
		},
	}

	for i, tc := range []struct {
		includeList []string
		excludeList []string
		expectedIDs []string
	}{
		{
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35"},
		},
		{
			excludeList: []string{"name:secret"},
			expectedIDs: []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35"},
		},
		{
			excludeList: []string{"image:secret"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35"},
		},
		{
			includeList: []string{},
			excludeList: []string{"image:apache", "image:alpine"},
			expectedIDs: []string{"1", "3", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35"},
		},
		{
			includeList: []string{"name:mysql"},
			excludeList: []string{"name:dd"},
			expectedIDs: []string{"3", "5", "6", "7", "8", "9", "10", "11", "12", "13", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35"},
		},
		{
			excludeList: []string{"kube_namespace:.*"},
			includeList: []string{"kube_namespace:foo"},
			expectedIDs: []string{"14", "31"},
		},
		{
			excludeList: []string{"kube_namespace:bar"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35"},
		},
		{
			excludeList: []string{"name:.*"},
			includeList: []string{"name:mysql-dd"},
			expectedIDs: []string{"3", "29"},
		},
		{
			excludeList: []string{"image:.*"},
			includeList: []string{"image:docker-dd-agent"},
			expectedIDs: []string{"1", "30"},
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
			expectedIDs: []string{"1", "2", "3", "4", "5", "14", "15", "29", "30", "31", "32", "33", "34", "35"},
		},
		{
			excludeList: []string{"annotation:foo.bar/exclude:true"},
			includeList: []string{},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "33", "35"},
		},
		{
			excludeList: []string{"annotation:foo.bar/exclude:true"},
			includeList: []string{"annotation:foo.bar/include:true"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "33", "34", "35"},
		},
		{
			excludeList: []string{"label:foo.bar/exclude:true"},
			includeList: []string{"label:include:true"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "35"},
		},
		{
			excludeList: []string{"annotation:.*", "name:.*"},
			includeList: []string{"label:foo:bar"},
			expectedIDs: []string{"29", "35"},
		},
	} {
		t.Run("", func(t *testing.T) {
			f, err := NewFilter(tc.includeList, tc.excludeList)
			require.Nil(t, err, "case %d", i)

			var allowed []string
			for _, c := range containers {
				if !f.IsExcluded(c.c.Name, c.c.Image, c.ns, c.annotations, c.labels) {
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

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "dummy", "", nil, nil))
	assert.True(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", "", nil, nil))
	assert.True(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", "", nil, nil))

	config.Datadog.SetDefault("exclude_pause_container", false)
	f, err = newMetricFilterFromConfig()
	require.NoError(t, err)
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", "", nil, nil))

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

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", "", nil, nil))
	assert.True(t, f.IsExcluded("ddmetric-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("ddmetric-152462", "nginx:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "dummy", "", nil, nil))
}

func TestNewAutodiscoveryFilter(t *testing.T) {
	resetConfig()

	// Global - legacy config
	config.Datadog.SetDefault("ac_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd-.*"})

	f, err := NewAutodiscoveryFilter(GlobalFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "dummy", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", "", nil, nil))
	resetConfig()

	// Global - new config - legacy config ignored
	config.Datadog.SetDefault("container_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude", []string{"name:dd-.*"})
	config.Datadog.SetDefault("ac_include", []string{"image:apache/legacy.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd/legacy-.*"})

	f, err = NewAutodiscoveryFilter(GlobalFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dd/legacy-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "dummy", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", "", nil, nil))
	resetConfig()

	// Metrics
	config.Datadog.SetDefault("container_include_metrics", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude_metrics", []string{"name:dd-.*"})

	f, err = NewAutodiscoveryFilter(MetricsFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "dummy", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", "", nil, nil))
	resetConfig()

	// Logs
	config.Datadog.SetDefault("container_include_logs", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude_logs", []string{"name:dd-.*"})

	f, err = NewAutodiscoveryFilter(LogsFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "dummy", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", "", nil, nil))
	resetConfig()

	// Filter errors - non-duplicate error messages
	config.Datadog.SetDefault("container_include", []string{"image:apache.*", "invalid"})
	config.Datadog.SetDefault("container_exclude", []string{"name:dd-.*", "invalid"})

	f, err = NewAutodiscoveryFilter(GlobalFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded("dd-152462", "dummy:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dd-152462", "apache:latest", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "dummy", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "k8s.gcr.io/pause-amd64:3.1", "", nil, nil))
	assert.False(t, f.IsExcluded("dummy", "rancher/pause-amd64:3.1", "", nil, nil))
	fe := map[string]struct{}{
		"Container filter \"invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'": {},
	}
	assert.Equal(t, fe, GetFilterErrors())
	ResetSharedFilter()
	resetConfig()

	// Filter errors - invalid regex
	config.Datadog.SetDefault("container_include", []string{"image:apache.*", "kube_namespace:?"})
	config.Datadog.SetDefault("container_exclude", []string{"name:dd-.*", "invalid"})

	f, err = NewAutodiscoveryFilter(GlobalFilter)
	assert.Error(t, err, errors.New("invalid regex '?': error parsing regexp: missing argument to repetition operator: `?`"))
	assert.NotNil(t, f)
	fe = map[string]struct{}{
		"invalid regex '?': error parsing regexp: missing argument to repetition operator: `?`":                                {},
		"Container filter \"invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'": {},
	}
	assert.Equal(t, fe, GetFilterErrors())
	ResetSharedFilter()
	resetConfig()
}

func TestValidateFilter(t *testing.T) {
	for filters, tc := range []struct {
		desc           string
		filter         string
		prefix         string
		expectedRegexp *regexp.Regexp
		expectedErr    error
	}{
		{
			desc:           "image filter",
			filter:         "image:apache.*",
			prefix:         imageFilterPrefix,
			expectedRegexp: regexp.MustCompile("apache.*"),
			expectedErr:    nil,
		},
		{
			desc:           "name filter",
			filter:         "name:dd-.*",
			prefix:         nameFilterPrefix,
			expectedRegexp: regexp.MustCompile("dd-.*"),
			expectedErr:    nil,
		},
		{
			desc:           "kube_namespace filter",
			filter:         "kube_namespace:monitoring",
			prefix:         kubeNamespaceFilterPrefix,
			expectedRegexp: regexp.MustCompile("monitoring"),
			expectedErr:    nil,
		},
		{
			desc:           "empty filter regex",
			filter:         "image:",
			prefix:         imageFilterPrefix,
			expectedRegexp: regexp.MustCompile(""),
			expectedErr:    nil,
		},
		{
			desc:           "annotation filter",
			filter:         "annotation:kubernetes.io/config.source:api",
			prefix:         annotationFilterPrefix,
			expectedRegexp: regexp.MustCompile("kubernetes.io/config.source:api"),
			expectedErr:    nil,
		},
		{
			desc:           "label filter",
			filter:         "label:k8s-app:gcp-compute-persistent-disk-csi-driver",
			prefix:         labelFilterPrefix,
			expectedRegexp: regexp.MustCompile("k8s-app:gcp-compute-persistent-disk-csi-driver"),
			expectedErr:    nil,
		},
		{
			desc:           "invalid golang regex",
			filter:         "image:?",
			prefix:         imageFilterPrefix,
			expectedRegexp: nil,
			expectedErr:    errors.New("invalid regex '?': error parsing regexp: missing argument to repetition operator: `?`"),
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", filters, tc.desc), func(t *testing.T) {
			r, err := filterToRegex(tc.filter, tc.prefix)
			assert.Equal(t, tc.expectedRegexp, r)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

func TestParseFilters(t *testing.T) {
	for filters, tc := range []struct {
		desc              string
		filters           []string
		imageFilters      []*regexp.Regexp
		nameFilters       []*regexp.Regexp
		namespaceFilters  []*regexp.Regexp
		annotationFilters []*regexp.Regexp
		labelFilters      []*regexp.Regexp
		expectedErrMsg    error
		filterErrors      []string
	}{
		{
			desc:              "valid filters",
			filters:           []string{"image:nginx.*", "name:xyz-.*", "kube_namespace:sandbox.*", "name:abc", "annotation:seccomp.security.alpha.kubernetes.io/pod:runtime/default", "label:controller-revision-hash:7d96cb4b8"},
			imageFilters:      []*regexp.Regexp{regexp.MustCompile("nginx.*")},
			nameFilters:       []*regexp.Regexp{regexp.MustCompile("xyz-.*"), regexp.MustCompile("abc")},
			namespaceFilters:  []*regexp.Regexp{regexp.MustCompile("sandbox.*")},
			annotationFilters: []*regexp.Regexp{regexp.MustCompile("seccomp.security.alpha.kubernetes.io/pod:runtime/default")},
			labelFilters:      []*regexp.Regexp{regexp.MustCompile("controller-revision-hash:7d96cb4b8")},
			expectedErrMsg:    nil,
			filterErrors:      nil,
		},
		{
			desc:              "invalid regex",
			filters:           []string{"image:apache.*", "name:a(?=b)", "kube_namespace:sandbox.*", "name:abc"},
			imageFilters:      nil,
			nameFilters:       nil,
			namespaceFilters:  nil,
			annotationFilters: nil,
			labelFilters:      nil,
			expectedErrMsg:    errors.New("invalid regex 'a(?=b)': error parsing regexp: invalid or unsupported Perl syntax: `(?=`"),
			filterErrors:      []string{"invalid regex 'a(?=b)': error parsing regexp: invalid or unsupported Perl syntax: `(?=`"},
		},
		{
			desc:              "invalid filter prefix, valid regex",
			filters:           []string{"image:redis.*", "invalid", "name:dd-.*", "kube_namespace:dev-.*", "name:abc", "annotation:.*", "label:abc/dce:fgh", "also invalid"},
			imageFilters:      []*regexp.Regexp{regexp.MustCompile("redis.*")},
			nameFilters:       []*regexp.Regexp{regexp.MustCompile("dd-.*"), regexp.MustCompile("abc")},
			namespaceFilters:  []*regexp.Regexp{regexp.MustCompile("dev-.*")},
			annotationFilters: []*regexp.Regexp{regexp.MustCompile(".*")},
			labelFilters:      []*regexp.Regexp{regexp.MustCompile("abc/dce:fgh")},
			expectedErrMsg:    nil,
			filterErrors: []string{
				"Container filter \"invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'",
				"Container filter \"also invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'",
			},
		},
		{
			desc:              "invalid regex and invalid filter prefix",
			filters:           []string{"invalid", "name:a(?=b)", "image:apache.*", "kube_namespace:?", "also invalid", "name:abc"},
			imageFilters:      nil,
			nameFilters:       nil,
			namespaceFilters:  nil,
			annotationFilters: nil,
			labelFilters:      nil,
			expectedErrMsg:    errors.New("invalid regex 'a(?=b)': error parsing regexp: invalid or unsupported Perl syntax: `(?=`"),
			filterErrors: []string{
				"invalid regex 'a(?=b)': error parsing regexp: invalid or unsupported Perl syntax: `(?=`",
				"invalid regex '?': error parsing regexp: missing argument to repetition operator: `?`",
				"Container filter \"invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'",
				"Container filter \"also invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'",
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", filters, tc.desc), func(t *testing.T) {
			imageFilters, nameFilters, namespaceFilters, annotationFilters, labelFilters, filterErrors, err := parseFilters(tc.filters)
			assert.Equal(t, tc.imageFilters, imageFilters)
			assert.Equal(t, tc.nameFilters, nameFilters)
			assert.Equal(t, tc.namespaceFilters, namespaceFilters)
			assert.Equal(t, tc.annotationFilters, annotationFilters)
			assert.Equal(t, tc.labelFilters, labelFilters)
			assert.Equal(t, tc.filterErrors, filterErrors)
			assert.Equal(t, tc.expectedErrMsg, err)
		})
	}
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
