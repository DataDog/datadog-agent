// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fileprovider

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	tailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// OpenFilesLimitWarningType is the key of the message generated when too many
// files are tailed
const openFilesLimitWarningType = "open_files_limit_warning"

// WildcardSelectionStrategy is used to specify if wildcard matches should be prioritized
// based on their filename or the modification time of each file
type WildcardSelectionStrategy int

const (
	// WildcardUseFileModTime means that the top 'filesLimit' most recently modified files
	// will be chosen from all wildcard matches
	WildcardUseFileModTime = iota
	// WildcardUseFileName means that wildcard matches will be chosen in a roughly reverse
	// lexicographical order
	WildcardUseFileName
)

// wildcardOrdering controls what ordering is applied to wildcard matches
type wildcardOrdering int

const (
	// wildcardReverseLexicographical is the default option and does a pseudo reverse alpha sort
	wildcardReverseLexicographical wildcardOrdering = iota
	// wildcardModTime sorts based on the most recently modified time for each matching wildcard file
	wildcardModTime
)

// selectionStrategy controls how the `filesLimit` slots we have are filled given a list of sources
type selectionStrategy int

const (
	// greedySelection will consider each source one-by-one, filling as many
	// slots as is possible from that source before proceeding to the next one
	greedySelection selectionStrategy = iota
	// globalSelection will consider files from all sources together and will choose the
	// top `filesLimit` files based on the `wildcardOrder` ordering
	globalSelection
)

// FileProvider implements the logic to retrieve at most filesLimit Files defined in sources
type FileProvider struct {
	filesLimit      int
	wildcardOrder   wildcardOrdering
	selectionMode   selectionStrategy
	shouldLogErrors bool
}

// NewFileProvider returns a new Provider
func NewFileProvider(filesLimit int, wildcardSelection WildcardSelectionStrategy) *FileProvider {
	wildcardOrder := wildcardReverseLexicographical
	selectionMode := greedySelection
	if wildcardSelection == WildcardUseFileModTime {
		wildcardOrder = wildcardModTime
		selectionMode = globalSelection
	}

	return &FileProvider{
		filesLimit:      filesLimit,
		wildcardOrder:   wildcardOrder,
		selectionMode:   selectionMode,
		shouldLogErrors: true,
	}
}

type matchCount struct {
	tracked int
	total   int
}

// wildcardFileCounter tracks how many files a wildcard source matches, and how many of those are actually tailed.
type wildcardFileCounter struct {
	counts map[*sources.LogSource]matchCount
}

func newWildcardFileCounter() wildcardFileCounter {
	return wildcardFileCounter{
		counts: map[*sources.LogSource]matchCount{},
	}
}

func (w *wildcardFileCounter) incrementTracked(src *sources.LogSource) {
	matchCnt, _ := w.counts[src]
	matchCnt.tracked++
	w.counts[src] = matchCnt
}

func (w *wildcardFileCounter) setTotal(src *sources.LogSource, total int) {
	matchCnt, _ := w.counts[src]
	matchCnt.total = total
	w.counts[src] = matchCnt
}

func (p *FileProvider) addFilesToTailList(inputFiles, filesToTail []*tailer.File, w *wildcardFileCounter) []*tailer.File {
	// Add each file one by one up to the limit
	for j := 0; j < len(inputFiles) && len(filesToTail) < p.filesLimit; j++ {
		file := inputFiles[j]
		filesToTail = append(filesToTail, file)
		src := file.Source.UnderlyingSource()
		if config.ContainsWildcard(src.Config.Path) {
			w.incrementTracked(src)
		}
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
	return filesToTail
}

// FilesToTail returns all the Files matching paths in sources,
// it cannot return more than filesLimit Files.
// Files are collected according to the fileProvider's wildcardOrder and selectionMode
func (p *FileProvider) FilesToTail(inputSources []*sources.LogSource) []*tailer.File {
	var filesToTail []*tailer.File
	shouldLogErrors := p.shouldLogErrors
	p.shouldLogErrors = false // Let's log errors on first run only
	wildcardFileCounter := newWildcardFileCounter()

	if p.selectionMode == globalSelection {
		wildcardSources := make([]*sources.LogSource, 0, len(inputSources))

		// First pass - collect all wildcard sources and add files for non-wildcard sources
		for i := 0; i < len(inputSources); i++ {
			source := inputSources[i]
			isWildcardSource := config.ContainsWildcard(source.Config.Path)
			if isWildcardSource {
				wildcardSources = append(wildcardSources, source)
				continue
			} else {
				files, err := p.CollectFiles(source)
				if err != nil {
					source.Status.Error(err)
					if shouldLogErrors {
						log.Warnf("Could not collect files: %v", err)
					}
					continue
				}
				filesToTail = p.addFilesToTailList(files, filesToTail, &wildcardFileCounter)
			}
		}

		// Second pass, resolve all wildcards and add them to one big list
		wildcardFiles := make([]*tailer.File, 0, p.filesLimit)
		for _, source := range wildcardSources {
			files, err := p.filesMatchingSource(source)
			wildcardFileCounter.setTotal(source, len(files))
			if err != nil {
				continue
			}
			wildcardFiles = append(wildcardFiles, files...)
		}

		p.applyOrdering(wildcardFiles)
		filesToTail = p.addFilesToTailList(wildcardFiles, filesToTail, &wildcardFileCounter)
	} else if p.selectionMode == greedySelection {
		// Consume all sources one-by-one, fitting as many as possible into 'filesToTail'
		for _, source := range inputSources {
			isWildcardSource := config.ContainsWildcard(source.Config.Path)
			files, err := p.CollectFiles(source)
			if isWildcardSource {
				wildcardFileCounter.setTotal(source, len(files))
			}
			if err != nil {
				source.Status.Error(err)
				if shouldLogErrors {
					log.Warnf("Could not collect files: %v", err)
				}
				continue
			}

			filesToTail = p.addFilesToTailList(files, filesToTail, &wildcardFileCounter)
		}
	} else {
		log.Errorf("Invalid file selection mode '%v', no files selected.", p.selectionMode)
	}

	// Record what ratio of files each wildcard source tracked
	for source, matchCnt := range wildcardFileCounter.counts {
		source.Messages.AddMessage(source.Config.Path, fmt.Sprintf("%d files tailed out of %d files matching", matchCnt.tracked, matchCnt.total))
	}

	if len(filesToTail) == p.filesLimit {
		log.Warn("Reached the limit on the maximum number of files in use: ", p.filesLimit)
	}

	return filesToTail
}

// CollectFiles takes a 'LogSource' and produces a list of tailers matching this source
// with ordering defined by 'wildcardOrder'
func (p *FileProvider) CollectFiles(source *sources.LogSource) ([]*tailer.File, error) {
	path := source.Config.Path
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return []*tailer.File{
			tailer.NewFile(path, source, false),
		}, nil
	case config.ContainsWildcard(path):
		files, err := p.filesMatchingSource(source)
		if err != nil {
			return nil, err
		}
		p.applyOrdering(files)

		return files, err
	default:
		return nil, fmt.Errorf("cannot read file %s: %s", path, err)
	}
}

// filesMatchingSource returns all the files matching the source path pattern.
func (p *FileProvider) filesMatchingSource(source *sources.LogSource) ([]*tailer.File, error) {
	pattern := source.Config.Path
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("malformed pattern, could not find any file: %s", pattern)
	}
	if len(paths) == 0 {
		// no file was found, its parent directories might have wrong permissions or it just does not exist
		return nil, fmt.Errorf("could not find any file matching pattern %s, check that all its subdirectories are executable", pattern)
	}

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

	files := make([]*tailer.File, 0, len(paths))
	for _, path := range paths {
		if excludedPaths[path] == 0 {
			files = append(files, tailer.NewFile(path, source, true))
		}
	}

	return files, nil
}

func applyModTimeOrdering(files []*tailer.File) {
	statResults := make(map[*tailer.File]time.Time, len(files))
	for _, file := range files {
		statRes, err := os.Stat(file.Path)
		if err != nil {
			// File has moved, de-prioritize this file to avoid selecting it
			// If it is selected anyway, Launcher#startNewTailer will fail and log a warning
			statResults[file] = time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)
		} else {
			statResults[file] = statRes.ModTime()
		}
	}
	// sort paths descending by mtime
	sort.SliceStable(files, func(i, j int) bool {
		return statResults[files[i]].After(statResults[files[j]])
	})
}

func applyReverseLexicographicalOrdering(files []*tailer.File) {
	// FIXME - this codepath assumes that the 'paths' will arrive in lexicographical order
	// This is true in the current go implementation, but it is unsafe to assume
	// https://cs.opensource.google/go/go/+/refs/tags/go1.19:src/path/filepath/match.go;l=330;drc=e4b624eae5fa3c51b8ca808da29442d3e3aaef04
	// https://github.com/golang/go/issues/17153
	//
	// Files are sorted because of a heuristic on the filename: often the filename and/or the folder name
	// contains information in the file datetime. Most of the time we want the most recent files.
	// Here, we reverse paths to have stable sort keep reverse lexicographical order w.r.t dir names. Example:
	// [/tmp/1/2017.log, /tmp/1/2018.log, /tmp/2/2018.log] becomes [/tmp/2/2018.log, /tmp/1/2018.log, /tmp/1/2017.log]
	// then kept as is by the sort below.

	// https://github.com/golang/go/wiki/SliceTricks#reversing
	for i := len(files)/2 - 1; i >= 0; i-- {
		opp := len(files) - 1 - i
		files[i], files[opp] = files[opp], files[i]
	}
	// sort paths by descending filenames
	sort.SliceStable(files, func(i, j int) bool {
		return filepath.Base(files[i].Path) > filepath.Base(files[j].Path)
	})
}

// applyOrdering sorts the 'files' slice in-place by the currently configured 'wildcardOrder'
func (p *FileProvider) applyOrdering(files []*tailer.File) {
	if p.wildcardOrder == wildcardModTime {
		applyModTimeOrdering(files)
	} else if p.wildcardOrder == wildcardReverseLexicographical {
		applyReverseLexicographicalOrdering(files)
	}
}
