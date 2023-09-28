// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"fmt"
	"os"

	"encoding/json"
)

// walk will go through loaded json and call callback on every strings allowing
// the callback to overwrite the string value
func walkJSON(data *interface{}, callback scrubCallback) {
	if data == nil {
		return
	}
	switch m := (*data).(type) {
	case map[string]interface{}:
		for key, value := range m {
			if match, newValue := callback(key, value); match {
				m[key] = newValue //key matched, stop searching and do replacing
				continue
			}
			walkJSON(&value, callback) //Not matched in key, keep searching
		}
	case []interface{}:
		for _, k := range m {
			walkJSON(&k, callback)
		}
	}
}

// ScrubJSON scrubs credentials from the given json by loading the data and scrubbing the
// object instead of the serialized string.
func (c *Scrubber) ScrubJSON(input []byte) ([]byte, error) {
	var data *interface{}
	err := json.Unmarshal(input, &data)

	// if we can't load the json run the default scrubber on the input
	if len(input) != 0 && err == nil {
		walkJSON(data, func(key string, value interface{}) (bool, interface{}) {
			for _, replacer := range c.singleLineReplacers {
				if replacer.YAMLKeyRegex == nil {
					continue
				}
				if replacer.YAMLKeyRegex.Match([]byte(key)) {
					if replacer.ProcessValue != nil {
						return true, replacer.ProcessValue(value)
					}
					return true, defaultReplacement
				}
			}
			return false, ""
		})

		newInput, err := json.Marshal(data)
		if err == nil {
			input = newInput
		} else {
			// Since the scrubber is a dependency of the logger we can use it here.
			fmt.Fprintf(os.Stderr, "error scrubbing json, falling back on text scrubber: %s\n", err)
		}
	}
	return c.ScrubBytes(input)
}
