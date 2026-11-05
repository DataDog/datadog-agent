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

// tailedFile is a tailer with the file it reads.
type tailedFile struct {
	tailer *tailer.Tailer
	// id is the tailer's ID on the status page, which tells a draining tailer from its replacement.
	id   string
	info os.FileInfo
}

// reportSharedFiles tells, on the status page, about files that several tailers read, which
// sends their logs more than once. That happens when:
//   - configured paths lead to the same file: a symlink or a hardlink, possibly matched by a
//     wildcard that overlaps an explicit path;
//   - the same path is collected for several containers, whose tailers have distinct scan keys;
//   - a tailer draining a rotated file reads the same file as another one: its replacement after
//     a copytruncate, or a tailer that another configuration started on the rotated name.
//
// Like the rotation check, this stats every tailed file once per scan.
func (s *Launcher) reportSharedFiles() {
	groups := groupBySameFile(s.tailers.All(), s.rotatedTailers)

	previous := s.sharedFileMessages
	s.sharedFileMessages = make(map[string]*config.Messages)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		for _, member := range group {
			s.recordSharedFile(member, group, previous)
		}
	}

	// Clear what no longer applies, including a message whose source was replaced.
	for scanKey, messages := range previous {
		if s.sharedFileMessages[scanKey] != messages {
			messages.RemoveMessage(sharedFileMessagePrefix + scanKey)
		}
	}
}

// recordSharedFile sets the status message of member, which reads the same file as the rest of
// group.
func (s *Launcher) recordSharedFile(member tailedFile, group []tailedFile, previous map[string]*config.Messages) {
	messages := fingerprintSkipMessages(member.tailer.File())
	if messages == nil {
		return
	}
	others := make([]string, 0, len(group)-1)
	for _, other := range group {
		if other.tailer != member.tailer {
			others = append(others, other.id)
		}
	}
	slices.Sort(others)

	message := fmt.Sprintf("%s reads the same file as %s. Each tailer sends what it reads, so logs from this file may be sent up to %d times. Check for overlapping log configurations, symlinks or hardlinks.",
		member.id, strings.Join(others, ", "), len(group))
	messages.AddMessage(sharedFileMessagePrefix+member.id, message)
	s.sharedFileMessages[member.id] = messages
	// Warn on the transition only: the status message carries the steady state.
	if previous[member.id] == nil {
		log.Warn(message)
	}
}

// groupBySameFile groups the active and the rotated tailers by the file they read.
//
// Each file is compared with one member of every group found so far. That is quadratic in the
// worst case, but bounded by logs_config.open_files_limit, and os.SameFile only compares a few
// integers.
func groupBySameFile(active, rotated []*tailer.Tailer) [][]tailedFile {
	files := make([]tailedFile, 0, len(active)+len(rotated))
	for _, t := range active {
		if info, ok := activeFileInfo(t); ok {
			files = append(files, tailedFile{tailer: t, id: t.GetID(), info: info})
		}
	}
	for _, t := range rotated {
		// A rotated tailer reads the file it has open, not the one its path now leads to. Without
		// an open file it reads nothing more: it is finished, or it runs on Windows.
		if info, ok := t.OpenFileInfo(); ok {
			files = append(files, tailedFile{tailer: t, id: rotatedTailerStatus{t}.GetID(), info: info})
		}
	}

	var groups [][]tailedFile
next:
	for _, file := range files {
		for i := range groups {
			if os.SameFile(groups[i][0].info, file.info) {
				groups[i] = append(groups[i], file)
				continue next
			}
		}
		groups = append(groups, []tailedFile{file})
	}
	return groups
}

// activeFileInfo describes the file an active tailer reads: the one it has open, or on Windows,
// where tailers keep no file open, the one its path resolves to. os.Stat follows symlinks and
// os.SameFile matches hardlinks on every platform. A file that cannot be stat'ed is left out:
// a missing file is the rotation check's business.
func activeFileInfo(t *tailer.Tailer) (os.FileInfo, bool) {
	if info, ok := t.OpenFileInfo(); ok {
		return info, true
	}
	info, err := os.Stat(t.File().Path)
	return info, err == nil
}

// clearSharedFiles removes every shared-file status message, when the launcher stops.
func (s *Launcher) clearSharedFiles() {
	for scanKey, messages := range s.sharedFileMessages {
		messages.RemoveMessage(sharedFileMessagePrefix + scanKey)
	}
	s.sharedFileMessages = nil
}
