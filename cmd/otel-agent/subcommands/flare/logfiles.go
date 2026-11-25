// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// The size that the flare keeps of each log file of the collector, which is the end of the file, and of all of them,
// the newest files first, so that large logs or many rotations can't exhaust the memory of the flare. Variables for
// the tests.
var (
	maxLogFileBytes  int64 = 10 << 20
	maxLogTotalBytes int64 = 50 << 20
)

// collectorLogFile returns the log file of the collector, resolved like `otel-agent run` does.
// It is empty when file logging is disabled.
func collectorLogFile(cfg pkgconfigmodel.Reader) string {
	return log.ForDaemon("", "log_file", defaultpaths.GetDefaultOTelAgentLogFile()).LogFileFn(cfg)
}

// collectLogFiles returns the log file of the collector and the files its rotation keeps next to it,
// under logs/ like the core Agent flare does. The logs of the other components that share the
// directory are not the collector's, so they are left out.
func collectLogFiles(cfg pkgconfigmodel.Reader) []flareFile {
	logFile := collectorLogFile(cfg)
	if logFile == "" {
		warnf("file logging is disabled (disable_file_logging), the flare won't contain the log of the collector.")
		return nil
	}

	dir, name := filepath.Dir(logFile), filepath.Base(logFile)
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		warnf("could not read the log directory of the collector: %v", err)
		return nil
	}

	// The log file first, then its rotations from the newest, so that the size limit leaves out the oldest
	var names []string
	for _, entry := range entries {
		if isLogFileOrRoll(entry.Name(), name) {
			names = append(names, entry.Name())
		}
	}
	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(rollNumber(a, name), rollNumber(b, name)) })

	remaining := maxLogTotalBytes
	var files []flareFile
	for _, fileName := range names {
		path := filepath.Join(dir, fileName)
		// Not a pipe or a device, such as a log file linked to /dev/stdout: reading it would block
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		if remaining <= 0 {
			warnf("the flare keeps at most %d bytes of the logs of the collector, so %s and the older log files are left out.", maxLogTotalBytes, fileName)
			break
		}
		fmt.Fprintf(color.Output, "Collecting %s...\n", path)
		content, read, err := readLogTail(path, min(maxLogFileBytes, remaining))
		if err != nil {
			warnf("could not read the log file %s: %v", path, err)
			continue
		}
		remaining -= read
		files = append(files, flareFile{name: "logs/" + fileName, content: content, scrub: true})
	}
	if len(files) == 0 {
		warnf("no log file of the collector found at %s, the flare won't contain its log. If the collector writes it elsewhere, set log_file (DD_LOG_FILE) to that file.", logFile)
	}
	return files
}

// rollNumber returns the number of a file of the rotation (otel-agent.log.2 is 2), 0 for the log file itself
func rollNumber(fileName, logFileName string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(fileName, logFileName+"."))
	if fileName == logFileName || err != nil {
		return 0
	}
	return n
}

// readLogTail returns the end of a log file, at most limit bytes of it, from the start of a line. When the beginning
// of the file is left out, a first line says so. read is the number of bytes read from the file.
func readLogTail(path string, limit int64) (content []byte, read int64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := info.Size()
	if size > limit {
		if _, err := file.Seek(size-limit, io.SeekStart); err != nil {
			return nil, 0, err
		}
	}
	tail, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, 0, err
	}
	read = int64(len(tail))
	if size <= limit {
		return tail, read, nil
	}
	// The cut is in the middle of a line: start at the next one
	if i := bytes.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	header := fmt.Sprintf("[the flare has the last %d bytes of this file of %d bytes, the beginning is left out]\n", len(tail), size)
	return append([]byte(header), tail...), read, nil
}

// isLogFileOrRoll returns whether the file is the log file or one of the files its rotation keeps,
// which are named after it with a number (otel-agent.log.1).
func isLogFileOrRoll(fileName, logFileName string) bool {
	if fileName == logFileName {
		return true
	}
	roll, ok := strings.CutPrefix(fileName, logFileName+".")
	return ok && roll != "" && strings.Trim(roll, "0123456789") == ""
}

// warnings are the warnings of this run, which the flare prints as they happen and keeps in flare-info.txt
var warnings []string

// warnf prints a warning on the console, and keeps it for the archive
func warnf(format string, args ...any) {
	warning := fmt.Sprintf(format, args...)
	warnings = append(warnings, warning)
	fmt.Fprintln(color.Output, color.YellowString("Warning: %s", warning))
}
