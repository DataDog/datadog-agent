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
	// pauseContainerGCR regex matches:
	// - k8s.gcr.io/pause-amd64:3.1
	// - asia.gcr.io/google_containers/pause-amd64:3.0
	// - gcr.io/google_containers/pause-amd64:3.0
	// - gcr.io/gke-release/pause-win:1.1.0
	pauseContainerGCR        = `image:(.*)gcr\.io(/google_containers/|/gke-release/|/)pause(.*)`
	pauseContainerOpenshift3 = "image:(openshift/origin-pod|(.*)rhel7/pod-infrastructure)"
	pauseContainerKubernetes = "image:kubernetes/pause"
	pauseContainerECS        = "image:amazon/amazon-ecs-pause"
	pauseContainerEKS        = "image:(amazonaws.com/)?eks/pause-(amd64|windows)"
	// pauseContainerAzure regex matches:
	// - k8s-gcrio.azureedge.net/pause-amd64
	// - gcrio.azureedge.net/google_containers/pause-amd64
	pauseContainerAzure   = `image:(.*)azureedge\.net(/google_containers/|/)pause(.*)`
	pauseContainerRancher = `image:rancher/pause(.*)`
	// pauseContainerAKS regex matches:
	// - mcr.microsoft.com/k8s/core/pause-amd64
	// - aksrepos.azurecr.io/mirror/pause-amd64
	pauseContainerAKS = `image:(mcr.microsoft.com/k8s/core/|aksrepos.azurecr.io/mirror/|kubeletwin/)pause(.*)`
	pauseContainerECR = `image:ecr(.*)amazonaws.com/pause(.*)`
)

// Filter holds the state for the container filtering logic
type Filter struct {
	Enabled            bool
	ImageWhitelist     []*regexp.Regexp
	NameWhitelist      []*regexp.Regexp
	NamespaceWhitelist []*regexp.Regexp
	ImageBlacklist     []*regexp.Regexp
	NameBlacklist      []*regexp.Regexp
	NamespaceBlacklist []*regexp.Regexp
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

// GetSharedFilter allows to share the result of NewFilterFromConfig
// for several user classes
func GetSharedFilter() (*Filter, error) {
	if sharedFilter != nil {
		return sharedFilter, nil
	}
	f, err := NewFilterFromConfig()
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
// regexp patterns for a whitelist and blacklist. Each pattern should have
// the following format: "field:pattern" where field can be: [image, name].
// An error is returned if any of the expression don't compile.
func NewFilter(whitelist, blacklist []string) (*Filter, error) {
	iwl, nwl, nswl, err := parseFilters(whitelist)
	if err != nil {
		return nil, err
	}
	ibl, nbl, nsbl, err := parseFilters(blacklist)
	if err != nil {
		return nil, err
	}

	return &Filter{
		Enabled:            len(whitelist) > 0 || len(blacklist) > 0,
		ImageWhitelist:     iwl,
		NameWhitelist:      nwl,
		NamespaceWhitelist: nswl,
		ImageBlacklist:     ibl,
		NameBlacklist:      nbl,
		NamespaceBlacklist: nsbl,
	}, nil
}

// NewFilterFromConfig creates a new container filter, sourcing patterns
// from the pkg/config options
func NewFilterFromConfig() (*Filter, error) {
	whitelist := config.Datadog.GetStringSlice("container_include")
	blacklist := config.Datadog.GetStringSlice("container_exclude")
	if len(whitelist) == 0 {
		// support legacy "ac_include" config
		whitelist = config.Datadog.GetStringSlice("ac_include")
	}
	if len(blacklist) == 0 {
		// support legacy "ac_exclude" config
		blacklist = config.Datadog.GetStringSlice("ac_exclude")
	}

	if config.Datadog.GetBool("exclude_pause_container") {
		blacklist = append(blacklist,
			pauseContainerGCR,
			pauseContainerOpenshift3,
			pauseContainerKubernetes,
			pauseContainerAzure,
			pauseContainerECS,
			pauseContainerEKS,
			pauseContainerRancher,
			pauseContainerAKS,
			pauseContainerECR,
		)
	}
	return NewFilter(whitelist, blacklist)
}

// NewAutodiscoveryFilter creates a new container filter for Autodiscovery
// It sources patterns from the pkg/config options but ignores the exclude_pause_container options
// It allows to filter metrics and logs separately
// For use in autodiscovery.
func NewAutodiscoveryFilter(filter FilterType) (*Filter, error) {
	whitelist := []string{}
	blacklist := []string{}
	switch filter {
	case GlobalFilter:
		whitelist = config.Datadog.GetStringSlice("container_include")
		blacklist = config.Datadog.GetStringSlice("container_exclude")
		if len(whitelist) == 0 {
			// fallback and support legacy "ac_include" config
			whitelist = config.Datadog.GetStringSlice("ac_include")
		}
		if len(blacklist) == 0 {
			// fallback and support legacy "ac_exclude" config
			blacklist = config.Datadog.GetStringSlice("ac_exclude")
		}
	case MetricsFilter:
		whitelist = config.Datadog.GetStringSlice("container_include_metrics")
		blacklist = config.Datadog.GetStringSlice("container_exclude_metrics")
	case LogsFilter:
		whitelist = config.Datadog.GetStringSlice("container_include_logs")
		blacklist = config.Datadog.GetStringSlice("container_exclude_logs")
	}
	return NewFilter(whitelist, blacklist)
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance.
func (cf Filter) IsExcluded(containerName, containerImage, podNamespace string) bool {
	if !cf.Enabled {
		return false
	}

	// Any whitelisted take precedence on excluded
	for _, r := range cf.ImageWhitelist {
		if r.MatchString(containerImage) {
			return false
		}
	}
	for _, r := range cf.NameWhitelist {
		if r.MatchString(containerName) {
			return false
		}
	}
	for _, r := range cf.NamespaceWhitelist {
		if r.MatchString(podNamespace) {
			return false
		}
	}

	// Check if blacklisted
	for _, r := range cf.ImageBlacklist {
		if r.MatchString(containerImage) {
			return true
		}
	}
	for _, r := range cf.NameBlacklist {
		if r.MatchString(containerName) {
			return true
		}
	}
	for _, r := range cf.NamespaceBlacklist {
		if r.MatchString(podNamespace) {
			return true
		}
	}

	return false
}
