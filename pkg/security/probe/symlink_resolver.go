// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// SymlinkResolver defines a symlink resolver
type SymlinkResolver struct {
	sync.RWMutex

	cache map[unsafe.Pointer]*eval.StringValues
	index map[string][]*eval.StringValues
	added map[string]bool
}

// InitStringValues initialize the symlinks resolution, should be called only during the
// rules compilations
func (s *SymlinkResolver) InitStringValues(key unsafe.Pointer, paths ...string) {
	var values eval.StringValues

	for _, path := range paths {
		values.AppendScalarValue(path)
		s.added[path] = true

		allValues := s.index[path]
		allValues = append(allValues, &values)
		s.index[path] = allValues
	}

	s.cache[key] = &values
}

func (s *SymlinkResolver) UpdateSymlinks(root string) {
	for path, allValues := range s.index {
		dest, err := filepath.EvalSymlinks(filepath.Join(root, path))
		if err != nil {
			continue
		}
		dest = strings.TrimPrefix(dest, root)

		if s.added[dest] {
			continue
		}

		seclog.Tracef("symlink resolved %s(%s) => %s\n", path, root, dest)

		s.Lock()
		for _, values := range allValues {
			values.AppendScalarValue(dest)
			s.added[dest] = true
		}
		s.Unlock()
	}
}

// GetStringValues returns the string values for the given key
func (s *SymlinkResolver) GetStringValues(key unsafe.Pointer) *eval.StringValues {
	s.RLock()
	defer s.RUnlock()

	if values := s.cache[key]; values != nil {
		return values
	}

	// return empty values to avoid crash, this should never append except during reload
	return &eval.StringValues{}
}

// Reset all the caches
func (s *SymlinkResolver) Reset() {
	s.RLock()
	defer s.RUnlock()

	s.cache = make(map[unsafe.Pointer]*eval.StringValues)
	s.index = make(map[string][]*eval.StringValues)
	s.added = make(map[string]bool)
}

// NewSymLinkResolver returns a new
func NewSymLinkResolver() *SymlinkResolver {
	sr := &SymlinkResolver{}
	sr.Reset()
	return sr
}
