// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"bytes"
	"fmt"
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
	walk(data, func(key string, value interface{}) (bool, interface{}) {
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
						lines := strings.Split(resultStr, "\n")
						for i, line := range lines {
							lines[i] = string(c.scrub([]byte(line), c.singleLineReplacers, true))
						}
						joined := strings.Join(lines, "\n")
						scrubbed := string(c.scrub([]byte(joined), c.multiLineReplacers, false))
						return true, scrubbed
					}
					return true, result
				}
				return true, defaultReplacement
			}
		}

		if isString {
			// Apply single-line replacers per line so regexes like `\bBearer\s+[^*]+\b`
			// (which match newlines via `[^*]`) can't consume content from following lines.
			lines := strings.Split(str, "\n")
			for i, line := range lines {
				lines[i] = string(c.scrub([]byte(line), c.singleLineReplacers, true))
			}
			joined := strings.Join(lines, "\n")
			scrubbed := string(c.scrub([]byte(joined), c.multiLineReplacers, false))
			if scrubbed != str {
				return true, scrubbed
			}
		}
		return false, ""
	})
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
