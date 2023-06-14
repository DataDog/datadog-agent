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
		c  ctnDef
		ns string
	}{
		{
			c: ctnDef{
				ID:    "1",
				Name:  "secret-container-dd",
				Image: "docker-dd-agent",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "2",
				Name:  "webapp1-dd",
				Image: "apache:2.2",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "3",
				Name:  "mysql-dd",
				Image: "mysql:5.3",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "4",
				Name:  "linux-dd",
				Image: "alpine:latest",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "5",
				Name:  "k8s_superpause_kube-apiserver-mega-node_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "gcr.io/random-project/superpause:1.0",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "6",
				Name:  "k8s_superpause_kube-apiserver-mega-node_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "gcr.io/random-project/pause:1.0",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "7",
				Name:  "k8s_POD.f8120f_kube-proxy-gke-pool-1-2890-pv0",
				Image: "gcr.io/google_containers/pause-amd64:3.0",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "8",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "k8s.gcr.io/pause-amd64:3.1",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "9",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "kubernetes/pause:latest",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "10",
				Name:  "k8s_POD_kube-apiserver-node-name_kube-system_1ffeada3879805c883bb6d9ba7beca44_0",
				Image: "asia.gcr.io/google_containers/pause-amd64:3.0",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "11",
				Name:  "k8s_POD_AZURE_pause",
				Image: "k8s-gcrio.azureedge.net/pause-amd64:3.0",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "12",
				Name:  "k8s_POD_AZURE_pause",
				Image: "gcrio.azureedge.net/google_containers/pause-amd64",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "13",
				Name:  "k8s_POD_rancher_pause",
				Image: "rancher/pause-amd64:3.0",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "14",
				Name:  "foo-dd",
				Image: "foo:1.0",
			},
			ns: "foo",
		},
		{
			c: ctnDef{
				ID:    "15",
				Name:  "bar-dd",
				Image: "bar:1.0",
			},
			ns: "bar",
		},
		{
			c: ctnDef{
				ID:    "16",
				Name:  "foo",
				Image: "gcr.io/gke-release/pause-win:1.1.0",
			},
			ns: "bar",
		},
		{
			c: ctnDef{
				ID:    "17",
				Name:  "foo",
				Image: "mcr.microsoft.com/k8s/core/pause:1.2.0",
			},
			ns: "bar",
		},
		{
			c: ctnDef{
				ID:    "18",
				Name:  "foo",
				Image: "ecr.us-east-1.amazonaws.com/pause",
			},
			ns: "bar",
		},
		{
			c: ctnDef{
				ID:    "19",
				Name:  "k8s_POD_AKS_pause",
				Image: "aksrepos.azurecr.io/mirror/pause-amd64:3.1",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "20",
				Name:  "k8s_POD_OSE3",
				Image: "registry.access.redhat.com/rhel7/pod-infrastructure:latest",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "23",
				Name:  "k8s_POD_EKS_Win",
				Image: "amazonaws.com/eks/pause-windows:latest",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "24",
				Name:  "k8s_POD_AKS_Win",
				Image: "kubeletwin/pause:latest",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "25",
				Name:  "eu_gcr",
				Image: "eu.gcr.io/k8s-artifacts-prod/pause:3.3",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "26",
				Name:  "private_jfrog",
				Image: "foo.jfrog.io/google_containers/pause",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "27",
				Name:  "private_ecr_upstream",
				Image: "2342834325.ecr.us-east-1.amazonaws.com/upstream/pause",
			},
			ns: "default",
		},
		{
			c: ctnDef{
				ID:    "28",
				Name:  "cdk",
				Image: "cdk/pause-amd64:3.1",
			},
			ns: "default",
		},
		{
			c: ctnDef{ // Empty name
				ID:    "29",
				Name:  "",
				Image: "redis",
			},
			ns: "default",
		},
		{
			c: ctnDef{ // Empty image
				ID:    "30",
				Name:  "empty_image",
				Image: "",
			},
			ns: "default",
		},
		{
			c: ctnDef{ // Empty namespace
				ID:    "31",
				Name:  "empty_namespace",
				Image: "redis",
			},
			ns: "",
		},
	}

	for i, tc := range []struct {
		includeList []string
		excludeList []string
		expectedIDs []string
	}{
		{
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31"},
		},
		{
			excludeList: []string{"name:secret"},
			expectedIDs: []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31"},
		},
		{
			excludeList: []string{"image:secret"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31"},
		},
		{
			includeList: []string{},
			excludeList: []string{"image:apache", "image:alpine"},
			expectedIDs: []string{"1", "3", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31"},
		},
		{
			includeList: []string{"name:mysql"},
			excludeList: []string{"name:dd"},
			expectedIDs: []string{"3", "5", "6", "7", "8", "9", "10", "11", "12", "13", "16", "17", "18", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31"},
		},
		{
			excludeList: []string{"kube_namespace:.*"},
			includeList: []string{"kube_namespace:foo"},
			expectedIDs: []string{"14", "31"},
		},
		{
			excludeList: []string{"kube_namespace:bar"},
			expectedIDs: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "19", "20", "23", "24", "25", "26", "27", "28", "29", "30", "31"},
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
			expectedIDs: []string{"1", "2", "3", "4", "5", "14", "15", "29", "30", "31"},
		},
	} {
		t.Run("", func(t *testing.T) {
			f, err := NewFilter(GlobalFilter, tc.includeList, tc.excludeList)
			require.Nil(t, err, "case %d", i)

			var allowed []string
			for _, c := range containers {
				if !f.IsExcluded(nil, c.c.Name, c.c.Image, c.ns) {
					allowed = append(allowed, c.c.ID)
				}
			}
			assert.Equal(t, tc.expectedIDs, allowed, "case %d", i)
		})
	}
}

func TestIsExcludedByAnnotation(t *testing.T) {
	containerExcludeName := "foo"
	containerIncludeName := "bar"
	containerNoMentionName := "other"
	annotations := map[string]string{
		fmt.Sprintf("ad.datadoghq.com/%s.exclude", containerExcludeName):         `true`,
		fmt.Sprintf("ad.datadoghq.com/%s.metrics_exclude", containerExcludeName): `true`,
		fmt.Sprintf("ad.datadoghq.com/%s.logs_exclude", containerExcludeName):    `true`,
		fmt.Sprintf("ad.datadoghq.com/%s.exclude", containerIncludeName):         `false`,
		fmt.Sprintf("ad.datadoghq.com/%s.metrics_exclude", containerIncludeName): `false`,
		fmt.Sprintf("ad.datadoghq.com/%s.logs_exclude", containerIncludeName):    `false`,
	}

	containerlessAnnotations := map[string]string{
		"ad.datadoghq.com/exclude":         `true`,
		"ad.datadoghq.com/metrics_exclude": `true`,
		"ad.datadoghq.com/logs_exclude":    `true`,
	}

	globalExcludeContainerLess := map[string]string{
		"ad.datadoghq.com/exclude": `true`,
	}

	globalExclude := map[string]string{
		fmt.Sprintf("ad.datadoghq.com/%s.exclude", containerIncludeName): `true`,
	}

	globalFilter, err := NewFilter(GlobalFilter, nil, nil)
	require.NoError(t, err)
	metricsFilter, err := NewFilter(MetricsFilter, nil, nil)
	require.NoError(t, err)
	logsFilter, err := NewFilter(LogsFilter, nil, nil)
	require.NoError(t, err)

	// Container-specific annotations
	assert.True(t, globalFilter.isExcludedByAnnotation(annotations, containerExcludeName))
	assert.True(t, metricsFilter.isExcludedByAnnotation(annotations, containerExcludeName))
	assert.True(t, logsFilter.isExcludedByAnnotation(annotations, containerExcludeName))

	assert.False(t, globalFilter.isExcludedByAnnotation(annotations, containerIncludeName))
	assert.False(t, metricsFilter.isExcludedByAnnotation(annotations, containerIncludeName))
	assert.False(t, logsFilter.isExcludedByAnnotation(annotations, containerIncludeName))

	assert.False(t, globalFilter.isExcludedByAnnotation(annotations, containerNoMentionName))
	assert.False(t, metricsFilter.isExcludedByAnnotation(annotations, containerNoMentionName))
	assert.False(t, logsFilter.isExcludedByAnnotation(annotations, containerNoMentionName))

	// Container-less annotations
	assert.True(t, globalFilter.isExcludedByAnnotation(containerlessAnnotations, containerExcludeName))
	assert.True(t, metricsFilter.isExcludedByAnnotation(containerlessAnnotations, containerExcludeName))
	assert.True(t, logsFilter.isExcludedByAnnotation(containerlessAnnotations, containerExcludeName))

	assert.True(t, globalFilter.isExcludedByAnnotation(containerlessAnnotations, containerIncludeName))
	assert.True(t, metricsFilter.isExcludedByAnnotation(containerlessAnnotations, containerIncludeName))
	assert.True(t, logsFilter.isExcludedByAnnotation(containerlessAnnotations, containerIncludeName))

	assert.True(t, globalFilter.isExcludedByAnnotation(containerlessAnnotations, containerNoMentionName))
	assert.True(t, metricsFilter.isExcludedByAnnotation(containerlessAnnotations, containerNoMentionName))
	assert.True(t, logsFilter.isExcludedByAnnotation(containerlessAnnotations, containerNoMentionName))

	// Container-less global exclude
	assert.True(t, globalFilter.isExcludedByAnnotation(globalExcludeContainerLess, containerIncludeName))
	assert.True(t, metricsFilter.isExcludedByAnnotation(globalExcludeContainerLess, containerIncludeName))
	assert.True(t, logsFilter.isExcludedByAnnotation(globalExcludeContainerLess, containerIncludeName))

	// Global exclude
	assert.True(t, globalFilter.isExcludedByAnnotation(globalExclude, containerIncludeName))
	assert.True(t, metricsFilter.isExcludedByAnnotation(globalExclude, containerIncludeName))
	assert.True(t, logsFilter.isExcludedByAnnotation(globalExclude, containerIncludeName))

	assert.False(t, logsFilter.isExcludedByAnnotation(nil, containerExcludeName))
}

func TestNewMetricFilterFromConfig(t *testing.T) {
	config.Datadog.SetDefault("exclude_pause_container", true)
	config.Datadog.SetDefault("ac_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd-.*"})

	f, err := newMetricFilterFromConfig()
	require.NoError(t, err)

	assert.True(t, f.IsExcluded(nil, "dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "dummy", ""))
	assert.True(t, f.IsExcluded(nil, "dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.True(t, f.IsExcluded(nil, "dummy", "rancher/pause-amd64:3.1", ""))

	config.Datadog.SetDefault("exclude_pause_container", false)
	f, err = newMetricFilterFromConfig()
	require.NoError(t, err)
	assert.False(t, f.IsExcluded(nil, "dummy", "k8s.gcr.io/pause-amd64:3.1", ""))

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

	assert.True(t, f.IsExcluded(nil, "dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dd-152462", "apache:latest", ""))
	assert.True(t, f.IsExcluded(nil, "ddmetric-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "ddmetric-152462", "nginx:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "dummy", ""))
}

func TestNewAutodiscoveryFilter(t *testing.T) {
	resetConfig()

	// Global - legacy config
	config.Datadog.SetDefault("ac_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd-.*"})

	f, err := NewAutodiscoveryFilter(GlobalFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded(nil, "dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "dummy", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "rancher/pause-amd64:3.1", ""))
	resetConfig()

	// Global - new config - legacy config ignored
	config.Datadog.SetDefault("container_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude", []string{"name:dd-.*"})
	config.Datadog.SetDefault("ac_include", []string{"image:apache/legacy.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd/legacy-.*"})

	f, err = NewAutodiscoveryFilter(GlobalFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded(nil, "dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dd/legacy-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "dummy", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "rancher/pause-amd64:3.1", ""))
	resetConfig()

	// Metrics
	config.Datadog.SetDefault("container_include_metrics", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude_metrics", []string{"name:dd-.*"})

	f, err = NewAutodiscoveryFilter(MetricsFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded(nil, "dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "dummy", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "rancher/pause-amd64:3.1", ""))
	resetConfig()

	// Logs
	config.Datadog.SetDefault("container_include_logs", []string{"image:apache.*"})
	config.Datadog.SetDefault("container_exclude_logs", []string{"name:dd-.*"})

	f, err = NewAutodiscoveryFilter(LogsFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded(nil, "dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "dummy", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "rancher/pause-amd64:3.1", ""))
	resetConfig()

	// Filter errors - non-duplicate error messages
	config.Datadog.SetDefault("container_include", []string{"image:apache.*", "invalid"})
	config.Datadog.SetDefault("container_exclude", []string{"name:dd-.*", "invalid"})

	f, err = NewAutodiscoveryFilter(GlobalFilter)
	require.NoError(t, err)

	assert.True(t, f.IsExcluded(nil, "dd-152462", "dummy:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dd-152462", "apache:latest", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "dummy", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "k8s.gcr.io/pause-amd64:3.1", ""))
	assert.False(t, f.IsExcluded(nil, "dummy", "rancher/pause-amd64:3.1", ""))
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
		desc             string
		filters          []string
		imageFilters     []*regexp.Regexp
		nameFilters      []*regexp.Regexp
		namespaceFilters []*regexp.Regexp
		expectedErrMsg   error
		filterErrors     []string
	}{
		{
			desc:             "valid filters",
			filters:          []string{"image:nginx.*", "name:xyz-.*", "kube_namespace:sandbox.*", "name:abc"},
			imageFilters:     []*regexp.Regexp{regexp.MustCompile("nginx.*")},
			nameFilters:      []*regexp.Regexp{regexp.MustCompile("xyz-.*"), regexp.MustCompile("abc")},
			namespaceFilters: []*regexp.Regexp{regexp.MustCompile("sandbox.*")},
			expectedErrMsg:   nil,
			filterErrors:     nil,
		},
		{
			desc:             "invalid regex",
			filters:          []string{"image:apache.*", "name:a(?=b)", "kube_namespace:sandbox.*", "name:abc"},
			imageFilters:     nil,
			nameFilters:      nil,
			namespaceFilters: nil,
			expectedErrMsg:   errors.New("invalid regex 'a(?=b)': error parsing regexp: invalid or unsupported Perl syntax: `(?=`"),
			filterErrors:     []string{"invalid regex 'a(?=b)': error parsing regexp: invalid or unsupported Perl syntax: `(?=`"},
		},
		{
			desc:             "invalid filter prefix, valid regex",
			filters:          []string{"image:redis.*", "invalid", "name:dd-.*", "kube_namespace:dev-.*", "name:abc", "also invalid"},
			imageFilters:     []*regexp.Regexp{regexp.MustCompile("redis.*")},
			nameFilters:      []*regexp.Regexp{regexp.MustCompile("dd-.*"), regexp.MustCompile("abc")},
			namespaceFilters: []*regexp.Regexp{regexp.MustCompile("dev-.*")},
			expectedErrMsg:   nil,
			filterErrors: []string{
				"Container filter \"invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'",
				"Container filter \"also invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'",
			},
		},
		{
			desc:             "invalid regex and invalid filter prefix",
			filters:          []string{"invalid", "name:a(?=b)", "image:apache.*", "kube_namespace:?", "also invalid", "name:abc"},
			imageFilters:     nil,
			nameFilters:      nil,
			namespaceFilters: nil,
			expectedErrMsg:   errors.New("invalid regex 'a(?=b)': error parsing regexp: invalid or unsupported Perl syntax: `(?=`"),
			filterErrors: []string{
				"invalid regex 'a(?=b)': error parsing regexp: invalid or unsupported Perl syntax: `(?=`",
				"invalid regex '?': error parsing regexp: missing argument to repetition operator: `?`",
				"Container filter \"invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'",
				"Container filter \"also invalid\" is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'",
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", filters, tc.desc), func(t *testing.T) {
			imageFilters, nameFilters, namespaceFilters, filterErrors, err := parseFilters(tc.filters)
			assert.Equal(t, tc.imageFilters, imageFilters)
			assert.Equal(t, tc.nameFilters, nameFilters)
			assert.Equal(t, tc.namespaceFilters, namespaceFilters)
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
