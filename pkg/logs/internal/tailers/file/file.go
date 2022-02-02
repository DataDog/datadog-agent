// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// File represents a file to tail
type File struct {
	Path string
	// IsWildcardPath is set to true when the File has been discovered
	// in a directory with wildcard(s) in the configuration.
	IsWildcardPath bool
	Source         *config.LogSource
}

// NewFile returns a new File
func NewFile(path string, source *config.LogSource, isWildcardPath bool) *File {
	return &File{
		Path:           path,
		Source:         source,
		IsWildcardPath: isWildcardPath,
	}
}

// GetScanKey returns a key used by the scanner to index the scanned file.
// If it is a file scanned for a container, it will use the format: <filepath>/<container_id>
// Otherwise, it will simply use the format: <filepath>
func (t *File) GetScanKey() string {
	if t.Source != nil && t.Source.Config != nil && t.Source.Config.Identifier != "" {
		return fmt.Sprintf("%s/%s", t.Path, t.Source.Config.Identifier)
	}
	return t.Path
}
