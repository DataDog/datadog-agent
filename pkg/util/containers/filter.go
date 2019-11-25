// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	pauseContainerGCR        = `image:(.*)gcr\.io(/google_containers/|/)pause(.*)`
	pauseContainerOpenshift  = "image:openshift/origin-pod"
	pauseContainerKubernetes = "image:kubernetes/pause"
	pauseContainerECS        = "image:amazon/amazon-ecs-pause"
	pauseContainerEKS        = "image:eks/pause-amd64"
	// pauseContainerAzure regex matches:
	// - k8s-gcrio.azureedge.net/pause-amd64
	// - gcrio.azureedge.net/google_containers/pause-amd64
	pauseContainerAzure   = `image:(.*)azureedge\.net(/google_containers/|/)pause(.*)`
	pauseContainerRancher = `image:rancher/pause(.*)`
)

// Filter holds the state for the container filtering logic
type Filter struct {
	Enabled        bool
	ImageWhitelist []*regexp.Regexp
	NameWhitelist  []*regexp.Regexp
	ImageBlacklist []*regexp.Regexp
	NameBlacklist  []*regexp.Regexp
}

var sharedFilter *Filter

func parseFilters(filters []string) (imageFilters, nameFilters []*regexp.Regexp, err error) {
	for _, filter := range filters {
		switch {
		case strings.HasPrefix(filter, "image:"):
			pat := strings.TrimPrefix(filter, "image:")
			r, err := regexp.Compile(strings.TrimPrefix(pat, "image:"))
			if err != nil {
				return nil, nil, fmt.Errorf("invalid regex '%s': %s", pat, err)
			}
			imageFilters = append(imageFilters, r)
		case strings.HasPrefix(filter, "name:"):
			pat := strings.TrimPrefix(filter, "name:")
			r, err := regexp.Compile(pat)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid regex '%s': %s", pat, err)
			}
			nameFilters = append(nameFilters, r)
		}
	}
	return imageFilters, nameFilters, nil
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
	iwl, nwl, err := parseFilters(whitelist)
	if err != nil {
		return nil, err
	}
	ibl, nbl, err := parseFilters(blacklist)
	if err != nil {
		return nil, err
	}

	return &Filter{
		Enabled:        len(whitelist) > 0 || len(blacklist) > 0,
		ImageWhitelist: iwl,
		NameWhitelist:  nwl,
		ImageBlacklist: ibl,
		NameBlacklist:  nbl,
	}, nil
}

// NewFilterFromConfig creates a new container filter, sourcing patterns
// from the pkg/config options
func NewFilterFromConfig() (*Filter, error) {
	whitelist := config.Datadog.GetStringSlice("ac_include")
	blacklist := config.Datadog.GetStringSlice("ac_exclude")

	if config.Datadog.GetBool("exclude_pause_container") {
		blacklist = append(blacklist,
			pauseContainerGCR,
			pauseContainerOpenshift,
			pauseContainerKubernetes,
			pauseContainerAzure,
			pauseContainerECS,
			pauseContainerEKS,
			pauseContainerRancher,
		)
	}
	return NewFilter(whitelist, blacklist)
}

// NewFilterFromConfigIncludePause creates a new container filter, sourcing patterns
// from the pkg/config options, but ignoring the exclude_pause_container option, for
// use in autodiscovery
func NewFilterFromConfigIncludePause() (*Filter, error) {
	whitelist := config.Datadog.GetStringSlice("ac_include")
	blacklist := config.Datadog.GetStringSlice("ac_exclude")
	return NewFilter(whitelist, blacklist)
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance.
func (cf Filter) IsExcluded(containerName, containerImage string) bool {
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
	return false
}
