// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

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

// Provider implements the logic to retrieve at most filesLimit Files defined in sources
type Provider struct {
	filesLimit      int
	shouldLogErrors bool
}

// NewProvider returns a new Provider
func NewProvider(filesLimit int) *Provider {
	return &Provider{
		filesLimit:      filesLimit,
		shouldLogErrors: true,
	}
}

// FilesToTail returns all the Files matching paths in sources,
// it cannot return more than filesLimit Files.
// For now, there is no way to prioritize specific Files over others,
// they are just returned in alphabetical order
func (p *Provider) FilesToTail(sources []*config.LogSource) []*File {
	var filesToTail []*File
	shouldLogErrors := p.shouldLogErrors
	p.shouldLogErrors = false // Let's log errors on first run only

	for i := 0; i < len(sources) && len(filesToTail) < p.filesLimit; i++ {
		source := sources[i]
		files, err := p.CollectFiles(source)
		if err != nil {
			source.Status.Error(err)
			if shouldLogErrors {
				log.Warnf("Could not collect files: %v", err)
			}
			continue
		}
		for j := 0; j < len(files) && len(filesToTail) < p.filesLimit; j++ {
			file := files[j]
			filesToTail = append(filesToTail, file)
		}
	}

	if len(filesToTail) == p.filesLimit {
		log.Warn("Reached the limit on the maximum number of files in use: ", p.filesLimit)
		return filesToTail
	}

	return filesToTail
}

// CollectFiles returns all the files matching the source path.
func (p *Provider) CollectFiles(source *config.LogSource) ([]*File, error) {
	path := source.Config.Path
	fileExists := p.exists(path)
	switch {
	case fileExists:
		return []*File{
			NewFile(path, source),
		}, nil
	case p.containsWildcard(path):
		pattern := path
		return p.searchFiles(pattern, source)
	default:
		return nil, fmt.Errorf("file %s does not exist", path)
	}
}

// searchFiles returns all the files matching the source path pattern.
func (p *Provider) searchFiles(pattern string, source *config.LogSource) ([]*File, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("malformed pattern, could not find any file: %s", pattern)
	}
	if len(paths) == 0 {
		// no file was found, its parent directories might have wrong permissions or it just does not exist
		return nil, fmt.Errorf("could not find any file matching pattern %s, check that all its subdirectories are exectutable", pattern)
	}
	var files []*File
	for _, path := range paths {
		files = append(files, NewFile(path, source))
	}
	return files, nil
}

// exists returns true if the file at path filePath exists
// Note: we can't rely on os.IsNotExist for windows, so we check error nullity.
// As we're tailing with *, the error is related to the path being malformed.
func (p *Provider) exists(filePath string) bool {
	if _, err := os.Stat(filePath); err != nil {
		return false
	}
	return true
}

// containsWildcard returns true if the path contains any wildcard character
func (p *Provider) containsWildcard(path string) bool {
	return strings.ContainsAny(path, "*?[")
}
