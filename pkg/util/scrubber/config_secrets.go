// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
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

// ScrubYamlWithSecrets uses the default scrubber, including dynamically registered sensitive keys.
func ScrubYamlWithSecrets(input []byte) ([]byte, []ConfigSecret, error) {
	return DefaultScrubber.ScrubYamlWithSecrets(input)
}

// ScrubYamlWithSecrets returns a safely scrubbed file and its secret declarations.
// Raw locations are derived from the same scrubbing operation as the returned file;
// already-redacted values and non-credential booleans/empty values are not reported.
// Like ScrubYamlPreserveStructure, unsafe YAML is rejected without returning metadata.
func (c *Scrubber) ScrubYamlWithSecrets(input []byte) ([]byte, []ConfigSecret, error) {
	declarations := []ConfigSecret{}
	cleaned, err := c.scrubYamlPreserveStructure(input, func(path string, original, cleaned *yaml.Node) {
		if original.Tag == "!!str" && IsEnc(original.Value) {
			value := strings.TrimSpace(original.Value)
			declarations = append(declarations, ConfigSecret{Path: path, Kind: "enc", Handle: value[4 : len(value)-1]})
			return
		}
		if original.Tag == "!!null" || original.Tag == "!!bool" || strings.TrimSpace(original.Value) == "" || original.Value == defaultReplacement {
			return
		}
		if cleaned == nil || cleaned.Kind != yaml.ScalarNode || original.Value != cleaned.Value {
			declarations = append(declarations, ConfigSecret{Path: path, Kind: "raw"})
		}
	})
	if err != nil {
		return nil, nil, err
	}
	return cleaned, declarations, nil
}
