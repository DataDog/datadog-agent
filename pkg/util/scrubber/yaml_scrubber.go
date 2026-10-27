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
	"strings"

	"go.yaml.in/yaml/v3"
)

type scrubCallback = func(string, interface{}) (bool, interface{})

func walkSlice(data []interface{}, callback scrubCallback) {
	for i, k := range data {
		switch v := k.(type) {
		case map[interface{}]interface{}:
			walkHash(v, callback)
		case []interface{}:
			walkSlice(v, callback)
		case map[string]interface{}:
			walkStringMap(v, callback)
		case string:
			if match, newValue := callback("", v); match {
				data[i] = newValue
			}
		}
	}
}

func walkHash(data map[interface{}]interface{}, callback scrubCallback) {
	for k, v := range data {
		if keyString, ok := k.(string); ok {
			if match, newValue := callback(keyString, v); match {
				data[keyString] = newValue
				continue
			}
		}

		switch v := data[k].(type) {
		case map[interface{}]interface{}:
			walkHash(v, callback)
		case []interface{}:
			walkSlice(v, callback)
		}
	}
}

func walkStringMap(data map[string]interface{}, callback scrubCallback) {
	for k, v := range data {
		if match, newValue := callback(k, v); match {
			data[k] = newValue
			continue
		}
		switch v := data[k].(type) {
		case map[string]interface{}:
			walkStringMap(v, callback)
		case []interface{}:
			walkSlice(v, callback)
		}

	}
}

// walk will go through loaded data and call callback on every strings allowing
// the callback to overwrite the string value
func walk(data *interface{}, callback scrubCallback) {
	if data == nil {
		return
	}

	switch v := (*data).(type) {
	case map[interface{}]interface{}:
		walkHash(v, callback)
	case []interface{}:
		walkSlice(v, callback)
	case map[string]interface{}:
		walkStringMap(v, callback)
	case string:
		if match, newValue := callback("", v); match {
			*data = newValue
		}
	}
}

// ScrubDataObj scrubs credentials from the data interface by recursively walking over all the nodes
func (c *Scrubber) ScrubDataObj(data *interface{}) {
	walk(data, c.scrubDataValue)
}

func (c *Scrubber) scrubDataValue(key string, value interface{}) (bool, interface{}) {
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
			if replacer.ProcessValue != nil {
				result := replacer.ProcessValue(value)
				// If ProcessValue returned a string, still apply the value-content pass
				// so embedded credentials (e.g. API keys in a JSON-encoded string) get scrubbed.
				if resultStr, ok := result.(string); ok {
					return true, c.scrubYamlText(resultStr)
				}
				return true, result
			}
			return true, defaultReplacement
		}
	}

	if isString {
		scrubbed := c.scrubYamlText(str)
		if scrubbed != str {
			return true, scrubbed
		}
	}
	return false, ""
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

	if err := c.scrubYamlNode(&document, make(map[*yaml.Node]struct{})); err != nil {
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

func (c *Scrubber) scrubYamlNode(node *yaml.Node, visited map[*yaml.Node]struct{}) error {
	if node == nil {
		return nil
	}
	if _, found := visited[node]; found {
		return nil
	}
	visited[node] = struct{}{}

	node.HeadComment = ""
	node.LineComment = ""
	node.FootComment = ""

	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if err := c.scrubYamlNode(child, visited); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			keyValue := key.Value
			var decodedKey interface{}
			if err := key.Decode(&decodedKey); err != nil {
				return err
			}
			if decodedKeyString, ok := decodedKey.(string); ok {
				keyValue = decodedKeyString
			}
			if err := c.scrubYamlNode(key, visited); err != nil {
				return err
			}

			var decoded interface{}
			if err := value.Decode(&decoded); err != nil {
				return err
			}
			if matched, replacement := c.scrubYamlDataValue(keyValue, decoded); matched {
				replaceYamlNode(value, replacement)
				if err := c.scrubYamlNode(value, visited); err != nil {
					return err
				}
			} else if err := c.scrubYamlNode(value, visited); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		var decoded interface{}
		if err := node.Decode(&decoded); err != nil {
			return err
		}
		if str, ok := decoded.(string); ok {
			if matched, replacement := c.scrubDataValue("", str); matched {
				replaceYamlNode(node, replacement)
			}
		}
	case yaml.AliasNode:
		return errors.New("YAML aliases are not supported")
	}
	return nil
}

func (c *Scrubber) scrubYamlText(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = string(c.scrub([]byte(line), c.singleLineReplacers, true))
	}
	joined := strings.Join(lines, "\n")
	return string(c.scrub([]byte(joined), c.multiLineReplacers, false))
}

func (c *Scrubber) scrubYamlDataValue(key string, value interface{}) (bool, interface{}) {
	if matched, replacement := c.scrubDataValue(key, value); matched {
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
		if replacer.ProcessValue != nil {
			return true, replacer.ProcessValue(value)
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
