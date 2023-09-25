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

// ScrubJSON scrubs credentials from the given json by loading the data and scrubbing the
// object instead of the serialized string. It calls the helper function walk in ScrubYaml
func (c *Scrubber) ScrubJSON(input []byte) ([]byte, error) {
	var data *interface{}
	err := json.Unmarshal(input, &data)

	// if we can't load the yaml run the default scrubber on the input
	if len(input) != 0 && err == nil {
		c.walk(data, func(key string, value interface{}) (bool, interface{}) {
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
