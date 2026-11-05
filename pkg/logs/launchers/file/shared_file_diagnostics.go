// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// sharedFileMessagePrefix keys the status message per tailer, so a source with several files
// reports each of them.
const sharedFileMessagePrefix = "sharedFile:"

// tailedFile is an active tailer with the file its path currently resolves to.
type tailedFile struct {
	tailer *tailer.Tailer
	info   os.FileInfo
}

// reportSharedFiles tells, on the status page, about files that several active tailers read. That
// happens when configured paths lead to the same file: a symlink or a hardlink, a wildcard that
// overlaps an explicit path, or the same path collected for several containers. Each tailer
// sends the whole file, so its logs are sent more than once.
//
// Like the rotation check, this stats every tailed file once per scan. Paths are stat'ed rather
// than open handles because the Windows tailer does not keep one; os.Stat follows symlinks and
// os.SameFile matches hardlinks on every platform.
func (s *Launcher) reportSharedFiles() {
	groups := groupBySameFile(s.tailers.All())

	previous := s.sharedFileMessages
	s.sharedFileMessages = make(map[string]*config.Messages)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		for _, member := range group {
			s.recordSharedFile(member.tailer, group, previous)
		}
	}

	// Clear what no longer applies, including a message whose source was replaced.
	for scanKey, messages := range previous {
		if s.sharedFileMessages[scanKey] != messages {
			messages.RemoveMessage(sharedFileMessagePrefix + scanKey)
		}
	}
}

// recordSharedFile sets the status message of t, which reads the same file as the rest of group.
func (s *Launcher) recordSharedFile(t *tailer.Tailer, group []tailedFile, previous map[string]*config.Messages) {
	messages := fingerprintSkipMessages(t.File())
	if messages == nil {
		return
	}
	scanKey := t.GetID()
	others := make([]string, 0, len(group)-1)
	for _, other := range group {
		if other.tailer != t {
			others = append(others, other.tailer.GetID())
		}
	}
	slices.Sort(others)

	message := fmt.Sprintf("%s is the same file as %s, which is also being tailed. Each tailer sends the whole file, so its logs are sent %d times. Check for overlapping log configurations, symlinks or hardlinks.",
		t.File().Path, strings.Join(others, ", "), len(group))
	messages.AddMessage(sharedFileMessagePrefix+scanKey, message)
	s.sharedFileMessages[scanKey] = messages
	// Warn on the transition only: the status message carries the steady state.
	if previous[scanKey] == nil {
		log.Warn(message)
	}
}

// groupBySameFile groups tailers by the file their path currently resolves to. Tailers whose
// file cannot be stat'ed are left out: a missing file is the rotation check's business.
//
// Each file is compared with one member of every group found so far. That is quadratic in the
// worst case, but bounded by logs_config.open_files_limit, and os.SameFile only compares a few
// integers.
func groupBySameFile(tailers []*tailer.Tailer) [][]tailedFile {
	var groups [][]tailedFile
next:
	for _, t := range tailers {
		info, err := os.Stat(t.File().Path)
		if err != nil {
			continue
		}
		current := tailedFile{tailer: t, info: info}
		for i := range groups {
			if os.SameFile(groups[i][0].info, info) {
				groups[i] = append(groups[i], current)
				continue next
			}
		}
		groups = append(groups, []tailedFile{current})
	}
	return groups
}

// clearSharedFiles removes every shared-file status message, when the launcher stops.
func (s *Launcher) clearSharedFiles() {
	for scanKey, messages := range s.sharedFileMessages {
		messages.RemoveMessage(sharedFileMessagePrefix + scanKey)
	}
	s.sharedFileMessages = nil
}
