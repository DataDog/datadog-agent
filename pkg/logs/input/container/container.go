// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package container

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	label := c.getLabel()
	if label != "" {
		return c.parseLabel(label)
	}
	var candidate *config.LogSource
	for _, source := range sources {
		if source.Config.Image != "" && !c.isImageMatch(source.Config.Image) {
			continue
		}
		if source.Config.Label != "" && !c.isLabelMatch(source.Config.Label) {
			continue
		}
		if source.Config.Name != "" && !c.isNameMatch(source.Config.Name) {
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
	if c.isNameMatch(source.Config.Name) {
		score++
	}
	return score
}

// digestPrefix represents a prefix that can be added to an image name.
const digestPrefix = "@sha256:"

// tagSeparator represents the separator in between an image name and its tag.
const tagSeparator = ":"

// isImageMatch returns true if the image of the container matches with imageFilter.
// The image of a container can have the following formats:
// - '[<repository>/]image[:<tag>]',
// - '[<repository>/]image[@sha256:<digest>]'
// The imageFilter must respect the format '[<repository>/]image[:<tag>]'.
func (c *Container) isImageMatch(imageFilter string) bool {
	// Trim digest if present
	splitted := strings.SplitN(c.Image, digestPrefix, 2)
	image := splitted[0]
	if !strings.Contains(imageFilter, tagSeparator) {
		// trim tag if present
		splitted := strings.SplitN(image, tagSeparator, 2)
		image = splitted[0]
	}
	// Expect prefix to end with '/'
	repository := strings.TrimSuffix(image, imageFilter)
	return len(repository) == 0 || strings.HasSuffix(repository, "/")
}

// isNameMatch returns true if one of the container name matches with the filter.
func (c *Container) isNameMatch(nameFilter string) bool {
	re, err := regexp.Compile(nameFilter)
	if err != nil {
		log.Warn("used invalid name to filter containers: ", nameFilter)
		return false
	}
	for _, name := range c.Names {
		if re.MatchString(name) {
			return true
		}
	}
	return false
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

// getLabel returns the autodiscovery config label if it exists.
func (c *Container) getLabel() string {
	label, exists := c.Labels[configPath]
	if exists {
		return label
	}
	return ""
}

// parseLabel returns the config present in the container label 'com.datadoghq.ad.logs',
// the config has to be conform with the format '[{...}]'.
func (c *Container) parseLabel(label string) *config.LogSource {
	var cfgs []config.LogsConfig
	err := json.Unmarshal([]byte(label), &cfgs)
	if err != nil || len(cfgs) < 1 {
		log.Warnf("Could not parse logs config for container %v, %v is malformed", c.Container.ID, label)
		return nil
	}
	cfg := cfgs[0]
	err = config.ValidateProcessingRules(cfg.ProcessingRules)
	if err != nil {
		log.Warnf("Invalid processing rules for container %v: %v", c.Container.ID, err)
		return nil
	}
	cfg.Type = config.DockerType
	config.CompileProcessingRules(cfg.ProcessingRules)
	return config.NewLogSource(configPath, &cfg)
}
