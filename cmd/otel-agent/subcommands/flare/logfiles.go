// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
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

	var files []flareFile
	for _, entry := range entries {
		if !isLogFileOrRoll(entry.Name(), name) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		// Not a pipe or a device, such as a log file linked to /dev/stdout: reading it would block
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		fmt.Fprintf(color.Output, "Collecting %s...\n", path)
		content, err := os.ReadFile(path)
		if err != nil {
			warnf("could not read the log file %s: %v", path, err)
			continue
		}
		files = append(files, flareFile{name: "logs/" + entry.Name(), content: content, scrub: true})
	}
	if len(files) == 0 {
		warnf("no log file of the collector found at %s, the flare won't contain its log. If the collector writes it elsewhere, set log_file (DD_LOG_FILE) to that file.", logFile)
	}
	return files
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

// warnf prints a warning on the console
func warnf(format string, args ...any) {
	fmt.Fprintln(color.Output, color.YellowString("Warning: "+format, args...))
}
