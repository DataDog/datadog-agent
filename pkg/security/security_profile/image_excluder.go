// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profile manager related files
package securityprofile

import (
	"fmt"
	"strings"
)

const imageTagWildcard = "*"

// imageExcluder matches workloads against a configured list of
// "image_name:image_tag" entries. The tag may be "*" to match any tag for a
// given image name.
type imageExcluder struct {
	// name -> set of tags ("*" sentinel means any tag matches)
	entries map[string]map[string]struct{}
}

// newImageExcluder parses the configured entries. Returns nil if the list is
// empty (no-op excluder).
func newImageExcluder(entries []string) (*imageExcluder, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	out := &imageExcluder{entries: make(map[string]map[string]struct{}, len(entries))}
	for _, entry := range entries {
		// Split on the last colon so registries with ":port" in the name survive.
		idx := strings.LastIndex(entry, ":")
		if idx <= 0 || idx == len(entry)-1 {
			return nil, fmt.Errorf("invalid excluded_images entry %q: expected \"image_name:image_tag\"", entry)
		}
		name, tag := entry[:idx], entry[idx+1:]

		tags, ok := out.entries[name]
		if !ok {
			tags = make(map[string]struct{})
			out.entries[name] = tags
		}
		tags[tag] = struct{}{}
	}
	return out, nil
}

// IsExcluded reports whether the given (image_name, image_tag) pair matches
// any configured entry. A nil receiver is a valid no-op excluder.
func (e *imageExcluder) IsExcluded(imageName, imageTag string) bool {
	if e == nil || imageName == "" {
		return false
	}
	tags, ok := e.entries[imageName]
	if !ok {
		return false
	}
	if _, ok := tags[imageTagWildcard]; ok {
		return true
	}
	_, ok = tags[imageTag]
	return ok
}
