// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// OpenFilesLimitWarningType is the key of the message generated when too many
// files are tailed
const openFilesLimitWarningType = "open_files_limit_warning"

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
// they are just returned in reverse lexicographical order, see `searchFiles`
func (p *Provider) FilesToTail(sources []*config.LogSource) []*File {
	var filesToTail []*File
	shouldLogErrors := p.shouldLogErrors
	p.shouldLogErrors = false // Let's log errors on first run only

	for i := 0; i < len(sources); i++ {
		source := sources[i]
		tailedFileCounter := 0
		files, err := p.CollectFiles(source)
		isWildcardPath := config.ContainsWildcard(source.Config.Path)
		if err != nil {
			source.Status.Error(err)
			if isWildcardPath {
				source.Messages.AddMessage(source.Config.Path, fmt.Sprintf("%d files tailed out of %d files matching", tailedFileCounter, len(files)))
			}
			if shouldLogErrors {
				log.Warnf("Could not collect files: %v", err)
			}
			continue
		}
		for j := 0; j < len(files) && len(filesToTail) < p.filesLimit; j++ {
			file := files[j]
			filesToTail = append(filesToTail, file)
			tailedFileCounter++
		}

		if len(filesToTail) >= p.filesLimit {
			status.AddGlobalWarning(
				openFilesLimitWarningType,
				fmt.Sprintf(
					"The limit on the maximum number of files in use (%d) has been reached. Increase this limit (thanks to the attribute logs_config.open_files_limit in datadog.yaml) or decrease the number of tailed file.",
					p.filesLimit,
				),
			)
		} else {
			status.RemoveGlobalWarning(openFilesLimitWarningType)
		}

		if isWildcardPath {
			source.Messages.AddMessage(source.Config.Path, fmt.Sprintf("%d files tailed out of %d files matching", tailedFileCounter, len(files)))
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
			NewFile(path, source, false),
		}, nil
	case config.ContainsWildcard(path):
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
		return nil, fmt.Errorf("could not find any file matching pattern %s, check that all its subdirectories are executable", pattern)
	}
	var files []*File

	// Files are sorted because of a heuristic on the filename: often the filename and/or the folder name
	// contains information in the file datetime. Most of the time we want the most recent files.
	// Here, we reverse paths to have stable sort keep reverse lexicographical order w.r.t dir names. Example:
	// [/tmp/1/2017.log, /tmp/1/2018.log, /tmp/2/2018.log] becomes [/tmp/2/2018.log, /tmp/1/2018.log, /tmp/1/2017.log]
	// then kept as is by the sort below.
	// https://github.com/golang/go/wiki/SliceTricks#reversing
	for i := len(paths)/2 - 1; i >= 0; i-- {
		opp := len(paths) - 1 - i
		paths[i], paths[opp] = paths[opp], paths[i]
	}
	// sort paths by descending filenames
	sort.SliceStable(paths, func(i, j int) bool {
		return filepath.Base(paths[i]) > filepath.Base(paths[j])
	})

	// Resolve excluded path(s)
	excludedPaths := make(map[string]int)
	for _, excludePattern := range source.Config.ExcludePaths {
		excludedGlob, err := filepath.Glob(excludePattern)
		if err != nil {
			return nil, fmt.Errorf("malformed exclusion pattern: %s, %s", excludePattern, err)
		}
		for _, excludedPath := range excludedGlob {
			log.Debugf("Adding excluded path: %s", excludedPath)
			excludedPaths[excludedPath]++
			if excludedPaths[excludedPath] > 1 {
				log.Debugf("Overlapping excluded path: %s", excludedPath)
			}
		}
	}

	for _, path := range paths {
		if excludedPaths[path] == 0 {
			files = append(files, NewFile(path, source, true))
		}
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
