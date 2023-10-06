// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import "gopkg.in/yaml.v2"

// checkName constants used to call ServiceCheck
const (
	DockerServiceUp = "docker.service_up"
	DockerExit      = "docker.exit"
)

// DockerConfig holds the docker check configuration
type DockerConfig struct {
	CollectContainerSize     bool               `yaml:"collect_container_size"`
	CollectContainerSizeFreq uint64             `yaml:"collect_container_size_frequency"`
	CollectExitCodes         bool               `yaml:"collect_exit_codes"`
	OkExitCodes              []int              `yaml:"ok_exit_codes"`
	CollectImagesStats       bool               `yaml:"collect_images_stats"`
	CollectImageSize         bool               `yaml:"collect_image_size"`
	CollectDiskStats         bool               `yaml:"collect_disk_stats"`
	CollectVolumeCount       bool               `yaml:"collect_volume_count"`
	Tags                     []string           `yaml:"tags"` // Used only by the configuration converter v5 → v6
	CappedMetrics            map[string]float64 `yaml:"capped_metrics"`

	// Event collection configuration
	CollectEvent   bool `yaml:"collect_events"`
	UnbundleEvents bool `yaml:"unbundle_events"`

	// FilteredEventTypes is a slice of docker event types that works as a
	// deny list of events to filter out. Only effective when
	// UnbundleEvents = false
	FilteredEventType []string `yaml:"filtered_event_types"`

	// CollectedEventTypes is a slice of docker event types to collect.
	// Only effective when UnbundleEvents = true
	CollectedEventTypes []string `yaml:"collected_event_types"`
}

// Parse reads the docker check configuration
func (c *DockerConfig) Parse(data []byte) error {
	// default values
	c.CollectEvent = true
	c.CollectContainerSizeFreq = 5

	return yaml.Unmarshal(data, c)
}
