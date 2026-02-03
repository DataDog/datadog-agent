// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// testLogView is a LogView implementation for loaded log files.
type testLogView struct {
	content   []byte
	status    string
	tags      []string
	hostname  string
	timestamp int64
}

func (v *testLogView) GetContent() []byte  { return v.content }
func (v *testLogView) GetStatus() string   { return v.status }
func (v *testLogView) GetTags() []string   { return v.tags }
func (v *testLogView) GetHostname() string { return v.hostname }
func (v *testLogView) GetTimestamp() int64 { return v.timestamp }

// LoadLogFile loads logs from a file and returns LogView instances.
// Supports JSON lines format and plain text with timestamps.
func LoadLogFile(path string) ([]observerdef.LogView, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var logs []observerdef.LogView
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

		log, err := parseLine(line)
		if err != nil {
			// Skip unparseable lines with a warning
			continue
		}
		logs = append(logs, log)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return logs, nil
}

// parseLine attempts to parse a single line as JSON or plain text.
func parseLine(line string) (observerdef.LogView, error) {
	// Try JSON first
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		return parseJSONLine(line)
	}

	// Fall back to plain text
	return parsePlainTextLine(line)
}

// parseJSONLine parses a JSON log line.
func parseJSONLine(line string) (observerdef.LogView, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return nil, err
	}

	log := &testLogView{
		content: []byte(line),
		status:  "info",
	}

	// Extract timestamp from common fields
	for _, field := range []string{"timestamp", "ts", "time", "@timestamp", "date"} {
		if ts, ok := data[field]; ok {
			if t := parseTimestamp(ts); t > 0 {
				log.timestamp = t
				break
			}
		}
	}

	// Extract level/status from common fields
	for _, field := range []string{"level", "severity", "status", "loglevel"} {
		if level, ok := data[field].(string); ok {
			log.status = strings.ToLower(level)
			break
		}
	}

	// Extract hostname
	for _, field := range []string{"hostname", "host", "node"} {
		if hostname, ok := data[field].(string); ok {
			log.hostname = hostname
			break
		}
	}

	// Build tags from string fields
	var tags []string
	for _, field := range []string{"service", "source", "component", "app", "application"} {
		if val, ok := data[field].(string); ok {
			tags = append(tags, field+":"+val)
		}
	}
	log.tags = tags

	// Default timestamp to 0 if not found (will be set to current time during processing)
	return log, nil
}

// parsePlainTextLine parses a plain text log line with timestamp extraction.
func parsePlainTextLine(line string) (observerdef.LogView, error) {
	log := &testLogView{
		content: []byte(line),
		status:  "info",
	}

	// Try to extract timestamp from the beginning of the line
	// Common formats:
	// 2024-01-15T10:30:00Z ...
	// 2024-01-15 10:30:00 ...
	// Jan 15 10:30:00 ...
	// 1705315800 ...

	patterns := []struct {
		regex  *regexp.Regexp
		layout string
	}{
		{regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?)`), time.RFC3339},
		{regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`), "2006-01-02 15:04:05"},
		{regexp.MustCompile(`^([A-Z][a-z]{2} \d{1,2} \d{2}:\d{2}:\d{2})`), "Jan 2 15:04:05"},
		{regexp.MustCompile(`^(\d{10,13})\b`), "unix"},
	}

	for _, p := range patterns {
		if matches := p.regex.FindStringSubmatch(line); len(matches) > 1 {
			if p.layout == "unix" {
				if ts, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
					// Handle milliseconds
					if ts > 1e12 {
						ts = ts / 1000
					}
					log.timestamp = ts
					break
				}
			} else {
				if t, err := time.Parse(p.layout, matches[1]); err == nil {
					log.timestamp = t.Unix()
					break
				}
			}
		}
	}

	// Try to extract log level
	levelPatterns := []struct {
		regex *regexp.Regexp
		level string
	}{
		{regexp.MustCompile(`\b(ERROR|ERR)\b`), "error"},
		{regexp.MustCompile(`\b(WARN|WARNING)\b`), "warn"},
		{regexp.MustCompile(`\b(INFO)\b`), "info"},
		{regexp.MustCompile(`\b(DEBUG|DBG)\b`), "debug"},
		{regexp.MustCompile(`\b(TRACE)\b`), "trace"},
	}

	for _, p := range levelPatterns {
		if p.regex.MatchString(line) {
			log.status = p.level
			break
		}
	}

	return log, nil
}

// parseTimestamp attempts to parse various timestamp formats.
func parseTimestamp(v interface{}) int64 {
	switch t := v.(type) {
	case float64:
		// Unix timestamp (might be in seconds or milliseconds)
		if t > 1e12 {
			return int64(t / 1000)
		}
		return int64(t)
	case int64:
		if t > 1e12 {
			return t / 1000
		}
		return t
	case string:
		// Try various formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05.000Z",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, format := range formats {
			if parsed, err := time.Parse(format, t); err == nil {
				return parsed.Unix()
			}
		}
		// Try unix timestamp as string
		if ts, err := strconv.ParseInt(t, 10, 64); err == nil {
			if ts > 1e12 {
				return ts / 1000
			}
			return ts
		}
	}
	return 0
}

// LoadEventFile loads events from a JSON file.
// Expects a JSON array of Signal objects.
func LoadEventFile(path string) ([]observerdef.Signal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try to parse as array first
	var events []observerdef.Signal
	if err := json.Unmarshal(data, &events); err == nil {
		return events, nil
	}

	// Try to parse as JSON lines
	var result []observerdef.Signal
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "[" || line == "]" {
			continue
		}

		// Remove trailing comma if present
		line = strings.TrimSuffix(line, ",")

		var event observerdef.Signal
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		result = append(result, event)
	}

	if len(result) > 0 {
		return result, nil
	}

	return nil, fmt.Errorf("could not parse event file")
}
