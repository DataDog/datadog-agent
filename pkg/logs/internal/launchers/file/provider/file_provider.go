// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file_provider

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	tailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// OpenFilesLimitWarningType is the key of the message generated when too many
// files are tailed
const openFilesLimitWarningType = "open_files_limit_warning"

// sortOptions controls what ordering is applied to discovered files
type sortOptions string

const (
	SortReverseLexicographical = "SortReverseLexicographical"
	SortMtime                  = "SortMtime"
)

// selectionStrategy controls how the `filesLimit` slots we have are filled given a list of sources
type selectionStrategy string

const (
	// GreedySelection will consider each source one-by-one, filling as many
	// slots as is possible from that source before proceeding to the next one
	GreedySelection = "ChooseGreedily"
	// GlobalSelection will consider files from all sources together and will choose the
	// top `filesLimit` files based on the `sortMode` ordering
	GlobalSelection = "ChooseGlobally"
)

// FileProvider implements the logic to retrieve at most filesLimit Files defined in sources
type FileProvider struct {
	filesLimit      int
	sortMode        sortOptions
	selectionMode   selectionStrategy
	shouldLogErrors bool
}

// NewFileProvider returns a new Provider
func NewFileProvider(filesLimit int, sortMode sortOptions, selectionMode selectionStrategy) *FileProvider {
	return &FileProvider{
		filesLimit:      filesLimit,
		sortMode:        sortMode,
		selectionMode:   selectionMode,
		shouldLogErrors: true,
	}
}

// FilesToTail returns all the Files matching paths in sources,
// it cannot return more than filesLimit Files.
// Files are collected according to the fileProvider's sortMode and chooseStrategy
func (p *FileProvider) FilesToTail(sources []*sources.LogSource) []*tailer.File {
	var filesToTail []*tailer.File
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

// CollectFiles takes a 'LogSource' and produces a list of tailers matching this source
// with ordering defined by 'sortMode'
func (p *FileProvider) CollectFiles(source *sources.LogSource) ([]*tailer.File, error) {
	path := source.Config.Path
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return []*tailer.File{
			tailer.NewFile(path, source, false),
		}, nil
	case config.ContainsWildcard(path):
		pattern := path
		paths, err := p.searchFiles(pattern)
		if err != nil {
			return nil, err
		}
		p.applyOrdering(paths)
		files, err := createIncludedTailers(paths, source)

		return files, err
	default:
		return nil, fmt.Errorf("cannot read file %s: %s", path, err)
	}
}

// searchFiles returns all the files matching the source path pattern.
func (p *FileProvider) searchFiles(pattern string) ([]string, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("malformed pattern, could not find any file: %s", pattern)
	}
	if len(paths) == 0 {
		// no file was found, its parent directories might have wrong permissions or it just does not exist
		return nil, fmt.Errorf("could not find any file matching pattern %s, check that all its subdirectories are executable", pattern)
	}

	return paths, nil
}

// applyOrdering sorts the 'paths' slice in-place by the currently configured 'sortMode'
// While 'paths' are just strings, they _must_ exist on the filesystem. Otherwise behavior is undefined.
func (p *FileProvider) applyOrdering(paths []string) {
	if p.sortMode == SortMtime {
		// sort paths descending by mtime
		sort.SliceStable(paths, func(i, j int) bool {
			statI, _ := os.Stat(paths[i])
			statJ, _ := os.Stat(paths[j])

			return statI.ModTime().After(statJ.ModTime())
		})
	} else if p.sortMode == SortReverseLexicographical {
		// FIXME - this codepath assumes that the 'paths' will arrive in lexicographical order
		// This is true in the current go implementation, but it is unsafe to assume
		// https://cs.opensource.google/go/go/+/refs/tags/go1.19:src/path/filepath/match.go;l=363;drc=e4b624eae5fa3c51b8ca808da29442d3e3aaef04
		// https://github.com/golang/go/issues/17153
		//
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
	}

}

func createIncludedTailers(paths []string, source *sources.LogSource) ([]*tailer.File, error) {
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

	var files []*tailer.File
	for _, path := range paths {
		if excludedPaths[path] == 0 {
			files = append(files, tailer.NewFile(path, source, true))
		}
	}
	return files, nil
}
