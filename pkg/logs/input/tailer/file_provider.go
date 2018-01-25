// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tailer

import (
	"fmt"
	"path/filepath"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// File represents a file to tail
type File struct {
	Path   string
	Source *config.LogSource
}

// NewFile returns a new File
func NewFile(path string, source *config.LogSource) *File {
	return &File{
		Path:   path,
		Source: source,
	}
}

// FileProvider implements the logic to retrieve at most filesLimit Files defined in sources
type FileProvider struct {
	sources    []*config.LogSource
	filesLimit int
}

// NewFileProvider returns a new FileProvider
func NewFileProvider(sources []*config.LogSource, filesLimit int) *FileProvider {
	return &FileProvider{
		sources:    sources,
		filesLimit: filesLimit,
	}
}

// FilesToTail returns all the Files matching paths in sources,
// it cannot return more than filesLimit Files.
// For now, there is no way to prioritize specific Files over others,
// they are just returned in alphabetical order
func (r *FileProvider) FilesToTail() []*File {
	filesToTail := []*File{}
	for _, source := range r.sources {
		// search all files matching pattern and append them all until filesLimit is reached
		pattern := source.Config.Path
		paths, err := filepath.Glob(pattern)
		if err != nil {
			err := fmt.Errorf("Malformed pattern, could not find any file: %s", pattern)
			source.Status.Error(err)
			log.Error(err)
			continue
		}
		if len(paths) == 0 {
			err := fmt.Errorf("No file are matching pattern: %s", pattern)
			source.Status.Error(err)
			log.Error(err)
			continue
		}
		for _, path := range paths {
			if len(filesToTail) == r.filesLimit {
				log.Warn("Reached the limit on the maximum number of files in use: ", r.filesLimit)
				return filesToTail
			}
			filesToTail = append(filesToTail, NewFile(path, source))
		}
	}
	return filesToTail
}
