// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

// LogReader reads JSON lines log files and provides logs in chronological order.
type LogReader struct {
	logs  []recorderdef.LogData
	index int
}

// NewLogReader creates a new log reader from a directory containing JSON lines log files.
func NewLogReader(dirPath string) (*LogReader, error) {
	// Find all jsonl files in the directory
	logFiles, err := findLogFiles(dirPath)
	if err != nil {
		return nil, fmt.Errorf("finding log files: %w", err)
	}

	if len(logFiles) == 0 {
		return nil, fmt.Errorf("no log files found in %s", dirPath)
	}

	// Read all logs from all files
	var allLogs []recorderdef.LogData
	for _, filePath := range logFiles {
		logs, err := readLogFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, err)
		}
		allLogs = append(allLogs, logs...)
	}

	// Sort by timestamp
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Timestamp < allLogs[j].Timestamp
	})

	return &LogReader{
		logs:  allLogs,
		index: 0,
	}, nil
}

// Next returns the next log entry, or nil if no more logs.
func (r *LogReader) Next() *recorderdef.LogData {
	if r.index >= len(r.logs) {
		return nil
	}
	log := &r.logs[r.index]
	r.index++
	return log
}

// Reset resets the reader to the beginning.
func (r *LogReader) Reset() {
	r.index = 0
}

// Len returns the total number of logs.
func (r *LogReader) Len() int {
	return len(r.logs)
}

// All returns all logs.
func (r *LogReader) All() []recorderdef.LogData {
	return r.logs
}

// StartTime returns the timestamp of the first log in seconds.
func (r *LogReader) StartTime() int64 {
	if len(r.logs) == 0 {
		return 0
	}
	return r.logs[0].Timestamp
}

// EndTime returns the timestamp of the last log in seconds.
func (r *LogReader) EndTime() int64 {
	if len(r.logs) == 0 {
		return 0
	}
	return r.logs[len(r.logs)-1].Timestamp
}

// findLogFiles finds all .jsonl files in a directory (non-recursive).
func findLogFiles(dirPath string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".json") {
			files = append(files, filepath.Join(dirPath, name))
		}
	}

	sort.Strings(files) // Sort for consistent ordering
	return files, nil
}

// readLogFile reads a single JSON lines file and extracts log entries.
func readLogFile(filePath string) ([]recorderdef.LogData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	var logs []recorderdef.LogData
	scanner := bufio.NewScanner(file)

	// Increase scanner buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Parse JSON line
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Skip unparseable lines
			continue
		}

		logs = append(logs, recorderdef.LogData{
			Source:    entry.Source,
			Content:   entry.Content,
			Status:    entry.Status,
			Hostname:  entry.Hostname,
			Timestamp: entry.Timestamp,
			Tags:      entry.Tags,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return logs, nil
}
