// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fileprovider provides file source provisioning for log launchers
package fileprovider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/opener"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/bmatcuk/doublestar/v4"
)

// OpenFilesLimitWarningType is the key of the message generated when too many
// files are tailed
const openFilesLimitWarningType = "open_files_limit_warning"

// rxContainerID is used in the shouldIgnore func to do a best-effort validation
// that the file currently scanned for a source is attached to the proper container.
// If the container ID we parse from the filename isn't matching this regexp, we *will*
// tail the file because we prefer a false-negative than a false-positive (best-effort).
var rxContainerID = regexp.MustCompile("^[a-fA-F0-9]{64}$")

// ContainersLogsDir is the directory in which we should find containers logsfile
// with the container ID in their filename.
// Public to be able to change it while running unit tests.
var ContainersLogsDir = "/var/log/containers"

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
	filesLimit          int
	wildcardOrder       wildcardOrdering
	selectionMode       selectionStrategy
	shouldLogErrors     bool
	reachedNumFileLimit bool
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
		filesLimit:          filesLimit,
		wildcardOrder:       wildcardOrder,
		selectionMode:       selectionMode,
		shouldLogErrors:     true,
		reachedNumFileLimit: false,
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
	matchCnt := w.counts[src]
	matchCnt.tracked++
	w.counts[src] = matchCnt
}

func (w *wildcardFileCounter) setTotal(src *sources.LogSource, total int) {
	matchCnt := w.counts[src]
	matchCnt.total = total
	w.counts[src] = matchCnt
}

func (p *FileProvider) addFilesToTailList(validatePodContainerID bool, inputFiles, filesToTail []*tailer.File, w *wildcardFileCounter, registry auditor.Registry) []*tailer.File {
	// Add each file one by one up to the limit
	for _, file := range inputFiles {
		// Unlike other tailers, there is a hard cap on the number of file tailers that can be concurrently active.
		// This means that we can't rely on the tailers themselves to keep the registry entries alive, and we need to
		// manually keep each valid file alive here.
		registry.KeepAlive(file.Identifier())

		if len(filesToTail) < p.filesLimit {
			if ShouldIgnore(validatePodContainerID, file) {
				continue
			}
			filesToTail = append(filesToTail, file)
			src := file.Source.UnderlyingSource()
			if config.ContainsWildcard(src.Config.Path) {
				w.incrementTracked(src)
			}
		}
	}

	if len(filesToTail) >= p.filesLimit {
		status.AddGlobalWarning(
			openFilesLimitWarningType,
			fmt.Sprintf(
				"The limit on the maximum number of files in use (%d) has been reached. If you aren't tailing the files you want to be tailing, increase this limit ("+
					"logs_config.open_files_limit in datadog.yaml), decrease the number of files you are tailing, or alter the logs_config.file_wildcard_selection_mode setting to by_modification_time.",
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
// Files are collected according to the fileProvider's wildcardOrder and selectionMode.
//
// currentlyTailed is the set of scan keys (see tailer.File.GetScanKey) of files
// that are currently being tailed by the caller. These files are exempt from the
// logs_config.ignore_older filter: ignore_older gates the creation of NEW tailers
// only, it must not stop tailers that are already running just because the file
// went quiet (that would be close_older semantics).
func (p *FileProvider) FilesToTail(ctx context.Context, validatePodContainerID bool, inputSources []*sources.LogSource, registry auditor.Registry, currentlyTailed map[string]bool) []*tailer.File {
	var filesToTail []*tailer.File
	shouldLogErrors := p.shouldLogErrors
	p.shouldLogErrors = false // Let's log errors on first run only
	wildcardFileCounter := newWildcardFileCounter()

	if p.selectionMode == globalSelection {
		wildcardSources := make([]*sources.LogSource, 0, len(inputSources))

		// First pass - collect all wildcard sources and add files for non-wildcard sources
		for _, inputSource := range inputSources {
			select {
			case <-ctx.Done():
				log.Debugf("FileProvider context cancelled, not collecting files.")
				return nil
			default:
				source := inputSource
				isWildcardSource := config.ContainsWildcard(source.Config.Path)
				if isWildcardSource {
					wildcardSources = append(wildcardSources, source)
					continue
				}
				files, err := p.collectFiles(source, currentlyTailed)
				if err != nil {
					source.Status.Error(err)
					if shouldLogErrors {
						log.Warnf("Could not collect files: %v", err)
					}
					continue
				}
				filesToTail = p.addFilesToTailList(validatePodContainerID, files, filesToTail, &wildcardFileCounter, registry)
			}
		}

		// Second pass, resolve all wildcards and add them to one big list
		wildcardFiles := make([]*tailer.File, 0, p.filesLimit)
		for _, source := range wildcardSources {
			select {
			case <-ctx.Done():
				log.Debugf("FileProvider context cancelled, not collecting files.")
				return nil
			default:
				files, err := p.filesMatchingSource(source, currentlyTailed)
				wildcardFileCounter.setTotal(source, len(files))
				if err != nil {
					continue
				}
				wildcardFiles = append(wildcardFiles, files...)
			}
		}

		p.applyOrdering(wildcardFiles)
		filesToTail = p.addFilesToTailList(validatePodContainerID, wildcardFiles, filesToTail, &wildcardFileCounter, registry)
	} else if p.selectionMode == greedySelection {
		// Consume all sources one-by-one, fitting as many as possible into 'filesToTail'
		for _, source := range inputSources {
			select {
			case <-ctx.Done():
				log.Debugf("FileProvider context cancelled, not collecting files.")
				return nil
			default:
				isWildcardSource := config.ContainsWildcard(source.Config.Path)
				files, err := p.collectFiles(source, currentlyTailed)
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
				filesToTail = p.addFilesToTailList(validatePodContainerID, files, filesToTail, &wildcardFileCounter, registry)
			}
		}
	} else {
		log.Errorf("Invalid file selection mode '%v', no files selected.", p.selectionMode)
	}

	// Record what ratio of files each wildcard source tracked
	for source, matchCnt := range wildcardFileCounter.counts {
		source.Messages.AddMessage(source.Config.Path, fmt.Sprintf("%d files tailed out of %d files matching", matchCnt.tracked, matchCnt.total))
	}

	if !p.reachedNumFileLimit && len(filesToTail) == p.filesLimit {
		log.Warn("Reached the limit on the maximum number of files in use: ", p.filesLimit)
		p.reachedNumFileLimit = true
	} else if len(filesToTail) < p.filesLimit {
		p.reachedNumFileLimit = false
	}

	return filesToTail
}

// CollectFiles takes a 'LogSource' and a set of currently-tailed scan keys,
// and produces a list of files matching this source with ordering defined by
// 'wildcardOrder'.
//
// currentlyTailed is the set of scan keys (see tailer.File.GetScanKey) whose
// files already have a running tailer. Those files bypass the
// logs_config.ignore_older filter so that:
//  1. An existing tailer is not stopped merely because its file went quiet
//     (that would be close_older semantics, which is out of scope for ignore_older).
//  2. When a source update arrives for a file that is already being tailed (e.g.
//     an Autodiscovery re-annotation with new tags), the file still appears in the
//     result list so the caller can call tailer.ReplaceSource — without this the
//     caller would see an empty list and silently leave the stale source in place.
//
// Pass nil if there are no currently-tailed files (the filter then applies
// unconditionally to all matched files).
func (p *FileProvider) CollectFiles(source *sources.LogSource, currentlyTailed map[string]bool) ([]*tailer.File, error) {
	return p.collectFiles(source, currentlyTailed)
}

// collectFiles is the internal implementation of CollectFiles. currentlyTailed
// is the set of scan keys whose files are already being tailed; those files
// bypass the ignore_older filter so we do not stop their tailers.
func (p *FileProvider) collectFiles(source *sources.LogSource, currentlyTailed map[string]bool) ([]*tailer.File, error) {
	path := source.Config.Path
	stat, err := opener.StatLogFile(path)
	switch {
	case err == nil:
		// Explicit single-path source: honor logs_config.ignore_older if set,
		// but only for files that are not currently being tailed. Stopping an
		// already-running tailer because its file went quiet would be
		// close_older semantics, which is explicitly out of scope.
		// We log at info level here so users see why a file they configured
		// explicitly is not being tailed.
		if ignoreOlder := getIgnoreOlder(); ignoreOlder > 0 && isFileOlderThan(stat.ModTime(), ignoreOlder) {
			if !isCurrentlyTailed(currentlyTailed, path, source) {
				log.Debugf("Skipping file %q: modification time (%s) is older than logs_config.ignore_older (%s)", path, stat.ModTime().Format(time.RFC3339), ignoreOlder)
				return nil, nil
			}
		}
		return []*tailer.File{
			tailer.NewFile(path, source, false),
		}, nil
	case config.ContainsWildcard(path):
		files, err := p.filesMatchingSource(source, currentlyTailed)
		if err != nil {
			return nil, err
		}
		p.applyOrdering(files)

		return files, err
	default:
		return nil, fmt.Errorf("cannot read file %s: %s", path, err)
	}
}

// scanKeyFor mirrors tailer.File.GetScanKey() for a given path/source pair so
// we can match against the launcher's set of currently-tailed scan keys
// without having to construct a *tailer.File first.
func scanKeyFor(path string, source *sources.LogSource) string {
	if source != nil && source.Config != nil && source.Config.Identifier != "" {
		return fmt.Sprintf("%s/%s", path, source.Config.Identifier)
	}
	return path
}

// isCurrentlyTailed reports whether the (path, source) pair currently has a
// running tailer according to the caller-provided set.
func isCurrentlyTailed(currentlyTailed map[string]bool, path string, source *sources.LogSource) bool {
	if len(currentlyTailed) == 0 {
		return false
	}
	return currentlyTailed[scanKeyFor(path, source)]
}

// getIgnoreOlder returns the configured logs_config.ignore_older duration.
// A return value of 0 means the filter is disabled.
func getIgnoreOlder() time.Duration {
	return pkgconfigsetup.Datadog().GetDuration("logs_config.ignore_older")
}

// isFileOlderThan reports whether the given modification time is older than
// `now - ignoreOlder`. The current time is read fresh on each call so the
// caller does not have to plumb a clock through.
func isFileOlderThan(modTime time.Time, ignoreOlder time.Duration) bool {
	return time.Since(modTime) > ignoreOlder
}

// filesMatchingSource returns all the files matching the source path pattern.
//
// The logs_config.ignore_older filter drops files whose mtime is too old, except
// for files whose scan key appears in currentlyTailed; those are preserved so a
// running tailer is not stopped just because the file went quiet.
func (p *FileProvider) filesMatchingSource(source *sources.LogSource, currentlyTailed map[string]bool) ([]*tailer.File, error) {
	pattern := source.Config.Path
	recursiveGlobEnabled := pkgconfigsetup.Datadog().GetBool("logs_config.enable_recursive_glob")

	var paths []string
	var err error
	if recursiveGlobEnabled && strings.Contains(pattern, "**") {
		paths, err = doublestar.FilepathGlob(pattern)
	} else {
		paths, err = filepath.Glob(pattern)
	}
	if err != nil {
		return nil, fmt.Errorf("malformed pattern, could not find any file: %s", pattern)
	}
	if len(paths) == 0 {
		// no file was found, its parent directories might have wrong permissions or it just does not exist
		return nil, fmt.Errorf("could not find any file matching pattern %s, check that all its subdirectories are executable", pattern)
	}

	excludedPaths := make(map[string]int)
	for _, excludePattern := range source.Config.ExcludePaths {
		var excludedGlob []string
		var err error
		if recursiveGlobEnabled && strings.Contains(excludePattern, "**") {
			excludedGlob, err = doublestar.FilepathGlob(excludePattern)
		} else {
			excludedGlob, err = filepath.Glob(excludePattern)
		}
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

	ignoreOlder := getIgnoreOlder()

	files := make([]*tailer.File, 0, len(paths))
	for _, path := range paths {
		if excludedPaths[path] != 0 {
			continue
		}
		if ignoreOlder > 0 {
			statRes, statErr := opener.StatLogFile(path)
			if statErr == nil && isFileOlderThan(statRes.ModTime(), ignoreOlder) {
				// ignore_older only gates the creation of new tailers; files
				// that are currently being tailed must stay in the list so
				// the launcher does not stop their tailers.
				if !isCurrentlyTailed(currentlyTailed, path, source) {
					log.Debugf("Skipping wildcard match %q: modification time (%s) is older than logs_config.ignore_older (%s)", path, statRes.ModTime().Format(time.RFC3339), ignoreOlder)
					continue
				}
			}
		}
		files = append(files, tailer.NewFile(path, source, true))
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

// ShouldIgnore resolves symlinks in /var/log/containers in order to use that redirection
// to validate that we will be reading a file for the correct container.
//
// We have to make sure that the file we just detected is tagged with the correct
// container ID if the source is a container source and `logs_config.validate_pod_container_id`
// is enabled`. The way k8s is storing files in /var/log/pods doesn't let us do that properly
// (the filename doesn't contain the container ID). However, the symlinks present in
// /var/log/containers are pointing to /var/log/pods files does, meaning that we can use them
// to validate that the file we have found is concerning us. That's what the function
// shouldIgnore is trying to do when the directory exists and is readable.
// See these links for more info:
//   - https://github.com/kubernetes/kubernetes/issues/58638
//   - https://github.com/fabric8io/fluent-plugin-kubernetes_metadata_filter/issues/105
func ShouldIgnore(validatePodContainerID bool, file *tailer.File) bool {
	// this method needs a source config to detect whether we should ignore that file or not
	if file == nil || file.Source == nil || file.Source.Config() == nil {
		return false
	}

	if !validatePodContainerID {
		return false
	}

	if file.Source.GetSourceType() != sources.KubernetesSourceType && file.Source.GetSourceType() != sources.DockerSourceType {
		return false
	}

	infos := make(map[string]string)
	err := filepath.WalkDir(ContainersLogsDir, func(containerLogFilename string, d os.DirEntry, err error) error {
		// we only wants to follow symlinks
		if d == nil || d.Type()&os.ModeSymlink != os.ModeSymlink || d.IsDir() {
			// not a symlink, we are not interested in this file
			return nil
		}

		// resolve the symlink
		podLogFilename, err2 := os.Readlink(containerLogFilename)
		if err2 != nil {
			log.Debug("Error while resolving symlink of", containerLogFilename, ":", err)
			return nil
		}

		infos[podLogFilename] = containerLogFilename
		return nil
	})

	// this is not an error if we are not currently looking for container logs files,
	// so not problem and just return false.
	// Still, we write a debug message to be able to troubleshoot that
	// in cases we're legitimately looking for containers logs.
	if err != nil {
		log.Debug("Can't look for symlinks in /var/log/containers:", err)
		return false
	}

	// container id extracted from the file found in /var/log/containers
	base := filepath.Base(infos[file.Path]) // only the file
	ext := filepath.Ext(base)               // file extension
	parts := strings.Split(base, "-")       // get only the container ID part from the file
	var containerIDFromFilename string
	if len(parts) > 1 {
		containerIDFromFilename = strings.TrimSuffix(parts[len(parts)-1], ext)
	}

	// basic validation of the ID that has been parsed, if it doesn't look like
	// an ID we don't want to compare another ID to it
	if containerIDFromFilename == "" || !rxContainerID.Match([]byte(containerIDFromFilename)) {
		return false
	}

	if file.Source.Config().Identifier != "" && containerIDFromFilename != "" {
		if strings.TrimSpace(strings.ToLower(containerIDFromFilename)) != strings.TrimSpace(strings.ToLower(file.Source.Config().Identifier)) {
			log.Debugf("We were about to tail a file attached to the wrong container (%s != %s), probably due to short-lived containers.",
				containerIDFromFilename, file.Source.Config().Identifier)
			// ignore this file, it is not concerning the container stored in file.Source
			return true
		}
	}

	return false
}
