// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// scrubCallback is called on every map entry and on every string in a slice. key is the map key, or "" for slice
// elements, while path is the full dotted path to the entry, where slice elements use their index (ex:
// "instances.0.community_string"). path is "" for a top-level string.
type scrubCallback = func(key string, path string, value interface{}) (bool, interface{})

// joinPath appends elem to the dotted path prefix
func joinPath(prefix string, elem string) string {
	if prefix == "" {
		return elem
	}
	return prefix + "." + elem
}

func walkSlice(data []interface{}, prefix string, callback scrubCallback) {
	for i, k := range data {
		path := joinPath(prefix, strconv.Itoa(i))
		switch v := k.(type) {
		case map[interface{}]interface{}:
			walkHash(v, path, callback)
		case []interface{}:
			walkSlice(v, path, callback)
		case map[string]interface{}:
			walkStringMap(v, path, callback)
		case string:
			if match, newValue := callback("", path, v); match {
				data[i] = newValue
			}
		}
	}
}

func walkHash(data map[interface{}]interface{}, prefix string, callback scrubCallback) {
	for k, v := range data {
		keyString, isString := k.(string)
		path := joinPath(prefix, fmt.Sprint(k))
		if isString {
			if match, newValue := callback(keyString, path, v); match {
				data[keyString] = newValue
				continue
			}
		}

		switch v := data[k].(type) {
		case map[interface{}]interface{}:
			walkHash(v, path, callback)
		case []interface{}:
			walkSlice(v, path, callback)
		}
	}
}

func walkStringMap(data map[string]interface{}, prefix string, callback scrubCallback) {
	for k, v := range data {
		path := joinPath(prefix, k)
		if match, newValue := callback(k, path, v); match {
			data[k] = newValue
			continue
		}
		switch v := data[k].(type) {
		case map[string]interface{}:
			walkStringMap(v, path, callback)
		case []interface{}:
			walkSlice(v, path, callback)
		}

	}
}

// walk will go through loaded data and call callback on every strings allowing
// the callback to overwrite the string value. prefix is the path of data itself.
func walk(data *interface{}, prefix string, callback scrubCallback) {
	if data == nil {
		return
	}

	switch v := (*data).(type) {
	case map[interface{}]interface{}:
		walkHash(v, prefix, callback)
	case []interface{}:
		walkSlice(v, prefix, callback)
	case map[string]interface{}:
		walkStringMap(v, prefix, callback)
	case string:
		if match, newValue := callback("", prefix, v); match {
			*data = newValue
		}
	}
}

// ScrubDataObj scrubs credentials from the data interface by recursively walking over all the nodes. The full paths of
// the settings whose values get scrubbed are recorded, see GetStrippedKeys.
func (c *Scrubber) ScrubDataObj(data *interface{}) {
	c.scrubDataObjAt(data, "")
}

// scrubDataObjAt is like ScrubDataObj for data found at the given path, which prefixes the recorded paths
func (c *Scrubber) scrubDataObjAt(data *interface{}, prefix string) {
	walk(data, prefix, c.scrubDataValue)
}

func (c *Scrubber) scrubDataValue(key string, path string, value interface{}) (bool, interface{}) {
	str, isString := value.(string)
	if isString && IsEnc(str) {
		return false, ""
	}

	for _, replacer := range c.singleLineReplacers {
		if replacer.YAMLKeyRegex == nil {
			continue
		}

		if c.shouldApply != nil && !c.shouldApply(replacer) {
			continue
		}

		lowerKey := strings.ToLower(key)
		if replacer.YAMLKeyRegex.Match([]byte(lowerKey)) {
			if replacer.hasProcessValue() {
				result := replacer.processValue(value, path)
				// If ProcessValue returned a string, still apply the value-content pass
				// so embedded credentials (e.g. API keys in a JSON-encoded string) get scrubbed.
				if resultStr, ok := result.(string); ok {
					if mayHoldCredential(value) {
						c.addStrippedKey(path)
					}
					return true, c.scrubYamlText(resultStr)
				}
				// Structured results (ex: additional_endpoints, private_action_runner) are only recorded
				// if they contain a scrubbed leaf. Replacers that re-enter ScrubDataObj record nested keys.
				if containsReplacement(result) {
					c.addStrippedKey(path)
				}
				return true, result
			}
			if mayHoldCredential(value) {
				c.addStrippedKey(path)
			}
			return true, defaultReplacement
		}
	}

	if isString {
		scrubbed := c.scrubYamlText(str)
		if scrubbed != str {
			// the value itself looks like a credential (ex: a raw API key or a password in a URL)
			c.addStrippedKey(path)
			return true, scrubbed
		}
	}
	return false, ""
}

// mayHoldCredential returns false if value holds no data that could be a credential, such as an empty value or a
// boolean. This avoids recording keys like "pass_through: false" that only match a sensitive key pattern.
func mayHoldCredential(value interface{}) bool {
	switch v := value.(type) {
	case nil, bool:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case map[string]interface{}:
		return len(v) != 0
	case map[interface{}]interface{}:
		return len(v) != 0
	case []interface{}:
		return len(v) != 0
	}
	return true
}

// containsReplacement returns true if any leaf of data was replaced by defaultReplacement
func containsReplacement(data interface{}) bool {
	switch v := data.(type) {
	case string:
		return v == defaultReplacement
	case map[string]interface{}:
		for _, vv := range v {
			if containsReplacement(vv) {
				return true
			}
		}
	case map[interface{}]interface{}:
		for _, vv := range v {
			if containsReplacement(vv) {
				return true
			}
		}
	case []interface{}:
		for _, vv := range v {
			if containsReplacement(vv) {
				return true
			}
		}
	}
	return false
}

// ScrubYaml scrubs credentials from the given YAML by loading the data and scrubbing the object instead of the
// serialized string.
func (c *Scrubber) ScrubYaml(input []byte) ([]byte, error) {
	var data *interface{}
	err := yaml.Unmarshal(input, &data)

	// if we can't load the yaml run the default scrubber on the input
	if len(input) != 0 && err == nil {
		c.ScrubDataObj(data)

		var buffer bytes.Buffer
		encoder := yaml.NewEncoder(&buffer)
		encoder.SetIndent(2)
		if err := encoder.Encode(&data); err != nil {
			fmt.Fprintf(os.Stderr, "error scrubbing YAML, falling back on text scrubber: %s\n", err)
		} else {
			input = buffer.Bytes()
		}
		encoder.Close()
	}
	return c.ScrubBytes(input)
}

// ScrubYamlPreserveStructure scrubs credentials from YAML while preserving the
// document's key order and scalar styles. Unlike ScrubYaml, it walks
// the YAML node tree instead of unmarshalling into maps, which avoids turning a
// configuration file into an interpreted and reordered view of its contents.
// Comments are omitted because they are free-form text and YAML key-based
// replacers cannot safely identify secrets in them.
//
// Invalid YAML is rejected instead of falling back to the text scrubber. The
// fallback cannot apply YAML-only replacers and is therefore not safe for data
// that is sent automatically as metadata.
func (c *Scrubber) ScrubYamlPreserveStructure(input []byte) ([]byte, error) {
	return c.scrubYamlPreserveStructure(input, nil)
}

// yamlScalarObserver observes original scalar values at their sanitized paths after
// scrubbing. It belongs to one call, never to the shared Scrubber. A nil cleaned
// node means a containing replacement removed the scalar from the output tree.
type yamlScalarObserver func(path string, original, cleaned *yaml.Node)

func (c *Scrubber) scrubYamlPreserveStructure(input []byte, observe yamlScalarObserver) ([]byte, error) {
	if len(input) == 0 {
		return []byte{}, nil
	}

	decoder := yaml.NewDecoder(bytes.NewReader(input))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return []byte{}, nil
		}
		return nil, err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple YAML documents are not supported")
		}
		return nil, err
	}
	if len(document.Content) == 0 {
		return []byte{}, nil
	}
	if containsYamlAlias(&document) {
		return nil, errors.New("YAML aliases are not supported")
	}
	var validation interface{}
	if err := document.Decode(&validation); err != nil {
		return nil, err
	}

	var original *yaml.Node
	if observe != nil {
		original = &document
	}
	if err := c.scrubYamlNode(&document, original, "", observe); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// scrubYamlNode scrubs and observes in one traversal. Structured replacements
// can reorder or collapse subtrees, so original children are matched by key or
// index rather than losing their declarations when the replacement is applied.
func (c *Scrubber) scrubYamlNode(node, original *yaml.Node, path string, observe yamlScalarObserver) error {
	if original != nil {
		snapshot := *original
		original = &snapshot
	}
	var originalChildren map[string]*yaml.Node
	var originalItems []*yaml.Node
	if original != nil {
		switch original.Kind {
		case yaml.MappingNode:
			originalChildren = make(map[string]*yaml.Node, len(original.Content)/2)
			for i := 0; i+1 < len(original.Content); i += 2 {
				key, err := yamlKeyString(original.Content[i])
				if err != nil {
					return err
				}
				originalChildren[key] = original.Content[i+1]
			}
		case yaml.SequenceNode, yaml.DocumentNode:
			originalItems = original.Content
		}
	}

	if node != nil {
		node.HeadComment = ""
		node.LineComment = ""
		node.FootComment = ""
		switch node.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for i, child := range node.Content {
				childPath := path
				if node.Kind == yaml.SequenceNode {
					childPath = joinPath(path, strconv.Itoa(i))
				}
				var originalChild *yaml.Node
				if i < len(originalItems) {
					originalChild = originalItems[i]
				}
				if err := c.scrubYamlNode(child, originalChild, childPath, observe); err != nil {
					return err
				}
			}
			if len(node.Content) < len(originalItems) {
				originalItems = originalItems[len(node.Content):]
			} else {
				originalItems = nil
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(node.Content); i += 2 {
				key, value := node.Content[i], node.Content[i+1]
				keyValue, err := yamlKeyString(key)
				if err != nil {
					return err
				}
				originalValue := originalChildren[keyValue]
				delete(originalChildren, keyValue)
				// Snapshot before a parent rule replaces this node in place.
				if originalValue != nil {
					snapshot := *originalValue
					originalValue = &snapshot
				}
				if err := c.scrubYamlNode(key, nil, path, nil); err != nil {
					return err
				}
				valuePath := joinPath(path, c.scrubYamlText(keyValue))
				var decoded interface{}
				if err := value.Decode(&decoded); err != nil {
					return err
				}
				if matched, replacement := c.scrubYamlDataValue(keyValue, valuePath, decoded); matched {
					replaceYamlNode(value, replacement)
				}
				if err := c.scrubYamlNode(value, originalValue, valuePath, observe); err != nil {
					return err
				}
			}
		case yaml.ScalarNode:
			var decoded interface{}
			if err := node.Decode(&decoded); err != nil {
				return err
			}
			if str, ok := decoded.(string); ok {
				if matched, replacement := c.scrubDataValue("", path, str); matched {
					replaceYamlNode(node, replacement)
				}
			}
		case yaml.AliasNode:
			return errors.New("YAML aliases are not supported")
		}
	}

	// Visit original leaves hidden by a replacement (for example a redacted
	// community_strings list), using the same traversal and call-local observer.
	if len(originalChildren) != 0 {
		// Keep source order for removed mapping children, too.
		for i := 0; i+1 < len(original.Content); i += 2 {
			key, err := yamlKeyString(original.Content[i])
			if err != nil {
				return err
			}
			if child := originalChildren[key]; child != nil {
				if err := c.scrubYamlNode(nil, child, joinPath(path, c.scrubYamlText(key)), observe); err != nil {
					return err
				}
			}
		}
	}
	for i, child := range originalItems {
		childPath := path
		if original.Kind == yaml.SequenceNode {
			childPath = joinPath(path, strconv.Itoa(len(original.Content)-len(originalItems)+i))
		}
		if err := c.scrubYamlNode(nil, child, childPath, observe); err != nil {
			return err
		}
	}
	if original != nil && original.Kind == yaml.ScalarNode && observe != nil {
		observe(path, original, node)
	}
	return nil
}

func yamlKeyString(node *yaml.Node) (string, error) {
	var decoded interface{}
	if err := node.Decode(&decoded); err != nil {
		return "", err
	}
	if key, ok := decoded.(string); ok {
		return key, nil
	}
	return node.Value, nil
}

func (c *Scrubber) scrubYamlText(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = string(c.scrub([]byte(line), c.singleLineReplacers, true))
	}
	joined := strings.Join(lines, "\n")
	return string(c.scrub([]byte(joined), c.multiLineReplacers, false))
}

func (c *Scrubber) scrubYamlDataValue(key string, path string, value interface{}) (bool, interface{}) {
	if matched, replacement := c.scrubDataValue(key, path, value); matched {
		return true, replacement
	}

	lowerKey := strings.ToLower(key)
	for _, replacer := range c.multiLineReplacers {
		if replacer.YAMLKeyRegex == nil || !replacer.YAMLKeyRegex.MatchString(lowerKey) {
			continue
		}
		if c.shouldApply != nil && !c.shouldApply(replacer) {
			continue
		}
		if replacer.hasProcessValue() {
			return true, replacer.processValue(value, path)
		}
		return true, defaultReplacement
	}
	return false, ""
}

func containsYamlAlias(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode {
		return true
	}
	for _, child := range node.Content {
		if containsYamlAlias(child) {
			return true
		}
	}
	return false
}

func replaceYamlNode(node *yaml.Node, value interface{}) {
	anchor := node.Anchor
	style := node.Style
	originalKind := node.Kind

	var replacement yaml.Node
	if err := replacement.Encode(value); err != nil {
		replacement = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: defaultReplacement}
	}
	if originalKind == yaml.ScalarNode && replacement.Kind == yaml.ScalarNode {
		replacement.Style = style
		if strings.HasPrefix(node.Tag, "!") && !strings.HasPrefix(node.Tag, "!!") {
			replacement.Tag = node.Tag
		}
	}
	replacement.Anchor = anchor
	*node = replacement
}
