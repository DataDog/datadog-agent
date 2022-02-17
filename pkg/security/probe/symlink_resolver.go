// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/twmb/murmur3"
)

// Targets defines the mapping between a path field and all its values
type Targets struct {
	Field  eval.Field
	Values *eval.StringValues
}

// SymlinkResolver defines a SymlinkResolver
type SymlinkResolver struct {
	sync.RWMutex

	OnNewSymlink func(field eval.Field, path string)

	requests chan string
	cache    map[unsafe.Pointer]*eval.StringValues
	index    map[string][]*Targets
	added    map[string]bool
	root     map[uint64]bool

	state *eval.RuleState
}

// InitStringValues initialize the symlinks resolution, should be called only during the
// rules compilations
func (s *SymlinkResolver) InitStringValues(key unsafe.Pointer, field eval.Field, values *eval.StringValues, state *eval.RuleState) error {
	target := Targets{
		Field:  field,
		Values: values.Clone(),
	}

	// symlink resolver only works on scalar values. It can't resolve symlinks on patterns like /etc/*
	for _, path := range target.Values.GetScalarValues() {
		s.added[path] = true

		targets := s.index[path]
		targets = append(targets, &target)
		s.index[path] = targets
	}

	// add field values the the rule state
	for _, value := range values.GetFieldValues() {
		if err := state.UpdateFieldValues(field, value); err != nil {
			return err
		}
	}

	s.cache[key] = target.Values
	s.state = state

	return nil
}

func (s *SymlinkResolver) rootFingerPrint(root string) uint64 {
	folders := []string{"/etc", "/usr/bin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin"}

	hasher := murmur3.New64()
	for _, folder := range folders {
		target := filepath.Join(root, folder)
		files, err := ioutil.ReadDir(target)
		if err != nil {
			continue
		}

		for _, file := range files {
			data := fmt.Sprintf("%s:%d:%s", file.Name(), file.Size(), file.ModTime().String())
			hasher.Write([]byte(data))
		}
	}

	return hasher.Sum64()
}

// UpdateSymlinks updates the symlinks for the given path
func (s *SymlinkResolver) UpdateSymlinks(root string) {
	seclog.Tracef("update symlinks for `%s`", root)

	fp := s.rootFingerPrint(root)
	if _, exists := s.root[fp]; exists {
		return
	}
	s.root[fp] = true

	for path, targets := range s.index {
		src := filepath.Join(root, path)

		dest, err := filepath.EvalSymlinks(src)
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
			fieldValue := target.Values.AppendScalarValue(dest)
			if fieldValue != nil {
				// we need to update the state to updated the values belonging to a field
				// as this values will be used for the discarders
				_ = s.state.UpdateFieldValues(target.Field, *fieldValue)
			}

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
	s.Lock()
	defer s.Unlock()

	s.cache = make(map[unsafe.Pointer]*eval.StringValues)
	s.index = make(map[string][]*Targets)
	s.added = make(map[string]bool)
	s.root = make(map[uint64]bool)
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
