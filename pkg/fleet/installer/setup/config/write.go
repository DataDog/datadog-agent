// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func writeConfig(path string, config any, perms os.FileMode, merge bool) error {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}
	var originalBytes []byte
	if merge {
		// Read the original YAML (for preserving comments)
		originalBytes, err = os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	lines := strings.Split(string(originalBytes), "\n")

	// Step 1: Marshal the given `config` to YAML bytes
	updatedBytes, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	// Step 2: Unmarshal that into a typed node tree
	var updatedRoot yaml.Node
	if err := yaml.Unmarshal(updatedBytes, &updatedRoot); err != nil {
		return err
	}

	// Step 3: Replace commented-out scalar keys (like "# site: xxx")
	updatedLines := replaceCommentedKeysInLines(lines, &updatedRoot)
	updatedYamlText := strings.Join(updatedLines, "\n")

	// Step 4: Load original YAML into a node tree
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(updatedYamlText), &root); err != nil {
		return err
	}

	// Step 5: Merge the updated `config` node tree into the original YAML
	if len(root.Content) > 0 && len(updatedRoot.Content) > 0 {
		mergeNodes(root.Content[0], updatedRoot.Content[0])
	} else if len(root.Content) == 0 {
		root = updatedRoot
	}

	// Step 6: Save result
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), perms)
}

var commentedKeyValueRegex = regexp.MustCompile(`^(\s*)#\s*([\w\-]+)\s*:\s*(.*?)(\s*)(#.*)?$`)

type commentedLine struct {
	lineIndex           int
	leadingSpaces       string
	key                 string
	value               string
	spacesBeforeComment string
	trailingComment     string
}

func parseCommentedKeysFromLines(lines []string) map[string]commentedLine {
	result := map[string]commentedLine{}
	for i, line := range lines {
		m := commentedKeyValueRegex.FindStringSubmatch(line)
		if m != nil {
			result[m[2]] = commentedLine{
				lineIndex:           i,
				leadingSpaces:       m[1],
				key:                 m[2],
				value:               m[3],
				spacesBeforeComment: m[4],
				trailingComment:     m[5],
			}
		}
	}
	return result
}

func replaceCommentedKeysInLines(lines []string, updatedRoot *yaml.Node) []string {
	commentedKeys := parseCommentedKeysFromLines(lines)

	// Detect existing non-commented top-level keys
	actualKeys := make(map[string]bool)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, ":") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		key := strings.TrimSpace(parts[0])
		if key != "" {
			actualKeys[key] = true
		}
	}

	// Build a map of top-level scalar values from updated YAML
	updatedValues := map[string]*yaml.Node{}
	if updatedRoot.Kind == yaml.DocumentNode && len(updatedRoot.Content) > 0 {
		mapping := updatedRoot.Content[0]
		if mapping.Kind == yaml.MappingNode {
			for i := 0; i < len(mapping.Content); i += 2 {
				key := mapping.Content[i].Value
				val := mapping.Content[i+1]
				if val.Kind == yaml.ScalarNode {
					updatedValues[key] = val
				}
			}
		}
	}

	updatedLines := make([]string, len(lines))
	copy(updatedLines, lines)

	for key, commentLine := range commentedKeys {
		valNode, ok := updatedValues[key]
		if !ok {
			continue
		}
		newLine := fmt.Sprintf("%s%s: %s%s%s",
			commentLine.leadingSpaces,
			key,
			valNode.Value,
			commentLine.spacesBeforeComment,
			commentLine.trailingComment,
		)
		updatedLines[commentLine.lineIndex] = newLine
	}

	return updatedLines
}

func mergeNodes(dst *yaml.Node, src *yaml.Node) {
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return
	}

	// Build key->index for existing keys in dst
	keyIndex := make(map[string]int)
	for i := 0; i < len(dst.Content); i += 2 {
		key := dst.Content[i].Value
		keyIndex[key] = i
	}

	for i := 0; i < len(src.Content); i += 2 {
		srcKey := src.Content[i]
		srcVal := src.Content[i+1]

		if idx, found := keyIndex[srcKey.Value]; found {
			dstVal := dst.Content[idx+1]
			if dstVal.Kind == yaml.MappingNode && srcVal.Kind == yaml.MappingNode {
				// Recursively merge nested maps
				mergeNodes(dstVal, srcVal)
			} else {
				dst.Content[idx+1] = srcVal
			}
		} else {
			dst.Content = append(dst.Content, srcKey, srcVal)
		}
	}
}
