// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package container

import (
	"encoding/json"
	"strings"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Container represents a container to tail logs from.
type Container struct {
	types.Container
}

// NewContainer returns a new Container
func NewContainer(container types.Container) *Container {
	return &Container{container}
}

// findSource returns the source that most closely matches the container,
// if no source is found return nil
func (c *Container) findSource(sources []*config.LogSource) *config.LogSource {
	if source := c.toSource(); source != nil {
		return source
	}
	var candidate *config.LogSource
	for _, source := range sources {
		if source.Config.Image != "" && !c.isImageMatch(source.Config.Image) {
			continue
		}
		if source.Config.Label != "" && !c.isLabelMatch(source.Config.Label) {
			continue
		}
		if candidate == nil {
			candidate = source
		}
		if c.computeScore(candidate) < c.computeScore(source) {
			candidate = source
		}
	}
	return candidate
}

// computeScore returns the matching score between the container and the source.
func (c *Container) computeScore(source *config.LogSource) int {
	score := 0
	if c.isImageMatch(source.Config.Image) {
		score++
	}
	if c.isLabelMatch(source.Config.Label) {
		score++
	}
	return score
}

// digestPrefix represents a prefix that can be added to an image name.
const digestPrefix = "@sha256:"

// isImageMatch returns true if container image respects format '[<repository>/]image[@sha256:<digest>]',
// imageFilter must respect format '[<repository>/]image'.
func (c *Container) isImageMatch(imageFilter string) bool {
	// Trim digest if present
	splitted := strings.SplitN(c.Image, digestPrefix, 2)
	image := splitted[0]
	// Expect prefix to end with '/'
	repository := strings.TrimSuffix(image, imageFilter)
	return len(repository) == 0 || strings.HasSuffix(repository, "/")
}

// isLabelMatch returns true if container labels contains at least one label from labelFilter.
func (c *Container) isLabelMatch(labelFilter string) bool {
	// Expect a comma-separated list of labels, eg: foo:bar, baz
	for _, value := range strings.Split(labelFilter, ",") {
		// Trim whitespace, then check whether the label format is either key:value or key=value
		label := strings.TrimSpace(value)
		parts := strings.FieldsFunc(label, func(c rune) bool {
			return c == ':' || c == '='
		})
		// If we have exactly two parts, check there is a container label that matches both.
		// Otherwise fall back to checking the whole label exists as a key.
		if _, exists := c.Labels[label]; exists || len(parts) == 2 && c.Labels[parts[0]] == parts[1] {
			return true
		}
	}
	return false
}

// configPath refers to the configuration that can be passed over a docker label,
// this feature is commonly named 'ad' or 'autodicovery'.
const configPath = "com.datadoghq.ad.logs"

// toSource converts a container to a source
func (c *Container) toSource() *config.LogSource {
	cfg := c.parseConfig()
	if cfg == nil {
		return nil
	}
	return config.NewLogSource(configPath, cfg)
}

// parseConfig returns the config present in the container label 'com.datadoghq.ad.logs',
// the config has to be conform with the format '[{...}]'.
func (c *Container) parseConfig() *config.LogsConfig {
	label, exists := c.Labels[configPath]
	if !exists {
		return nil
	}
	var configs []config.LogsConfig
	err := json.Unmarshal([]byte(label), &configs)
	if err != nil || len(configs) < 1 {
		log.Warnf("Could not parse logs configs, %v is malformed", label)
		return nil
	}
	config := configs[0]
	return &config
}
