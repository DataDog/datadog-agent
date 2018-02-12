// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"fmt"
	"regexp"
	"strings"
)

type containerFilter struct {
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

// NewcontainerFilter creates a new container filter from a two slices of
// regexp patterns for a whitelist and blacklist. Each pattern should have
// the following format: "field:pattern" where field can be: [image, name].
// An error is returned if any of the expression don't compile.
func newContainerFilter(whitelist, blacklist []string) (*containerFilter, error) {
	iwl, nwl, err := parseFilters(whitelist)
	if err != nil {
		return nil, err
	}
	ibl, nbl, err := parseFilters(blacklist)
	if err != nil {
		return nil, err
	}

	return &containerFilter{
		Enabled:        len(whitelist) > 0 || len(blacklist) > 0,
		ImageWhitelist: iwl,
		NameWhitelist:  nwl,
		ImageBlacklist: ibl,
		NameBlacklist:  nbl,
	}, nil
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance.
func (cf containerFilter) IsExcluded(container *Container) bool {
	return cf.computeIsExcluded(container.Name, container.Image)
}

func (cf containerFilter) computeIsExcluded(containerName, containerImage string) bool {
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
