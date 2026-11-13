// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"reflect"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ConfigSecret describes a secret declaration in a configuration file, never its value.
// Path is relative to the file, using dotted notation.
// Sequence indexes are path segments, for example instances.0.password.
type ConfigSecret struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Handle string `json:"handle,omitempty"`
}

// recordRawSecret returns true when the replacement is covered by a field-level
// reference, so its descendants do not need additional raw references.
func recordRawSecret(secrets *[]ConfigSecret, path string, before, after any) bool {
	if secrets == nil || !mayHoldCredential(before) || reflect.DeepEqual(before, after) {
		return false
	}
	if value, ok := before.(string); ok && (IsEnc(value) || value == defaultReplacement) {
		return false
	}
	*secrets = append(*secrets, ConfigSecret{Path: path, Kind: "raw"})
	return true
}

// Collect ENC references from validated YAML before replacements can hide them.
func (c *Scrubber) collectYamlEncSecrets(node *yaml.Node, path string, secrets *[]ConfigSecret) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			c.collectYamlEncSecrets(child, path, secrets)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			c.collectYamlEncSecrets(child, joinPath(path, strconv.Itoa(i)), secrets)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			keyValue := key.Value
			var decodedKey interface{}
			if err := key.Decode(&decodedKey); err != nil {
				continue
			}
			if decodedKeyString, ok := decodedKey.(string); ok {
				keyValue = decodedKeyString
			}
			c.collectYamlEncSecrets(node.Content[i+1], joinPath(path, c.scrubYamlText(keyValue)), secrets)
		}
	case yaml.ScalarNode:
		var value interface{}
		if err := node.Decode(&value); err != nil {
			return
		}
		if value, ok := value.(string); ok && IsEnc(value) {
			value = strings.TrimSpace(value)
			*secrets = append(*secrets, ConfigSecret{Path: path, Kind: "enc", Handle: value[4 : len(value)-1]})
		}
	}
}
