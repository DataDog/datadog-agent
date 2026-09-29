// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import "go.yaml.in/yaml/v3"

// CheckName constants used to call ServiceCheck
const (
	DockerServiceUp = "docker.service_up"
	DockerExit      = "docker.exit"
)

// DockerConfig holds the docker check configuration
type DockerConfig struct {
	CappedMetrics map[string]float64 `yaml:"capped_metrics"`
	OkExitCodes   []int              `yaml:"ok_exit_codes"`
	Tags          []string           `yaml:"tags"` // Used only by the configuration converter v5 → v6
	// FilteredEventTypes is a slice of docker event types that works as a
	// deny list of events to filter out.
	FilteredEventType []string `yaml:"filtered_event_types"`
	// CollectedEventTypes is a slice of docker event types to collect.
	CollectedEventTypes      []string `yaml:"collected_event_types"`
	CollectContainerSizeFreq uint64   `yaml:"collect_container_size_frequency"`
	CollectContainerSize     bool     `yaml:"collect_container_size"`
	CollectExitCodes         bool     `yaml:"collect_exit_codes"`
	CollectImagesStats       bool     `yaml:"collect_images_stats"`
	CollectImageSize         bool     `yaml:"collect_image_size"`
	CollectDiskStats         bool     `yaml:"collect_disk_stats"`
	CollectVolumeCount       bool     `yaml:"collect_volume_count"`
	// Event collection configuration
	CollectEvent            bool `yaml:"collect_events"`
	UnbundleEvents          bool `yaml:"unbundle_events"`
	BundleUnspecifiedEvents bool `yaml:"bundle_unspecified_events"`
}

// Parse reads the docker check configuration
func (c *DockerConfig) Parse(data []byte) error {
	// default values
	c.CollectEvent = true
	c.CollectContainerSizeFreq = 5

	return yaml.Unmarshal(data, c)
}
