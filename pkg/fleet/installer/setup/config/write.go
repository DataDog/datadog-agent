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

	"gopkg.in/yaml.v3"
)

func writeConfig(path string, config any, perms os.FileMode, merge bool) error {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}

	// Marshal the given `config` to yaml.Node
	updatedBytes, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	var updatedRoot yaml.Node
	if err := yaml.Unmarshal(updatedBytes, &updatedRoot); err != nil {
		return err
	}

	// Load original YAML into a node tree
	var originalBytes []byte
	if merge {
		// Read the original YAML (for preserving comments)
		originalBytes, err = os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		// Remove CR (\r) from originalBytes
		// TODO: There seems to be an issue with how the yaml package handles CRLF
		originalBytes = bytes.ReplaceAll(originalBytes, []byte("\r"), []byte(""))
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(originalBytes), &root); err != nil {
		return err
	}

	// Merge the updated `config` node tree into the original YAML
	rootIsEmpty := len(root.Content) == 0
	if len(root.Content) > 0 && len(updatedRoot.Content) > 0 {
		// Merge at the DocumentNode level to handle non-mapping roots (e.g., scalar or empty)
		mergeNodes(&root, &updatedRoot)
	} else if rootIsEmpty {
		root = updatedRoot
	}

	// Save result
	var buf bytes.Buffer
	if rootIsEmpty {
		// Add generated disclaimer
		if disclaimerGenerated != "" && !bytes.HasPrefix(originalBytes, []byte(disclaimerGenerated+"\n\n")) {
			buf.WriteString(disclaimerGenerated + "\n\n")
		}
		// file may contain only comments and those are not preserved by yaml.Node
		// write them manually here
		if len(originalBytes) > 0 {
			buf.WriteString(string(originalBytes))
			if !bytes.HasSuffix(originalBytes, []byte("\n")) {
				buf.WriteString("\n")
			}
		}
	}
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), perms)
}

// mergeNodes merges the src node into the dst node
//
// The values are merged as follows:
//   - If the value is a mapping, the nodes are merged recursively
//   - for other types, the src value overrides the dst value
func mergeNodes(dst *yaml.Node, src *yaml.Node) {
	// Handle top-level DocumentNode merging to support empty, scalar, and mapping roots
	if dst.Kind == yaml.DocumentNode && src.Kind == yaml.DocumentNode {
		// If source document has no content, nothing to merge
		if len(src.Content) == 0 {
			return
		}
		// If the destination document has no content, copy source content
		// Example: file with only comments
		if len(dst.Content) == 0 {
			dst.Content = src.Content[:]
			return
		}

		dstChild := dst.Content[0]
		srcChild := src.Content[0]

		if dstChild.Kind == yaml.MappingNode && srcChild.Kind == yaml.MappingNode {
			mergeNodes(dstChild, srcChild)
			return
		}

		// For non-mapping roots, replace destination root with source root
		// and copy node-level comments if missing on current.
		// Example: --- header and only a comment, no other fields.
		//          not sure if this is a yaml.Node bug or expected behavior...
		copyNodeComments(srcChild, dstChild)
		dst.Content[0] = srcChild
		return
	}

	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return
	}

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
				// If the value is a mapping, the nodes are merged recursively
				mergeNodes(dstVal, srcVal)
			} else {
				// for other types, the src value overrides the dst value

				// Replace node and copy node-level comments if missing on current
				copyNodeComments(srcVal, dstVal)
				dst.Content[idx+1] = srcVal
			}
		} else {
			dst.Content = append(dst.Content, srcKey, srcVal)
		}
	}
}

func copyNodeComments(dst *yaml.Node, src *yaml.Node) {
	if src.HeadComment != "" && dst.HeadComment == "" {
		dst.HeadComment = src.HeadComment
	}
	if src.LineComment != "" && dst.LineComment == "" {
		dst.LineComment = src.LineComment
	}
	if src.FootComment != "" && dst.FootComment == "" {
		dst.FootComment = src.FootComment
	}
}
