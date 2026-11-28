// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ScrubCRManifest scrubs sensitive information from a Custom Resource Manifest
func ScrubCRManifest(r *unstructured.Unstructured, scrubber *DataScrubber) {
	// Custom resources can store configuration outside spec, such as Kong's config.
	// Metadata is processed separately. Preserve resource identity fields.
	fields := make(map[string]interface{}, len(r.Object))
	for key, value := range r.Object {
		switch key {
		case "apiVersion", "kind", "metadata":
			continue
		default:
			fields[key] = value
		}
	}
	scrubMap(fields, scrubber, false)
	for key, value := range fields {
		r.Object[key] = value
	}
}

// scrubMap recursively scrubs sensitive values in a map
func scrubMap(m map[string]interface{}, scrubber *DataScrubber, parentSensitive bool) {
	scrubUnstructuredCommand(m, scrubber)

	for k, v := range m {
		lowerK := strings.ToLower(k)
		isSensitiveKey := scrubber.ContainsSensitiveWord(lowerK)
		shouldRedact := parentSensitive || isSensitiveKey

		if shouldRedact {
			if _, ok := v.(string); ok {
				m[k] = redactedSecret
				continue
			}
		}

		if k == "env" {
			switch env := v.(type) {
			case []interface{}:
				scrubEnv(env, scrubber, shouldRedact)
			case map[string]interface{}:
				scrubEnvMap(env, scrubber, shouldRedact)
			}
			continue
		}

		switch val := v.(type) {
		case map[string]interface{}:
			scrubMap(val, scrubber, shouldRedact)
		case []interface{}:
			scrubSlice(val, scrubber, shouldRedact)
		}
	}
}

// scrubUnstructuredCommand processes command and args together so that a
// sensitive option also redacts its value in the next element or in args.
func scrubUnstructuredCommand(m map[string]interface{}, scrubber *DataScrubber) {
	command, _ := m["command"].([]interface{})
	args, _ := m["args"].([]interface{})
	scrubbedCommand, scrubbedArgs, changed := scrubber.scrubCommandArguments(commandStringValues(command), commandStringValues(args))
	if !changed {
		return
	}
	for i, value := range command {
		if _, ok := value.(string); ok {
			command[i] = scrubbedCommand[i]
		}
	}
	for i, value := range args {
		if _, ok := value.(string); ok {
			args[i] = scrubbedArgs[i]
		}
	}
}

// commandStringValues keeps non-string entries as empty slots, preserving
// argument positions and the executable's index when scanning mixed arrays.
func commandStringValues(values []interface{}) []string {
	command := make([]string, len(values))
	for i, value := range values {
		command[i], _ = value.(string)
	}
	return command
}

// scrubCommandArguments preserves the original argument boundaries. A regex
// match masks the whole argument containing the value, including any spaces.
func (ds *DataScrubber) scrubCommandArguments(cmd, args []string) ([]string, []string, bool) {
	original := make([]string, 0, len(cmd)+len(args))
	original = append(original, cmd...)
	original = append(original, args...)
	cmdline := append([]string(nil), original...)
	rawCmdline := " " + strings.Join(original, " ")
	firstParam := 0
	if len(cmd) > 0 {
		firstParam = 1
	}
	changed := ds.scrubCommandTokens(cmdline, firstParam)
	for _, pattern := range ds.RegexSensitivePatterns {
		valueIndex := pattern.SubexpIndex("value")
		if valueIndex < 0 {
			continue
		}
		argIndex, offset := 0, 1
		// Matches are ordered, so advance through the argument boundaries once
		// per pattern. Keep the last overlapping argument for later matches
		// that may fall within the same argument.
		for _, match := range pattern.FindAllStringSubmatchIndex(rawCmdline, -1) {
			start, end := match[2*valueIndex], match[2*valueIndex+1]
			if start < 0 {
				continue
			}
			for argIndex < len(original) {
				argEnd := offset + len(original[argIndex])
				overlapsValue := start < argEnd && end > offset
				emptyValue := start == end && start >= offset && start <= argEnd
				if overlapsValue || emptyValue {
					cmdline[argIndex] = redactedSecret
					changed = true
				}
				if argEnd >= end {
					break
				}
				offset = argEnd + 1
				argIndex++
			}
		}
	}
	return cmdline[:len(cmd)], cmdline[len(cmd):], changed
}

// scrubSlice recursively scrubs sensitive values in a slice
func scrubSlice(a []interface{}, scrubber *DataScrubber, parentSensitive bool) {
	for i, item := range a {
		switch item := item.(type) {
		case string:
			if scrubber.ContainsSensitiveWord(item) || parentSensitive {
				a[i] = redactedSecret
			}
		case map[string]interface{}:
			scrubMap(item, scrubber, parentSensitive)
		case []interface{}:
			scrubSlice(item, scrubber, parentSensitive)
		}
	}
}

// scrubEnv scrubs sensitive values in an env slice
func scrubEnv(env []interface{}, scrubber *DataScrubber, parentSensitive bool) {
	for _, item := range env {
		if item, ok := item.(map[string]interface{}); ok {
			if name, ok := item["name"].(string); ok {
				if _, ok := item["value"].(string); ok {
					if isSensitiveEnvVarName(name, scrubber) || parentSensitive {
						item["value"] = redactedSecret
					}
				}
			}
		}
	}
}

// scrubEnvMap scrubs sensitive values from map-form environment variables.
func scrubEnvMap(env map[string]interface{}, scrubber *DataScrubber, parentSensitive bool) {
	for name, value := range env {
		if _, ok := value.(string); ok && (isSensitiveEnvVarName(name, scrubber) || parentSensitive) {
			env[name] = redactedSecret
		}
	}
}

func isSensitiveEnvVarName(name string, scrubber *DataScrubber) bool {
	lowerName := strings.ToLower(name)
	return scrubber.ContainsSensitiveWord(name) || lowerName == "token" || strings.HasSuffix(lowerName, "_token")
}
