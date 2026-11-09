// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ConfigSecret describes a secret declaration in a configuration file, never its value.
// Path is relative to the file, using the dotted notation of enc_info.
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
	cleaned, err := c.ScrubYamlPreserveStructure(input)
	if err != nil {
		return nil, nil, err
	}
	var original, scrubbed yaml.Node
	if err := yaml.Unmarshal(input, &original); err != nil {
		return nil, nil, err
	}
	if err := yaml.Unmarshal(cleaned, &scrubbed); err != nil {
		return nil, nil, err
	}
	declarations := []ConfigSecret{}
	c.configSecrets(&original, &scrubbed, "", &declarations)
	return cleaned, declarations, nil
}

func (c *Scrubber) configSecrets(original, cleaned *yaml.Node, path string, out *[]ConfigSecret) {
	switch original.Kind {
	case yaml.DocumentNode:
		if len(original.Content) != 0 {
			var child *yaml.Node
			if cleaned != nil && cleaned.Kind == yaml.DocumentNode && len(cleaned.Content) != 0 {
				child = cleaned.Content[0]
			}
			c.configSecrets(original.Content[0], child, path, out)
		}
	case yaml.MappingNode:
		// Look up by key because structured replacers can reorder mappings.
		children := map[string]*yaml.Node{}
		if cleaned != nil && cleaned.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(cleaned.Content); i += 2 {
				children[cleaned.Content[i].Value] = cleaned.Content[i+1]
			}
		}
		for i := 0; i+1 < len(original.Content); i += 2 {
			key := original.Content[i].Value
			// A URL used as a map key can itself contain credentials. Do not
			// bypass scrubbing by copying it into the metadata path.
			safeKey := c.scrubYamlText(key)
			c.configSecrets(original.Content[i+1], children[safeKey], joinPath(path, safeKey), out)
		}
	case yaml.SequenceNode:
		for i, child := range original.Content {
			var scrubbedChild *yaml.Node
			if cleaned != nil && cleaned.Kind == yaml.SequenceNode && i < len(cleaned.Content) {
				scrubbedChild = cleaned.Content[i]
			}
			c.configSecrets(child, scrubbedChild, joinPath(path, strconv.Itoa(i)), out)
		}
	case yaml.ScalarNode:
		if original.Tag == "!!str" && IsEnc(original.Value) {
			value := strings.TrimSpace(original.Value)
			*out = append(*out, ConfigSecret{Path: path, Kind: "enc", Handle: value[4 : len(value)-1]})
			return
		}
		if original.Tag == "!!null" || original.Tag == "!!bool" || strings.TrimSpace(original.Value) == "" || original.Value == defaultReplacement {
			return
		}
		if cleaned == nil || cleaned.Kind != yaml.ScalarNode || original.Value != cleaned.Value {
			*out = append(*out, ConfigSecret{Path: path, Kind: "raw"})
		}
	}
}
