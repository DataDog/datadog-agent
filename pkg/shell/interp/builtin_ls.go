// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (r *Runner) builtinLs(args []string) error {
	longFormat := false
	showHidden := false
	humanReadable := false
	sortByTime := false
	recursive := false
	reverseSort := false
	onePerLine := false
	dirOnly := false
	var paths []string
	endOfFlags := false

	for _, a := range args {
		if endOfFlags || !strings.HasPrefix(a, "-") || a == "-" {
			paths = append(paths, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		for _, c := range a[1:] {
			switch c {
			case 'l':
				longFormat = true
			case 'a':
				showHidden = true
			case 'h':
				humanReadable = true
			case 't':
				sortByTime = true
			case 'R':
				recursive = true
			case 'r':
				reverseSort = true
			case '1':
				onePerLine = true
			case 'd':
				dirOnly = true
			default:
				return fmt.Errorf("flag \"-%c\" is not allowed for command \"ls\"", c)
			}
		}
	}

	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Resolve paths relative to runner's directory.
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(r.dir, p)
	}

	hasError := false
	multiPath := len(paths) > 1

	for idx, p := range paths {
		absPath := resolve(p)

		info, err := os.Lstat(absPath)
		if err != nil {
			fmt.Fprintf(r.stderr, "ls: cannot access '%s': No such file or directory\n", p)
			hasError = true
			continue
		}

		if dirOnly {
			r.lsPrintEntry(info, p, longFormat, humanReadable)
			continue
		}

		if !info.IsDir() {
			r.lsPrintEntry(info, p, longFormat, humanReadable)
			continue
		}

		if recursive {
			r.lsRecursive(absPath, p, showHidden, longFormat, humanReadable, sortByTime, reverseSort, onePerLine, true)
		} else {
			if multiPath {
				if idx > 0 {
					fmt.Fprintln(r.stdout)
				}
				fmt.Fprintf(r.stdout, "%s:\n", p)
			}
			r.lsDir(absPath, showHidden, longFormat, humanReadable, sortByTime, reverseSort, onePerLine)
		}
	}

	if hasError {
		r.exitCode = 1
	} else {
		r.exitCode = 0
	}
	return nil
}

func (r *Runner) lsDir(dir string, showHidden, longFormat, humanReadable, sortByTime, reverseSort, onePerLine bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(r.stderr, "ls: cannot open directory '%s': %v\n", dir, err)
		r.exitCode = 1
		return
	}

	// Filter hidden files.
	if !showHidden {
		filtered := entries[:0]
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Sort.
	if sortByTime {
		sort.SliceStable(entries, func(i, j int) bool {
			ti := modTime(entries[i])
			tj := modTime(entries[j])
			if reverseSort {
				return ti.Before(tj)
			}
			return ti.After(tj)
		})
	} else if reverseSort {
		// Reverse alphabetical.
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Name() > entries[j].Name()
		})
	}

	if longFormat {
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			r.lsPrintLong(info, e.Name(), humanReadable)
		}
	} else if onePerLine {
		for _, e := range entries {
			fmt.Fprintln(r.stdout, e.Name())
		}
	} else {
		// Simple space-separated output, one per line for simplicity.
		for _, e := range entries {
			fmt.Fprintln(r.stdout, e.Name())
		}
	}
}

func (r *Runner) lsRecursive(absPath, displayPath string, showHidden, longFormat, humanReadable, sortByTime, reverseSort, onePerLine, isFirst bool) {
	if !isFirst {
		fmt.Fprintln(r.stdout)
	}
	fmt.Fprintf(r.stdout, "%s:\n", displayPath)
	r.lsDir(absPath, showHidden, longFormat, humanReadable, sortByTime, reverseSort, onePerLine)

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			subAbs := filepath.Join(absPath, e.Name())
			subDisplay := filepath.Join(displayPath, e.Name())
			r.lsRecursive(subAbs, subDisplay, showHidden, longFormat, humanReadable, sortByTime, reverseSort, onePerLine, false)
		}
	}
}

func (r *Runner) lsPrintEntry(info fs.FileInfo, name string, longFormat, humanReadable bool) {
	if longFormat {
		r.lsPrintLong(info, name, humanReadable)
	} else {
		fmt.Fprintln(r.stdout, name)
	}
}

func (r *Runner) lsPrintLong(info fs.FileInfo, name string, humanReadable bool) {
	mode := info.Mode().String()
	size := info.Size()
	modTime := info.ModTime()

	var sizeStr string
	if humanReadable {
		sizeStr = formatHumanSize(size)
	} else {
		sizeStr = fmt.Sprintf("%d", size)
	}

	timeStr := formatLsTime(modTime)
	fmt.Fprintf(r.stdout, "%s %8s %s %s\n", mode, sizeStr, timeStr, name)
}

func formatHumanSize(bytes int64) string {
	const (
		_  = iota
		kB = 1 << (10 * iota)
		mB
		gB
		tB
	)
	switch {
	case bytes >= tB:
		return fmt.Sprintf("%.1fT", float64(bytes)/float64(tB))
	case bytes >= gB:
		return fmt.Sprintf("%.1fG", float64(bytes)/float64(gB))
	case bytes >= mB:
		return fmt.Sprintf("%.1fM", float64(bytes)/float64(mB))
	case bytes >= kB:
		return fmt.Sprintf("%.1fK", float64(bytes)/float64(kB))
	default:
		return fmt.Sprintf("%d", bytes)
	}
}

func formatLsTime(t time.Time) string {
	now := time.Now()
	sixMonthsAgo := now.AddDate(0, -6, 0)
	if t.Before(sixMonthsAgo) || t.After(now) {
		return t.Format("Jan _2  2006")
	}
	return t.Format("Jan _2 15:04")
}

func modTime(e os.DirEntry) time.Time {
	info, err := e.Info()
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
