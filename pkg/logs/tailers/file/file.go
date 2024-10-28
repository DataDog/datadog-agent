// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package file

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// File represents a file to tail
type File struct {
	// Path contains the path to the file which should be tailed.
	Path string

	// IsWildcardPath is set to true when the File has been discovered
	// in a directory with wildcard(s) in the configuration.
	IsWildcardPath bool

	// Source is the ReplaceableSource that led to this File.
	Source *sources.ReplaceableSource
}

// NewFile returns a new File
func NewFile(path string, source *sources.LogSource, isWildcardPath bool) *File {
	return &File{
		Path:           path,
		Source:         sources.NewReplaceableSource(source),
		IsWildcardPath: isWildcardPath,
	}
}

// GetScanKey returns a key used by the scanner to index the scanned file.  The
// string uniquely identifies this File, even if sources for multiple
// containers use the same Path.
func (t *File) GetScanKey() string {
	// If it is a file scanned for a container, it will use the format: <filepath>/<container_id>
	// Otherwise, it will simply use the format: <filepath>
	if t.Source != nil && t.Source.Config() != nil && t.Source.Config().Identifier != "" {
		return fmt.Sprintf("%s/%s", t.Path, t.Source.Config().Identifier)
	}
	return t.Path
}
