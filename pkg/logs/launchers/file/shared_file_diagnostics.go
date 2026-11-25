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

// drainingTailerSuffix tells a tailer draining a rotated file from its replacement, which has the
// same ID.
const drainingTailerSuffix = " (rotated, draining)"

// sharedFileMessageKey keys the status message per tailer instance: tailers draining files
// rotated away from one path have the same ID.
func sharedFileMessageKey(t *tailer.Tailer) string {
	return fmt.Sprintf("sharedFile:%p", t)
}

// tailedFile is a tailer, named as on the status page, with the file it reads.
type tailedFile struct {
	tailer *tailer.Tailer
	id     string
	info   os.FileInfo
}

// reportSharedFiles puts a status message on the source of every tailer that reads the same file
// as another one, since that file's logs are then sent once per tailer. This happens with two
// paths to one file (symlink, hardlink), one path collected for several containers, and a tailer
// draining a rotated file next to its replacement (copytruncate) or next to a tailer on the
// rotated name.
func (s *Launcher) reportSharedFiles() {
	previous := s.sharedFileMessages
	s.sharedFileMessages = make(map[*tailer.Tailer]*config.Messages, len(previous))
	for _, group := range groupBySameFile(s.tailers.All(), s.rotatedTailers) {
		if len(group) > 1 {
			for _, member := range group {
				s.recordSharedFile(member, group, previous[member.tailer] == nil)
			}
		}
	}
	// Clear what no longer applies, including a message whose source was replaced.
	for t, messages := range previous {
		if s.sharedFileMessages[t] != messages {
			messages.RemoveMessage(sharedFileMessageKey(t))
		}
	}
}

// recordSharedFile sets the status message of member, which reads the same file as the rest of
// group, and logs it when it is new.
func (s *Launcher) recordSharedFile(member tailedFile, group []tailedFile, isNew bool) {
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
	message := fmt.Sprintf("%s reads the same file as %s, so logs from this file may be sent up to %d times. Check for overlapping log configurations, symlinks or hardlinks.",
		member.id, strings.Join(others, ", "), len(group))
	messages.AddMessage(sharedFileMessageKey(member.tailer), message)
	s.sharedFileMessages[member.tailer] = messages
	if isNew {
		log.Warn(message)
	}
}

// clearSharedFiles removes every shared-file status message, when the launcher stops.
func (s *Launcher) clearSharedFiles() {
	for t, messages := range s.sharedFileMessages {
		messages.RemoveMessage(sharedFileMessageKey(t))
	}
	s.sharedFileMessages = nil
}

// groupBySameFile groups tailers by the file they read. That is the file a tailer has open,
// which for a rotated tailer is no longer the one its path leads to. Windows tailers keep no file
// open, so active ones fall back to their path and rotated ones, which stop reading at rotation,
// are left out. Comparing with one member per group is quadratic at worst, but bounded by
// logs_config.open_files_limit and cheap per comparison.
func groupBySameFile(active, rotated []*tailer.Tailer) [][]tailedFile {
	files := make([]tailedFile, 0, len(active)+len(rotated))
	for _, t := range active {
		info, ok := t.OpenFileInfo()
		if !ok {
			var err error
			info, err = os.Stat(t.File().Path)
			ok = err == nil
		}
		if ok {
			files = append(files, tailedFile{tailer: t, id: t.GetID(), info: info})
		}
	}
	for _, t := range rotated {
		if info, ok := t.OpenFileInfo(); ok {
			files = append(files, tailedFile{tailer: t, id: t.GetID() + drainingTailerSuffix, info: info})
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
