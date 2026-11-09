// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import "reflect"

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
// Raw declarations identify the fields replaced by scrubbing, including whole
// containers. ENC declarations retain every original path and handle.
// Like ScrubYamlPreserveStructure, unsafe YAML is rejected without returning metadata.
func (c *Scrubber) ScrubYamlWithSecrets(input []byte) ([]byte, []ConfigSecret, error) {
	declarations := []ConfigSecret{}
	cleaned, err := c.scrubYamlPreserveStructure(input, &declarations)
	if err != nil {
		return nil, nil, err
	}
	return cleaned, declarations, nil
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
