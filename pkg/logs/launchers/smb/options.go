// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package smb

// Option configures NewLauncher.
type Option func(*options)

type options struct {
	openFilesLimit int
}

// WithOpenFilesLimit sets the limits on the files all the smb sources together
// hold open. The Agent passes logs_config.open_files_limit, the limit the file
// launcher applies to its own files; 0 or less means the default, 500. Each
// tailer has its own goroutines and decoder, so the limits keep a pattern that
// matches a very large number of files, or a writer that fills the share with
// files, from exhausting the Agent's memory. There are two budgets of that size,
// so the tailers and the drains together never exceed twice the limit, whatever
// the writer does.
//
// The tailer budget: at most that many active tailers. A new file, one that is
// not the next file of a path whose tailer was removed, starts only while a
// slot is free, the most recently modified first. A file that gets no slot
// waits, and the source status says how many do ("N files not tailed
// (open_files_limit reached)"); it starts, from start_position or its registry
// offset like any new tailer, once a slot frees. The limit never stops anything:
// a running tailer is never stopped for a newer file, so a file that stopped
// being written keeps its slot for as long as the pattern matches it, and enough
// of them keep every newer file waiting. Narrow the pattern to the files that are
// written, or raise the limit.
//
// When the tailer of a path is removed, by a rotation, a truncation or a
// replacement, the path's next tailer takes over its slot, so the active file
// never waits and the tailers never grow with the rotations. The path keeps the
// slot while its rotated file is drained, for a writer that creates the new file
// late, and for one more scan when nothing is being drained, then gives it back.
//
// The drain budget: at most that many drains of rotated files. A drain ends once
// its file had no new data for logs_config.close_timeout, and ten close timeouts
// at most, and frees its slot. A rotation, or a restart that finds a file rotated
// away, takes a drain slot when one is free; when none is, the file gets no
// drain, and the bytes of it the Agent had not read are reported missed (never
// lost silently): the listed size when known, else what the tailer knew was
// unread. The file keeps its place after those bytes, so a tailer of it that
// starts later does not read them again.
func WithOpenFilesLimit(n int) Option {
	return func(o *options) { o.openFilesLimit = n }
}
