// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Targets defines the mapping between a path field and all its values
type Targets struct {
	Field  eval.Field
	Values eval.StringValues
}

// SymlinkResolver defines a SymlinkResolver
type SymlinkResolver struct {
	sync.RWMutex

	OnNewSymlink func(field eval.Field, path string)

	requests chan string
	cache    map[unsafe.Pointer]*eval.StringValues
	index    map[string][]*Targets
	added    map[string]bool
}

// InitStringValues initialize the symlinks resolution, should be called only during the
// rules compilations
func (s *SymlinkResolver) InitStringValues(key unsafe.Pointer, field eval.Field, paths ...string) {
	//var target eval.StringValues
	target := Targets{
		Field: field,
	}

	for _, path := range paths {
		target.Values.AppendScalarValue(path)
		s.added[path] = true

		targets := s.index[path]
		targets = append(targets, &target)
		s.index[path] = targets
	}

	s.cache[key] = &target.Values
}

// UpdateSymlinks updates the symlinks for the given path
func (s *SymlinkResolver) UpdateSymlinks(root string) {
	for path, targets := range s.index {
		dest, err := filepath.EvalSymlinks(filepath.Join(root, path))
		if err != nil {
			continue
		}

		if root != "/" {
			dest = strings.TrimPrefix(dest, root)
		}

		if s.added[dest] {
			continue
		}

		seclog.Tracef("symlink resolved %s(%s) => %s => %+v\n", path, root, dest, targets)

		s.Lock()
		for _, target := range targets {
			target.Values.AppendScalarValue(dest)
			s.added[dest] = true

			if s.OnNewSymlink != nil {
				s.OnNewSymlink(target.Field, dest)
			}
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

// ScheduleUpdate schedules a symlink update
func (s *SymlinkResolver) ScheduleUpdate(root string) {
	s.requests <- root
}

// Start start the resolver
func (s *SymlinkResolver) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case root := <-s.requests:
				s.UpdateSymlinks(root)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Reset all the caches
func (s *SymlinkResolver) Reset() {
	s.RLock()
	defer s.RUnlock()

	s.cache = make(map[unsafe.Pointer]*eval.StringValues)
	s.index = make(map[string][]*Targets)
	s.added = make(map[string]bool)
}

// NewSymLinkResolver returns a new
func NewSymLinkResolver(onNewSymlink func(field eval.Field, path string)) *SymlinkResolver {
	sr := &SymlinkResolver{
		OnNewSymlink: onNewSymlink,
		requests:     make(chan string, 1000),
	}
	sr.Reset()
	return sr
}
