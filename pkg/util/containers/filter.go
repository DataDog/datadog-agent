// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// KubeNamespaceFilterPrefix if the prefix used for Kubernetes namespaces
	KubeNamespaceFilterPrefix = `kube_namespace:`

	// Pause container image names that should be filtered out.
	// Where appropriate, each constant is loosely structured as
	// image:domain.*/pause.*

	pauseContainerKubernetes = "image:kubernetes/pause"
	pauseContainerECS        = "image:amazon/amazon-ecs-pause"
	pauseContainerFargate    = "image:aws-fargate-pause"
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
	imageFilterPrefix = `image:`
	nameFilterPrefix  = `name:`

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

func parseFilters(filters []string) (imageFilters, nameFilters, namespaceFilters []*regexp.Regexp, filterErrs []string) {
	for _, filter := range filters {
		switch {
		case strings.HasPrefix(filter, imageFilterPrefix):
			filter = PreprocessImageFilter(filter)
			r, err := filterToRegex(filter, imageFilterPrefix)
			if err != nil {
				filterErrs = append(filterErrs, err.Error())
				continue
			}
			imageFilters = append(imageFilters, r)
		case strings.HasPrefix(filter, nameFilterPrefix):
			r, err := filterToRegex(filter, nameFilterPrefix)
			if err != nil {
				filterErrs = append(filterErrs, err.Error())
				continue
			}
			nameFilters = append(nameFilters, r)
		case strings.HasPrefix(filter, KubeNamespaceFilterPrefix):
			r, err := filterToRegex(filter, KubeNamespaceFilterPrefix)
			if err != nil {
				filterErrs = append(filterErrs, err.Error())
				continue
			}
			namespaceFilters = append(namespaceFilters, r)
		default:
			warnmsg := fmt.Sprintf("Container filter %q is unknown, ignoring it. The supported filters are 'image', 'name' and 'kube_namespace'", filter)
			log.Warn(warnmsg)
			filterErrs = append(filterErrs, warnmsg)

		}
	}

	return imageFilters, nameFilters, namespaceFilters, filterErrs
}

// PreprocessImageFilter modifies image filters having the format `name$`, where {name} doesn't include a colon (e.g. nginx$, ^nginx$), to
// `name:.*`.
// This is done so that image filters can still match even if the matched image contains the tag or digest.
func PreprocessImageFilter(imageFilter string) string {
	regexVal := strings.TrimPrefix(imageFilter, imageFilterPrefix)
	if strings.HasSuffix(regexVal, "$") && !strings.Contains(regexVal, ":") {
		mutatedRegexVal := regexVal[:len(regexVal)-1] + "(@sha256)?:.*"
		return imageFilterPrefix + mutatedRegexVal
	}

	return imageFilter
}

// filterToRegex checks a filter's regex
func filterToRegex(filter string, filterPrefix string) (*regexp.Regexp, error) {
	pat := strings.TrimPrefix(filter, filterPrefix)
	r, err := regexp.Compile(pat)
	if err != nil {
		errormsg := fmt.Errorf("invalid regex '%s': %s", pat, err)
		return nil, errormsg
	}
	return r, nil
}

// GetPauseContainerExcludeList returns the exclude list for pause containers
func GetPauseContainerExcludeList() []string {
	return []string{
		pauseContainerGCR,
		pauseContainerOpenshift,
		pauseContainerOpenshift3,
		pauseContainerKubernetes,
		pauseContainerGoogle,
		pauseContainerAzure,
		pauseContainerECS,
		pauseContainerFargate,
		pauseContainerEKS,
		pauseContainerRancher,
		pauseContainerRancherMirrored,
		pauseContainerMCR,
		pauseContainerWin,
		pauseContainerAKS,
		pauseContainerECR,
		pauseContainerUpstream,
		pauseContainerCDK,
		pauseContainerGiantSwarm,
		pauseContainerRegistryK8sIo,
	}
}

// GetPauseContainerFilter returns a filter only excluding pause containers
func GetPauseContainerFilter() (*Filter, error) {
	var excludeList []string
	if pkgconfigsetup.Datadog().GetBool("exclude_pause_container") {
		excludeList = GetPauseContainerExcludeList()
	}

	return NewFilter(GlobalFilter, nil, excludeList)
}

// NewFilter creates a new container filter from two slices of
// regexp patterns for a include list and exclude list. Each pattern should have
// the following format: "field:pattern" where field can be: [image, name, kube_namespace].
// An error is returned if any of the expression don't compile or if any filter field is not
// recognized
func NewFilter(ft FilterType, includeList, excludeList []string) (*Filter, error) {
	imgIncl, nameIncl, nsIncl, filterErrsIncl := parseFilters(includeList)
	imgExcl, nameExcl, nsExcl, filterErrsExcl := parseFilters(excludeList)

	var lastError error

	filterErrs := append(filterErrsIncl, filterErrsExcl...)
	errorsMap := make(map[string]struct{})
	if len(filterErrs) > 0 {
		for _, err := range filterErrs {
			errorsMap[err] = struct{}{}
		}

		lastError = errors.New(filterErrs[len(filterErrs)-1])
	}

	return &Filter{
		FilterType:           ft,
		Enabled:              len(includeList) > 0 || len(excludeList) > 0,
		ImageIncludeList:     imgIncl,
		NameIncludeList:      nameIncl,
		NamespaceIncludeList: nsIncl,
		ImageExcludeList:     imgExcl,
		NameExcludeList:      nameExcl,
		NamespaceExcludeList: nsExcl,
		Errors:               errorsMap,
	}, lastError
}

// GetResult returns a workloadfilter.Result indicating if the container should be included, excluded or unknown.
// Note: exclude filters are not applied to empty container names, empty images and empty namespaces.
//
// containerImage may or may not contain the image tag or image digest. (e.g. nginx:latest and nginx are both valid)
func (cf Filter) GetResult(annotations map[string]string, containerName, containerImage, podNamespace string) workloadfilter.Result {

	// If containerImage doesn't include the tag or digest, add a colon so that it
	// can match image filters
	if len(containerImage) > 0 && !strings.Contains(containerImage, ":") {
		containerImage += ":"
	}

	if cf.isExcludedByAnnotation(annotations, containerName) {
		return workloadfilter.Excluded
	}

	if !cf.Enabled {
		return workloadfilter.Unknown
	}

	// Any includeListed take precedence on excluded
	for _, r := range cf.ImageIncludeList {
		if r.MatchString(containerImage) {
			return workloadfilter.Included
		}
	}
	for _, r := range cf.NameIncludeList {
		if r.MatchString(containerName) {
			return workloadfilter.Included
		}
	}
	for _, r := range cf.NamespaceIncludeList {
		if r.MatchString(podNamespace) {
			return workloadfilter.Included
		}
	}

	// Check if excludeListed
	if containerImage != "" {
		for _, r := range cf.ImageExcludeList {
			if r.MatchString(containerImage) {
				return workloadfilter.Excluded
			}
		}
	}

	if containerName != "" {
		for _, r := range cf.NameExcludeList {
			if r.MatchString(containerName) {
				return workloadfilter.Excluded
			}
		}
	}

	if podNamespace != "" {
		for _, r := range cf.NamespaceExcludeList {
			if r.MatchString(podNamespace) {
				return workloadfilter.Excluded
			}
		}
	}

	return workloadfilter.Unknown
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance.
// Note: exclude filters are not applied to empty container names, empty
// images and empty namespaces.
//
// containerImage may or may not contain the image tag or image digest. (e.g. nginx:latest and nginx are both valid)
func (cf Filter) IsExcluded(annotations map[string]string, containerName, containerImage, podNamespace string) bool {
	return cf.GetResult(annotations, containerName, containerImage, podNamespace) == workloadfilter.Excluded
}

// isExcludedByAnnotation identifies whether a container should be excluded
// based on the contents of the supplied annotations.
func (cf Filter) isExcludedByAnnotation(annotations map[string]string, containerName string) bool {
	if annotations == nil {
		return false
	}
	switch cf.FilterType {
	case GlobalFilter:
	case MetricsFilter:
		if IsExcludedByAnnotationInner(annotations, containerName, "metrics_") {
			return true
		}
	case LogsFilter:
		if IsExcludedByAnnotationInner(annotations, containerName, "logs_") {
			return true
		}
	default:
		log.Warnf("unrecognized filter type: %s", cf.FilterType)
	}
	return IsExcludedByAnnotationInner(annotations, containerName, "")
}

// IsExcludedByAnnotationInner checks if an entity is excluded by annotations.
func IsExcludedByAnnotationInner(annotations map[string]string, containerName string, excludePrefix string) bool {
	var e bool
	// try container-less annotations first
	exclude, found := annotations[fmt.Sprintf(kubeAutodiscoveryAnnotation, excludePrefix)]
	if found {
		if e, _ = strconv.ParseBool(exclude); e {
			return true
		}
	}

	// Check if excluded at container level
	exclude, found = annotations[fmt.Sprintf(kubeAutodiscoveryContainerAnnotation, containerName, excludePrefix)]
	if found {
		e, _ = strconv.ParseBool(exclude)
	}
	return e
}
