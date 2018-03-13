// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tailer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
func (p *FileProvider) FilesToTail() []*File {
	filesToTail := []*File{}
	for i := 0; i < len(p.sources) && len(filesToTail) < p.filesLimit; i++ {
		source := p.sources[i]
		sourcePath := source.Config.Path
		if p.exists(sourcePath) {
			// no need to traverse the file system here as we found a file
			filesToTail = append(filesToTail, NewFile(sourcePath, source))
			continue
		}
		// search all files matching pattern and append them all until filesLimit is reached
		pattern := sourcePath
		paths, err := filepath.Glob(pattern)
		if err != nil {
			err := fmt.Errorf("Malformed pattern, could not find any file: %s", pattern)
			source.Status.Error(err)
			log.Error(err)
			continue
		}
		if len(paths) == 0 {
			// no file was found, its parent directories might have wrong permissions or it just does not exist
			if p.containsWildcard(pattern) {
				log.Warnf("Could not find any file matching pattern %s, check that all its subdirectories are exectutable", pattern)
			} else {
				log.Warnf("File %s does not exist", sourcePath)
			}
			continue
		}
		for j := 0; j < len(paths) && len(filesToTail) < p.filesLimit; j++ {
			path := paths[j]
			filesToTail = append(filesToTail, NewFile(path, source))
		}
	}
	if len(filesToTail) == p.filesLimit {
		log.Warn("Reached the limit on the maximum number of files in use: ", p.filesLimit)
		return filesToTail
	}

	return filesToTail
}

// exists returns true if the file at path filePath exists
// Note: we can't rely on os.IsNotExist for windows, so we check error nullity.
// As we're tailing with *, the error is related to the path being malformed.
func (p *FileProvider) exists(filePath string) bool {
	if _, err := os.Stat(filePath); err != nil {
		return false
	}
	return true
}

// containsWildcard returns true if the path contains any wildcard character
func (p *FileProvider) containsWildcard(path string) bool {
	return strings.ContainsAny(path, "*?[")
}
