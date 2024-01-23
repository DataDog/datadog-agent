// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"regexp"
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
	// rancher/mirrored-pause:3.7
	pauseContainerRancherMirrored = `image:rancher/mirrored-pause.*`
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
	// - cdk/pause-amd64
	pauseContainerCDK = `image:cdk/pause.*`
	// - giantswarm/pause
	pauseContainerGiantSwarm = `image:giantswarm/pause.*`
	// - registry.k8s.io/pause
	pauseContainerRegistryK8sIo = `image:registry\.k8s\.io/pause.*`

	// filter prefixes for inclusion/exclusion
	imageFilterPrefix         = `image:`
	nameFilterPrefix          = `name:`
	kubeNamespaceFilterPrefix = `kube_namespace:`

	// filter based on AD annotations
	kubeAutodiscoveryAnnotation          = "ad.datadoghq.com/%sexclude"
	kubeAutodiscoveryContainerAnnotation = "ad.datadoghq.com/%s.%sexclude"
)

// FilterType indicates the container filter type
type FilterType string

const (
	// GlobalFilter is used to cover both MetricsFilter and LogsFilter filter types
	GlobalFilter FilterType = "GlobalFilter"
	// MetricsFilter refers to the Metrics filter type
	MetricsFilter FilterType = "MetricsFilter"
	// LogsFilter refers to the Logs filter type
	LogsFilter FilterType = "LogsFilter"
)

// Filter holds the state for the container filtering logic
type Filter struct {
	FilterType           FilterType
	Enabled              bool
	ImageIncludeList     []*regexp.Regexp
	NameIncludeList      []*regexp.Regexp
	NamespaceIncludeList []*regexp.Regexp
	ImageExcludeList     []*regexp.Regexp
	NameExcludeList      []*regexp.Regexp
	NamespaceExcludeList []*regexp.Regexp
	Errors               map[string]struct{}
}

var sharedFilter *Filter

func parseFilters(filters []string) (imageFilters, nameFilters, namespaceFilters []*regexp.Regexp, filterErrs []string, err error) {
	panic("not called")
}

// filterToRegex checks a filter's regex
func filterToRegex(filter string, filterPrefix string) (*regexp.Regexp, error) {
	panic("not called")
}

// GetSharedMetricFilter allows to share the result of NewFilterFromConfig
// for several user classes
func GetSharedMetricFilter() (*Filter, error) {
	panic("not called")
}

// GetPauseContainerFilter returns a filter only excluding pause containers
func GetPauseContainerFilter() (*Filter, error) {
	panic("not called")
}

// ResetSharedFilter is only to be used in unit tests: it resets the global
// filter instance to force re-parsing of the configuration.
func ResetSharedFilter() {
	panic("not called")
}

// GetFilterErrors retrieves a list of errors and warnings resulting from parseFilters
func GetFilterErrors() map[string]struct{} {
	panic("not called")
}

// NewFilter creates a new container filter from a two slices of
// regexp patterns for a include list and exclude list. Each pattern should have
// the following format: "field:pattern" where field can be: [image, name, kube_namespace].
// An error is returned if any of the expression don't compile.
func NewFilter(ft FilterType, includeList, excludeList []string) (*Filter, error) {
	panic("not called")
}

// newMetricFilterFromConfig creates a new container filter, sourcing patterns
// from the pkg/config options, to be used only for metrics
func newMetricFilterFromConfig() (*Filter, error) {
	panic("not called")
}

// NewAutodiscoveryFilter creates a new container filter for Autodiscovery
// It sources patterns from the pkg/config options but ignores the exclude_pause_container options
// It allows to filter metrics and logs separately
// For use in autodiscovery.
func NewAutodiscoveryFilter(ft FilterType) (*Filter, error) {
	panic("not called")
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance. Consider also using
// Note: exclude filters are not applied to empty container names, empty
// images and empty namespaces.
func (cf Filter) IsExcluded(annotations map[string]string, containerName, containerImage, podNamespace string) bool {
	panic("not called")
}

// isExcludedByAnnotation identifies whether a container should be excluded
// based on the contents of the supplied annotations.
func (cf Filter) isExcludedByAnnotation(annotations map[string]string, containerName string) bool {
	panic("not called")
}

func isExcludedByAnnotationInner(annotations map[string]string, containerName string, excludePrefix string) bool {
	panic("not called")
}
