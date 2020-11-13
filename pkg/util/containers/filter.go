// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package containers

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	// Pause container image names that should be filtered out.
	// Where appropriate, each constant is loosely structured as
	// image:domain.*/pause.*

	pauseContainerKubernetes = "image:kubernetes/pause"
	pauseContainerECS        = "image:amazon/amazon-ecs-pause"
	pauseContainerOpenshift  = "image:openshift/origin-pod"
	pauseContainerOpenshift3 = "image:.*rhel7/pod-infrastructure"

	// - asia.gcr.io/google-containers/pause-amd64
	// - gcr.io/google-containers/pause
	// - *.gcr.io/google_containers/pause
	// - *.jfrog.io/google_containers/pause
	pauseContainerGoogle = "image:google(_|-)containers/pause.*"

	// - k8s.gcr.io/pause-amd64:3.1
	// - asia.gcr.io/google_containers/pause-amd64:3.0
	// - gcr.io/google_containers/pause-amd64:3.0
	// - gcr.io/gke-release/pause-win:1.1.0
	// - eu.gcr.io/k8s-artifacts-prod/pause:3.3
	// - k8s.gcr.io/pause
	pauseContainerGCR = `image:.*gcr\.io(.*)/pause.*`

	// - k8s-gcrio.azureedge.net/pause-amd64
	// - gcrio.azureedge.net/google_containers/pause-amd64
	pauseContainerAzure = `image:.*azureedge\.net(.*)/pause.*`

	// amazonaws.com/eks/pause-windows:latest
	// eks/pause-amd64
	pauseContainerEKS = `image:(amazonaws\.com/)?eks/pause.*`
	// rancher/pause-amd64:3.0
	pauseContainerRancher = `image:rancher/pause.*`
	// - mcr.microsoft.com/k8s/core/pause-amd64
	pauseContainerMCR = `image:mcr\.microsoft\.com(.*)/pause.*`
	// - aksrepos.azurecr.io/mirror/pause-amd64
	pauseContainerAKS = `image:aksrepos\.azurecr\.io(.*)/pause.*`
	// - kubeletwin/pause:latest
	pauseContainerWin = `image:kubeletwin/pause.*`
	// - ecr.us-east-1.amazonaws.com/pause
	pauseContainerECR = `image:ecr(.*)amazonaws\.com/pause.*`
	// - *.ecr.us-east-1.amazonaws.com/upstream/pause
	pauseContainerUpstream = `image:upstream/pause.*`
)

// Filter holds the state for the container filtering logic
type Filter struct {
	Enabled              bool
	ImageIncludeList     []*regexp.Regexp
	NameIncludeList      []*regexp.Regexp
	NamespaceIncludeList []*regexp.Regexp
	ImageExcludeList     []*regexp.Regexp
	NameExcludeList      []*regexp.Regexp
	NamespaceExcludeList []*regexp.Regexp
}

var sharedFilter *Filter

func parseFilters(filters []string) (imageFilters, nameFilters, namespaceFilters []*regexp.Regexp, err error) {
	for _, filter := range filters {
		switch {
		case strings.HasPrefix(filter, "image:"):
			pat := strings.TrimPrefix(filter, "image:")
			r, err := regexp.Compile(strings.TrimPrefix(pat, "image:"))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid regex '%s': %s", pat, err)
			}
			imageFilters = append(imageFilters, r)
		case strings.HasPrefix(filter, "name:"):
			pat := strings.TrimPrefix(filter, "name:")
			r, err := regexp.Compile(pat)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid regex '%s': %s", pat, err)
			}
			nameFilters = append(nameFilters, r)
		case strings.HasPrefix(filter, "kube_namespace:"):
			pat := strings.TrimPrefix(filter, "kube_namespace:")
			r, err := regexp.Compile(pat)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid regex '%s': %s", pat, err)
			}
			namespaceFilters = append(namespaceFilters, r)
		}
	}
	return imageFilters, nameFilters, namespaceFilters, nil
}

// GetSharedMetricFilter allows to share the result of NewFilterFromConfig
// for several user classes
func GetSharedMetricFilter() (*Filter, error) {
	if sharedFilter != nil {
		return sharedFilter, nil
	}
	f, err := newMetricFilterFromConfig()
	if err != nil {
		return nil, err
	}
	sharedFilter = f
	return f, nil
}

// ResetSharedFilter is only to be used in unit tests: it resets the global
// filter instance to force re-parsing of the configuration.
func ResetSharedFilter() {
	sharedFilter = nil
}

// NewFilter creates a new container filter from a two slices of
// regexp patterns for a include list and exclude list. Each pattern should have
// the following format: "field:pattern" where field can be: [image, name].
// An error is returned if any of the expression don't compile.
func NewFilter(includeList, excludeList []string) (*Filter, error) {
	imgIncl, nameIncl, nsIncl, err := parseFilters(includeList)
	if err != nil {
		return nil, err
	}
	imgExcl, nameExcl, nsExcl, err := parseFilters(excludeList)
	if err != nil {
		return nil, err
	}

	return &Filter{
		Enabled:              len(includeList) > 0 || len(excludeList) > 0,
		ImageIncludeList:     imgIncl,
		NameIncludeList:      nameIncl,
		NamespaceIncludeList: nsIncl,
		ImageExcludeList:     imgExcl,
		NameExcludeList:      nameExcl,
		NamespaceExcludeList: nsExcl,
	}, nil
}

// newMetricFilterFromConfig creates a new container filter, sourcing patterns
// from the pkg/config options, to be used only for metrics
func newMetricFilterFromConfig() (*Filter, error) {
	// We merge `container_include` and `container_include_metrics` as this filter
	// is used by all core and python checks (so components sending metrics).
	includeList := config.Datadog.GetStringSlice("container_include")
	excludeList := config.Datadog.GetStringSlice("container_exclude")
	includeList = append(includeList, config.Datadog.GetStringSlice("container_include_metrics")...)
	excludeList = append(excludeList, config.Datadog.GetStringSlice("container_exclude_metrics")...)

	if len(includeList) == 0 {
		// support legacy "ac_include" config
		includeList = config.Datadog.GetStringSlice("ac_include")
	}
	if len(excludeList) == 0 {
		// support legacy "ac_exclude" config
		excludeList = config.Datadog.GetStringSlice("ac_exclude")
	}

	if config.Datadog.GetBool("exclude_pause_container") {
		excludeList = append(excludeList,
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
		)
	}
	return NewFilter(includeList, excludeList)
}

// NewAutodiscoveryFilter creates a new container filter for Autodiscovery
// It sources patterns from the pkg/config options but ignores the exclude_pause_container options
// It allows to filter metrics and logs separately
// For use in autodiscovery.
func NewAutodiscoveryFilter(filter FilterType) (*Filter, error) {
	includeList := []string{}
	excludeList := []string{}
	switch filter {
	case GlobalFilter:
		includeList = config.Datadog.GetStringSlice("container_include")
		excludeList = config.Datadog.GetStringSlice("container_exclude")
		if len(includeList) == 0 {
			// fallback and support legacy "ac_include" config
			includeList = config.Datadog.GetStringSlice("ac_include")
		}
		if len(excludeList) == 0 {
			// fallback and support legacy "ac_exclude" config
			excludeList = config.Datadog.GetStringSlice("ac_exclude")
		}
	case MetricsFilter:
		includeList = config.Datadog.GetStringSlice("container_include_metrics")
		excludeList = config.Datadog.GetStringSlice("container_exclude_metrics")
	case LogsFilter:
		includeList = config.Datadog.GetStringSlice("container_include_logs")
		excludeList = config.Datadog.GetStringSlice("container_exclude_logs")
	}
	return NewFilter(includeList, excludeList)
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance.
func (cf Filter) IsExcluded(containerName, containerImage, podNamespace string) bool {
	if !cf.Enabled {
		return false
	}

	// Any includeListed take precedence on excluded
	for _, r := range cf.ImageIncludeList {
		if r.MatchString(containerImage) {
			return false
		}
	}
	for _, r := range cf.NameIncludeList {
		if r.MatchString(containerName) {
			return false
		}
	}
	for _, r := range cf.NamespaceIncludeList {
		if r.MatchString(podNamespace) {
			return false
		}
	}

	// Check if excludeListed
	for _, r := range cf.ImageExcludeList {
		if r.MatchString(containerImage) {
			return true
		}
	}
	for _, r := range cf.NameExcludeList {
		if r.MatchString(containerName) {
			return true
		}
	}
	for _, r := range cf.NamespaceExcludeList {
		if r.MatchString(podNamespace) {
			return true
		}
	}

	return false
}
