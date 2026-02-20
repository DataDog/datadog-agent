// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"golang.org/x/text/encoding/unicode"
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
		originalBytes, err = readConfig(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	var root yaml.Node
	if err := yaml.Unmarshal(originalBytes, &root); err != nil {
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

// readConfig returns the Agent config bytes from path and performs the following normalizations:
//   - Converts from UTF-16 to UTF-8
//   - Removes CR (\r) bytes
//
// the yaml package does its own decoding, but since we're stripping out CR (\r) bytes we need
// to decode the config, too.
func readConfig(path string) ([]byte, error) {
	originalBytes, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if len(originalBytes) == 0 {
		return originalBytes, nil
	}

	// Normalize encoding to UTF-8 if needed
	if originalBytes, err = ensureUTF8(originalBytes); err != nil {
		return nil, fmt.Errorf("%s is not valid UTF-8: %w", path, err)
	}

	// Remove CR (\r) from originalBytes after decoding
	// TODO: There seems to be an issue with how the yaml package handles CRLF
	originalBytes = bytes.ReplaceAll(originalBytes, []byte("\r"), []byte(""))

	return originalBytes, nil
}

// ensureUTF8 converts input bytes to UTF-8 if they are encoded as UTF-16 with BOM.
//
// Files created/edited on Windows are often written as UTF-16
//
// It also strips a UTF-8 BOM if present. If no BOM is found, the input is returned unchanged.
func ensureUTF8(input []byte) ([]byte, error) {
	// fast paths, check for BOMs

	// UTF-8 BOM: EF BB BF
	if len(input) >= 3 && input[0] == 0xEF && input[1] == 0xBB && input[2] == 0xBF {
		return input[3:], nil
	}

	// UTF-16 LE BOM: FF FE
	if len(input) >= 2 && input[0] == 0xFF && input[1] == 0xFE {
		utf16 := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
		utf8, err := utf16.NewDecoder().Bytes(input)
		if err != nil {
			return nil, fmt.Errorf("file has UTF-16 BOM, but failed to convert to UTF-8: %w", err)
		}
		return utf8, nil
	}

	// UTF-16 BE BOM: FE FF
	if len(input) >= 2 && input[0] == 0xFE && input[1] == 0xFF {
		utf16 := unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
		utf8, err := utf16.NewDecoder().Bytes(input)
		if err != nil {
			return nil, fmt.Errorf("has UTF-16 BOM, but failed to convert to UTF-8: %w", err)
		}
		return utf8, nil
	}

	// no BOM or unknown BOM

	// if contains null bytes, try to utf16 decode (assume LE)
	// UTF-8 text should not contain NUL bytes
	if bytes.Contains(input, []byte{0x00}) {
		utf16 := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
		utf8, err := utf16.NewDecoder().Bytes(input)
		if err != nil {
			return nil, fmt.Errorf("contains null bytes, but failed to convert from UTF-16 to UTF-8: %w", err)
		}
		return utf8, nil
	}

	// Ensure already UTF-8
	if !utf8.Valid(input) {
		return nil, errors.New("contains bytes that are not valid UTF-8")
	}

	return input, nil
}
