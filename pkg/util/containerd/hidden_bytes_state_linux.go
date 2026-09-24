// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && containerd

package containerd

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"path"
	"strings"
)

type hiddenData struct {
	size  uint64
	layer int
	refs  uint64
}

// HiddenBytesResult contains paired measurements from a complete image scan.
type HiddenBytesResult struct {
	Counts []uint64
	// UncompressedSize counts logical regular-file bytes across all layers,
	// including hidden versions, with same-layer hardlinks counted once.
	UncompressedSize uint64
}

type hiddenEntry struct {
	path     string
	mode     fs.FileMode
	data     *hiddenData
	whiteout bool
	opaque   bool
	implicit bool
}

// Both sources replay complete layers; markers only affect older layers.
type hiddenBytesState struct {
	ctx              context.Context
	limits           HiddenBytesLimits
	counts           []uint64
	uncompressedSize uint64
	visible          map[string]hiddenEntry
	entries          uint64
	pathBytes        uint64
	currentPathBytes uint64
}

func (s *hiddenBytesState) newData(size uint64, layer int) (*hiddenData, error) {
	if size > math.MaxUint64-s.uncompressedSize {
		return nil, errors.New("uncompressed image size overflow")
	}
	s.uncompressedSize += size
	return &hiddenData{size: size, layer: layer}, nil
}

func (s *hiddenBytesState) result() *HiddenBytesResult {
	return &HiddenBytesResult{Counts: s.counts, UncompressedSize: s.uncompressedSize}
}

func newHiddenBytesState(ctx context.Context, layers int, limits HiddenBytesLimits) (*hiddenBytesState, error) {
	if limits.MaxEntries == 0 || limits.MaxPathBytes == 0 || layers <= 0 {
		return nil, errors.New("hidden byte scan limits and layer count must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &hiddenBytesState{ctx: ctx, limits: limits, counts: make([]uint64, layers), visible: make(map[string]hiddenEntry)}, nil
}

func (s *hiddenBytesState) reserve(path string) error {
	if uint64(len(path)) > s.limits.MaxPathBytes-s.pathBytes {
		return errors.New("hidden byte scan path memory limit exceeded")
	}
	s.pathBytes += uint64(len(path))
	return nil
}

func (s *hiddenBytesState) trackEntry(path string) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	s.entries++
	if s.entries > s.limits.MaxEntries {
		return errors.New("hidden byte scan entry limit exceeded")
	}
	if err := s.reserve(path); err != nil {
		return err
	}
	s.currentPathBytes += uint64(len(path))
	return nil
}

func (s *hiddenBytesState) remove(path string, old hiddenEntry) error {
	if old.data != nil {
		if old.data.refs == 0 {
			return errors.New("invalid hidden byte reference count")
		}
		old.data.refs--
		if old.data.refs == 0 {
			if old.data.size > math.MaxUint64-s.counts[old.data.layer] {
				return errors.New("hidden byte count overflow")
			}
			s.counts[old.data.layer] += old.data.size
		}
	}
	delete(s.visible, path)
	s.pathBytes -= uint64(len(path))
	return nil
}

func (s *hiddenBytesState) hide(path string, childrenOnly bool) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if path != "." {
		old, exists := s.visible[path]
		if !exists {
			return nil
		}
		if !old.mode.IsDir() {
			if childrenOnly {
				return nil
			}
			return s.remove(path, old)
		}
	}
	for key, old := range s.visible {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		if (key == path && !childrenOnly) || strings.HasPrefix(key, path+"/") || path == "." {
			if err := s.remove(key, old); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *hiddenBytesState) applyLayer(current []hiddenEntry) error {
	for _, entry := range current {
		if entry.whiteout || entry.opaque {
			if err := s.hide(entry.path, entry.opaque); err != nil {
				return err
			}
		}
	}
	for _, entry := range current {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		if entry.whiteout || entry.path == "." {
			continue
		}
		for parent := path.Dir(entry.path); parent != "."; parent = path.Dir(parent) {
			if ancestor, ok := s.visible[parent]; !ok || !ancestor.mode.IsDir() {
				return errors.New("unsupported non-directory image path ancestor")
			}
		}
		old, exists := s.visible[entry.path]
		if entry.implicit && exists && !old.mode.IsDir() {
			return errors.New("implicit directory replaces non-directory image path")
		}
		if exists && !(entry.mode.IsDir() && old.mode.IsDir()) {
			if err := s.hide(entry.path, false); err != nil {
				return err
			}
		}
		if _, exists := s.visible[entry.path]; exists {
			s.pathBytes -= uint64(len(entry.path))
		}
		if err := s.reserve(entry.path); err != nil {
			return err
		}
		if entry.data != nil {
			entry.data.refs++
		}
		s.visible[entry.path] = entry
	}
	s.pathBytes -= s.currentPathBytes
	s.currentPathBytes = 0
	return nil
}
