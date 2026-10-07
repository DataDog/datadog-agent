// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// go-yaml reports a line number but no column, so quote whole lines.
var yamlErrorLineRe = regexp.MustCompile(`line (\d+)`)

// yamlErrorContextRadius is how many lines to show either side of the error.
const yamlErrorContextRadius = 2

// recordConfigFileError logs a config file problem and retains it so that status,
// the GUI and flare can surface it. loadCustom returns before logging warnings when
// the file is unparseable, so log here too.
func (c *ntmConfig) recordConfigFileError(msg string) {
	log.Error(msg)
	c.warnings = append(c.warnings, msg)
	c.configFileError = msg
}

// describeConfigSource names the config source for error messages.
func describeConfigSource(filePath string) string {
	if filePath == "" {
		return "the provided configuration content"
	}
	return filePath
}

// formatYAMLErrorContext quotes the offending line and its neighbours, marking the
// offender with '>'. Returns an empty string if the error carries no usable line.
func formatYAMLErrorContext(content []byte, err error) string {
	match := yamlErrorLineRe.FindStringSubmatch(err.Error())
	if match == nil {
		return ""
	}
	target, convErr := strconv.Atoi(match[1])
	if convErr != nil || target < 1 {
		return ""
	}

	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	// A trailing newline leaves a final empty element that is not a real line.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if target > len(lines) {
		return ""
	}

	first := max(1, target-yamlErrorContextRadius)
	last := min(len(lines), target+yamlErrorContextRadius)
	width := len(strconv.Itoa(last))

	var out strings.Builder
	for n := first; n <= last; n++ {
		marker := "  "
		if n == target {
			marker = "> "
		}
		fmt.Fprintf(&out, "\n%s%*d | %s", marker, width, n, lines[n-1])
	}
	return out.String()
}
