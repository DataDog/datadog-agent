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
	Identifier string
	Image      string
	Source     *config.LogSource
}

// NewContainer returns a new Container
func NewContainer(container types.Container, source *config.LogSource) *Container {
	return &Container{
		Identifier: container.ID,
		Image:      container.Image,
		Source:     source,
	}
}

// Filter returns all the containers for which a source exists.
func Filter(containers []types.Container, sources []*config.LogSource) []*Container {
	containersToTail := []*Container{}
	for _, container := range containers {
		if source := searchSource(container, sources); source != nil {
			containersToTail = append(containersToTail, NewContainer(container, source))
		}
	}
	return containersToTail
}

// searchSource returns a valid source for container,
// if no source is found, return nil
func searchSource(container types.Container, sources []*config.LogSource) *config.LogSource {
	if source := sourceFromContainer(container); source != nil {
		return source
	}
	var candidate *config.LogSource
	for _, source := range sources {
		if source.Config.Image != "" && !isImageMatch(source.Config.Image, container.Image) {
			continue
		}
		if source.Config.Label != "" && !isLabelMatch(source.Config.Label, container.Labels) {
			continue
		}
		if candidate == nil {
			candidate = source
		}
		if computeScore(container, candidate) < computeScore(container, source) {
			candidate = source
		}
	}
	return candidate
}

// computeScore returns the matching score between the container and the source,
// the higher, the better
func computeScore(container types.Container, source *config.LogSource) int {
	score := 0
	if isImageMatch(source.Config.Image, container.Image) {
		score++
	}
	if isLabelMatch(source.Config.Label, container.Labels) {
		score++
	}
	return score
}

// digestPrefix represents a prefix that can be added to an image name.
const digestPrefix = "@sha256:"

// isImageMatch returns true if image respects format: "Y/imageFilter@sha256:X" with 'Y/' and '@sha256:X' optional.
func isImageMatch(imageFilter string, image string) bool {
	if strings.Contains(image, digestPrefix) {
		// Trim digest if present
		splitted := strings.SplitN(image, digestPrefix, 2)
		image = splitted[0]
	}
	// Expect prefix to end with '/'
	repository := strings.TrimSuffix(image, imageFilter)
	return len(repository) == 0 || strings.HasSuffix(repository, "/")
}

// isLabelMatch returns true if labels contains at least one label from labelFilter.
func isLabelMatch(labelFilter string, labels map[string]string) bool {
	// Expect a comma-separated list of labels, eg: foo:bar, baz
	for _, value := range strings.Split(labelFilter, ",") {
		// Trim whitespace, then check whether the label format is either key:value or key=value
		label := strings.TrimSpace(value)
		parts := strings.FieldsFunc(label, func(c rune) bool {
			return c == ':' || c == '='
		})
		// If we have exactly two parts, check there is a container label that matches both.
		// Otherwise fall back to checking the whole label exists as a key.
		if _, exists := labels[label]; exists || len(parts) == 2 && labels[parts[0]] == parts[1] {
			return true
		}
	}
	return false
}

// logsConfigPath refers to the logs configuration that can be passed over a docker label,
// this feature is commonly named autodicovery.
const logsConfigPath = "com.datadoghq.ad.logs"

// sourceFromContainer returns the source extracted and computed from the container labels.
func sourceFromContainer(container types.Container) *config.LogSource {
	logsConfig := extractLogsConfig(container.Labels)
	if logsConfig == nil {
		return nil
	}
	return config.NewLogSource(logsConfigPath, logsConfig)
}

// extractLogsConfig returns the logs config present in the label 'com.datadoghq.ad.logs',
// the config has to be conform with the format '[{...}]'.
func extractLogsConfig(labels map[string]string) *config.LogsConfig {
	label, exists := labels[logsConfigPath]
	if !exists {
		return nil
	}
	var configs []config.LogsConfig
	err := json.Unmarshal([]byte(label), &configs)
	if err != nil || len(configs) < 1 {
		log.Warnf("Could not parse logs configs, got %v, expect value with format '[{\"source\":\"a_source\",\"service\":\"a_service\", ...}]'")
		return nil
	}
	logsConfig := configs[0]
	return &logsConfig
}
