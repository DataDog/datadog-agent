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

const (
	secretKindRaw = "raw"
	secretKindEnc = "enc"
)

// ConfigSecret describes a secret declaration in a configuration file, never its value.
// Path is relative to the file, using dotted notation.
// Sequence indexes are path segments, for example instances.0.password.
type ConfigSecret struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Handle string `json:"handle,omitempty"`
}

func recordRawSecret(secrets *[]ConfigSecret, path string, before, after any) {
	if secrets == nil || !mayHoldCredential(before) || reflect.DeepEqual(before, after) {
		return
	}
	if value, ok := before.(string); ok && (IsEnc(value) || value == defaultReplacement) {
		return
	}
	*secrets = append(*secrets, ConfigSecret{Path: path, Kind: secretKindRaw})
}

// recordYamlRawSecrets compares a replaced subtree with its fully scrubbed result.
// Descend where keys and indexes still match; otherwise reference the whole field.
func (c *Scrubber) recordYamlRawSecrets(secrets *[]ConfigSecret, path string, before, after *yaml.Node) {
	if secrets == nil {
		return
	}
	if before.Kind == after.Kind && len(before.Content) == len(after.Content) {
		switch before.Kind {
		case yaml.SequenceNode:
			for i, child := range before.Content {
				c.recordYamlRawSecrets(secrets, joinPath(path, strconv.Itoa(i)), child, after.Content[i])
			}
			return
		case yaml.MappingNode:
			if children, paths, ok := c.matchYamlSecretChildren(before, after); ok {
				for i, child := range children {
					c.recordYamlRawSecrets(secrets, joinPath(path, paths[i]), before.Content[2*i+1], child)
				}
				return
			}
		}
	}

	// Decode values so formatting changes, such as 0x1f90 becoming 8080,
	// do not create secret declarations.
	var original, scrubbed any
	if before.Decode(&original) != nil {
		return
	}
	if after.Decode(&scrubbed) != nil {
		// Scrubbed keys may collide. Retain the field reference when the
		// rewritten mapping cannot be decoded for a finer comparison.
		scrubbed = nil
	}
	recordRawSecret(secrets, path, original, scrubbed)
}

// matchYamlSecretChildren matches mapping values independently of YAML key order.
// Scrub string keys before matching and exposing them in metadata paths. If keys
// collide after scrubbing, use the parent reference instead of guessing a match.
func (c *Scrubber) matchYamlSecretChildren(before, after *yaml.Node) ([]*yaml.Node, []string, bool) {
	decodeKey := func(node *yaml.Node) (any, string, bool) {
		var key any
		if err := node.Decode(&key); err != nil || (key != nil && !reflect.TypeOf(key).Comparable()) {
			return nil, "", false
		}
		path := node.Value
		if str, ok := key.(string); ok {
			path = str
			key = c.scrubYamlText(str)
		}
		return key, c.scrubYamlText(path), true
	}

	values := make(map[any]*yaml.Node, len(after.Content)/2)
	for i := 0; i+1 < len(after.Content); i += 2 {
		key, _, ok := decodeKey(after.Content[i])
		if _, duplicate := values[key]; !ok || duplicate {
			return nil, nil, false
		}
		values[key] = after.Content[i+1]
	}
	children := make([]*yaml.Node, 0, len(before.Content)/2)
	paths := make([]string, 0, len(before.Content)/2)
	for i := 0; i+1 < len(before.Content); i += 2 {
		key, path, ok := decodeKey(before.Content[i])
		child, found := values[key]
		if !ok || !found {
			return nil, nil, false
		}
		delete(values, key)
		children = append(children, child)
		paths = append(paths, path)
	}
	return children, paths, true
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
			*secrets = append(*secrets, ConfigSecret{Path: path, Kind: secretKindEnc, Handle: value[4 : len(value)-1]})
		}
	}
}
