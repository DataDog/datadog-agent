// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"os"
	"unsafe"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// SymlinkResolver defines a symlink resolver
type SymlinkResolver struct {
	cache map[unsafe.Pointer]*eval.StringValues
}

// Resolve resolves the given symlinks
func (s *SymlinkResolver) Resolve(key unsafe.Pointer, path ...string) *eval.StringValues {
	if values, exists := s.cache[key]; exists {
		return values
	}

	values := &eval.StringValues{}

	for _, p := range path {
		// re-add existing value
		values.AppendScalarValue(p)

		dest, err := os.Readlink(p)
		if err != nil {
			continue
		}
		seclog.Tracef("new symlink resolved : src %s dst %s\n", p, dest)
		values.AppendScalarValue(dest)
	}

	s.cache[key] = values

	return values
}

// NewSymLinkResolver returns a new symlink resolver
func NewSymLinkResolver() *SymlinkResolver {
	return &SymlinkResolver{
		cache: make(map[unsafe.Pointer]*eval.StringValues),
	}
}
