// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"os"
)

type SymlinkResolver struct {
}

func (s *SymlinkResolver) Resolve(path string) (string, error) {
	dest, err := os.Readlink(path)
	if err != nil {
		return "", err
	}

	return dest, nil
}

func NewSymLinkResolver() *SymlinkResolver {
	return &SymlinkResolver{}
}
