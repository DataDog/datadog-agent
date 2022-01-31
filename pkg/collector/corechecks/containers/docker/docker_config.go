// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import "gopkg.in/yaml.v2"

// checkName constants used to call ServiceCheck
const (
	DockerServiceUp = "docker.service_up"
	DockerExit      = "docker.exit"
)

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
	CollectEvent             bool               `yaml:"collect_events"`
	FilteredEventType        []string           `yaml:"filtered_event_types"`
	CappedMetrics            map[string]float64 `yaml:"capped_metrics"`
}

func (c *DockerConfig) Parse(data []byte) error {
	// default values
	c.CollectEvent = true
	c.CollectContainerSizeFreq = 5

	return yaml.Unmarshal(data, c)
}
