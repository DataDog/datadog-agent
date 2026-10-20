// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fsstat reads filesystem metadata for a path opened through os.Root.
//
// Platform implementations must keep the filesystem query tied to a rooted
// handle for the target filesystem and revalidate the target identity. The
// caller uses ErrPathChanged to retry when a path component becomes a symlink
// or reparse point during resolution.
package fsstat

import (
	"errors"
	"os"
)

// Info is the normalized subset of filesystem metadata used by stat -f.
type Info struct {
	ID          uint64
	IDAvailable bool

	NameMax          uint64
	NameMaxAvailable bool

	TypeID          uint64
	TypeIDAvailable bool
	TypeName        string

	IOBlockSize          uint64
	FundamentalBlockSize uint64
	Blocks               uint64
	BlocksFree           uint64
	BlocksAvailable      uint64

	Files          uint64
	FilesFree      uint64
	FilesAvailable bool
}

// ErrPathChanged indicates that a path became a symlink or reparse point
// after sandbox resolution. Callers should resolve the original path again.
var ErrPathChanged = errors.New("path changed during filesystem stat")

// ErrNotSupported indicates that the current platform has no filesystem-stat
// backend.
var ErrNotSupported = errors.New("not supported on this platform")

// ErrNotDirectory indicates that a caller-required final target is not a
// directory.
var ErrNotDirectory = errors.New("not a directory")

// Read returns filesystem metadata for relPath beneath root. When
// requireDirectory is true, the identity-checked target must be a directory.
func Read(root *os.Root, relPath string, requireDirectory bool) (Info, error) {
	return read(root, relPath, requireDirectory)
}
