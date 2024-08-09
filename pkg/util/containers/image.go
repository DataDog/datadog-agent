// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"errors"
	"strings"
)

var (
	// ErrEmptyImage is returned when image name argument is empty
	ErrEmptyImage = errors.New("empty image name")
	// ErrImageIsSha256 is returned when image name argument is a sha256
	ErrImageIsSha256 = errors.New("invalid image name (is a sha256)")
)

type StructImageName struct {
	Long     string
	Registry string
	Short    string
	Tag      string
	Digest   string
}

// SplitImageName splits a valid image name (from ResolveImageName) and returns:
//   - the "long image name" with registry and prefix, without tag
//   - the registry
//   - the "short image name", without registry, prefix nor tag
//   - the image tag if present
//   - an error if parsing failed
func SplitImageName(image string) (s StructImageName, err error) {
	// See TestSplitImageName for supported formats (number 6 will surprise you!)
	if image == "" {
		return StructImageName{}, ErrEmptyImage
	}
	if strings.HasPrefix(image, "sha256:") {
		return StructImageName{}, ErrImageIsSha256
	}
	s.Long = image
	if pos := strings.LastIndex(s.Long, "@sha"); pos > 0 {
		// Remove @sha suffix when orchestrator is sha-pinning
		s.Long = s.Long[0:pos]
		s.Digest = image[pos+1:]
	}

	lastColon := strings.LastIndex(s.Long, ":")
	lastSlash := strings.LastIndex(s.Long, "/")
	firstSlash := strings.Index(s.Long, "/")

	if lastColon > -1 && lastColon > lastSlash {
		// We have a tag
		s.Tag = s.Long[lastColon+1:]
		s.Long = s.Long[:lastColon]
	}
	if lastSlash > -1 {
		// we have a prefix / registry
		s.Short = s.Long[lastSlash+1:]
	} else {
		s.Short = s.Long
	}
	if firstSlash > -1 && firstSlash != lastSlash {
		// we have a registry
		s.Registry = s.Long[:firstSlash]
	}
	return s, nil
}
