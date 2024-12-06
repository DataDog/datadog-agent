// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package image provides utilities to handle container images for pkg/util/containers
package image

import (
	"errors"
	"os"
	"strings"
)

var (
	// ErrEmptyImage is returned when image name argument is empty
	ErrEmptyImage = errors.New("empty image name")
	// ErrImageIsSha256 is returned when image name argument is a sha256
	ErrImageIsSha256 = errors.New("invalid image name (is a sha256)")
)

// SplitImageName splits a valid image name (from ResolveImageName) and returns:
//   - the "long image name" with registry and prefix, without tag
//   - the registry
//   - the "short image name", without registry, prefix nor tag
//   - the image tag if present
//   - an error if parsing failed
func SplitImageName(image string) (string, string, string, string, error) {
	// See TestSplitImageName for supported formats (number 6 will surprise you!)
	if image == "" {
		return "", "", "", "", ErrEmptyImage
	}
	if strings.HasPrefix(image, "sha256:") {
		return "", "", "", "", ErrImageIsSha256
	}
	long := image
	if pos := strings.LastIndex(long, "@sha"); pos > 0 {
		// Remove @sha suffix when orchestrator is sha-pinning
		long = long[0:pos]
	}

	var registry, short, tag string
	lastColon := strings.LastIndex(long, ":")
	lastSlash := strings.LastIndex(long, "/")
	firstSlash := strings.Index(long, "/")

	if lastColon > -1 && lastColon > lastSlash {
		// We have a tag
		tag = long[lastColon+1:]
		long = long[:lastColon]
	}
	if lastSlash > -1 {
		// we have a prefix / registry
		short = long[lastSlash+1:]
	} else {
		short = long
	}
	if firstSlash > -1 && firstSlash != lastSlash {
		// we have a registry
		registry = long[:firstSlash]
	}
	return long, registry, short, tag, nil
}

// SanitizeHostPath changes the specified path by prepending the mount point of the host's filesystem
func SanitizeHostPath(path string) string {
	hostPath := os.Getenv("HOST_ROOT")
	if hostPath == "" {
		hostPath = "/host"
	}

	if index := strings.Index(path, "/var/lib"); index != -1 {
		return hostPath + path[index:]
	}

	return hostPath + path
}
