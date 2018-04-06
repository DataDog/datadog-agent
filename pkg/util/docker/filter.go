// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// TODO: move to pkg/util/container once it does not
// import pkg/util/docker anymore (circular import)
// It's already decoupled from that package

const (
	// pauseContainerGCR regex matches:
	// - k8s.gcr.io/pause-amd64:3.1
	// - asia.gcr.io/google_containers/pause-amd64:3.0
	// - gcr.io/google_containers/pause-amd64:3.0
	pauseContainerGCR        = `image:(.*)gcr\.io(/google_containers/|/)pause(.*)`
	pauseContainerOpenshift  = "image:openshift/origin-pod"
	pauseContainerKubernetes = "image:kubernetes/pause"
)

// Filter holds the state for the container filtering logic
type Filter struct {
	Enabled        bool
	ImageWhitelist []*regexp.Regexp
	NameWhitelist  []*regexp.Regexp
	ImageBlacklist []*regexp.Regexp
	NameBlacklist  []*regexp.Regexp
}

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
		blacklist = append(blacklist, pauseContainerGCR, pauseContainerOpenshift, pauseContainerKubernetes)
	}
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
