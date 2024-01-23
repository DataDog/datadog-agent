// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"errors"
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
	panic("not called")
}
