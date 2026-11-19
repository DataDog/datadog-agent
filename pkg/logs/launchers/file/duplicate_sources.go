// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// A file has a single tailer, so when several sources match it (for instance two configs in
// the same container's logs annotation) only one of them is applied. The owner is the most
// recently added active source: that is the source launchTailers hands an already-tailed file
// to, so scans, rotations and source removals have to agree with it. Without this, a scan kept
// whichever source it listed first, and the config in effect flipped after every rotation.

const duplicateSourceMessagePrefix = "duplicateSource:"

// resolveDuplicateSources returns files with a single entry per scan key, the one of the source
// that owns the file, points the existing tailers at their file's owner, and reports the sources
// that are left out.
func (s *Launcher) resolveDuplicateSources(files []*tailer.File) []*tailer.File {
	rank := make(map[*sources.LogSource]int, len(s.activeSources))
	for i, source := range s.activeSources {
		rank[source] = i
	}
	// Sources that are no longer active rank below every active one.
	rankOf := func(source *sources.LogSource) int {
		if r, ok := rank[source]; ok {
			return r
		}
		return -1
	}

	ownerIndex := make(map[string]int, len(files))
	matchingSources := make(map[string][]*sources.LogSource)
	deduped := make([]*tailer.File, 0, len(files))
	for _, file := range files {
		scanKey := file.GetScanKey()
		candidate := file.Source.UnderlyingSource()
		i, seen := ownerIndex[scanKey]
		if !seen {
			ownerIndex[scanKey] = len(deduped)
			deduped = append(deduped, file)
			matchingSources[scanKey] = []*sources.LogSource{candidate}
			continue
		}
		if !slices.Contains(matchingSources[scanKey], candidate) {
			matchingSources[scanKey] = append(matchingSources[scanKey], candidate)
		}
		if rankOf(candidate) > rankOf(deduped[i].Source.UnderlyingSource()) {
			deduped[i] = file
		}
	}

	for scanKey, i := range ownerIndex {
		t, isTailed := s.tailers.Get(scanKey)
		if !isTailed {
			continue
		}
		owner := deduped[i].Source.UnderlyingSource()
		tailed := t.Source()
		switch {
		case tailed == owner:
		case rankOf(tailed) > rankOf(owner):
			// The tailer was handed a source added after this scan took its copy of
			// activeSources. Keep it, including for a replacement tailer after a rotation.
			deduped[i] = tailer.NewFile(deduped[i].Path, tailed, deduped[i].IsWildcardPath)
			if !slices.Contains(matchingSources[scanKey], tailed) {
				matchingSources[scanKey] = append(matchingSources[scanKey], tailed)
			}
		case rankOf(owner) >= 0:
			// The tailer's source was removed, or is older than a source it was never handed:
			// move the tailer to the owner, the same way launchTailers does.
			owner.SetStatus(tailed.Status())
			t.ReplaceSource(owner)
		}
	}

	s.reportDuplicateSources(deduped, ownerIndex, matchingSources, rankOf)
	return deduped
}

// reportDuplicateSources warns once in the agent log about each source that matches a file owned
// by another source, and keeps a message on that source's status for as long as it is left out.
func (s *Launcher) reportDuplicateSources(files []*tailer.File, ownerIndex map[string]int, matchingSources map[string][]*sources.LogSource, rankOf func(*sources.LogSource) int) {
	reported := make(map[string]map[*sources.LogSource]string)
	for scanKey, matching := range matchingSources {
		if len(matching) < 2 {
			continue
		}
		file := files[ownerIndex[scanKey]]
		owner := file.Source.UnderlyingSource()
		for _, source := range matching {
			if source == owner || rankOf(source) < 0 {
				continue
			}
			message := fmt.Sprintf("%s is also matched by another log config (%s), which is the one applied to it. Only one log config is applied per file, so this config is ignored for this file.",
				file.Path, describeSource(owner))
			if reported[scanKey] == nil {
				reported[scanKey] = make(map[*sources.LogSource]string)
			}
			reported[scanKey][source] = message
			if s.duplicateSources[scanKey][source] != message {
				log.Warnf("Log file %s is matched by several log configs. Only one is applied: %s. Ignored: %s.", file.Path, describeSource(owner), describeSource(source))
				source.Messages.AddMessage(duplicateSourceMessagePrefix+scanKey, message)
			}
		}
	}

	for scanKey, previous := range s.duplicateSources {
		for source := range previous {
			if _, stillReported := reported[scanKey][source]; !stillReported {
				source.Messages.RemoveMessage(duplicateSourceMessagePrefix + scanKey)
			}
		}
	}
	s.duplicateSources = reported
}

// clearDuplicateSources removes every duplicate source message.
func (s *Launcher) clearDuplicateSources() {
	for scanKey, reported := range s.duplicateSources {
		for source := range reported {
			source.Messages.RemoveMessage(duplicateSourceMessagePrefix + scanKey)
		}
	}
	s.duplicateSources = nil
}

// describeSource names a source by the fields users set in a log config. Several sources of the
// same container share their name, so it is not enough to tell them apart. The processing rules
// are included because they can be the only difference between two configs, and they are what
// decides whether a file's logs are kept.
func describeSource(source *sources.LogSource) string {
	var parts []string
	if source.Config.Source != "" {
		parts = append(parts, "source: "+source.Config.Source)
	}
	if source.Config.Service != "" {
		parts = append(parts, "service: "+source.Config.Service)
	}
	if len(source.Config.Tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(source.Config.Tags, ","))
	}
	if len(source.Config.ProcessingRules) > 0 {
		rules := make([]string, 0, len(source.Config.ProcessingRules))
		for _, rule := range source.Config.ProcessingRules {
			if rule.Name != "" {
				rules = append(rules, rule.Type+" "+rule.Name)
			} else {
				rules = append(rules, rule.Type)
			}
		}
		parts = append(parts, "log_processing_rules: "+strings.Join(rules, ","))
	}
	if len(parts) == 0 {
		return source.Name
	}
	return strings.Join(parts, ", ")
}
