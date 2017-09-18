// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package containers

import (
	"fmt"
	"math"
	"time"

	log "github.com/cihub/seelog"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

type dockerConfig struct {
	//Url                    string             `yaml:"url"`
	//CollectEvent           bool               `yaml:"collect_events"`
	//FilteredEventType      []string           `yaml:"filtered_event_types"`
	CollectContainerSize bool `yaml:"collect_container_size"`
	//CustomCGroup           bool               `yaml:"custom_cgroups"`
	//HealthServiceWhitelist []string           `yaml:"health_service_check_whitelist"`
	//CollectContainerCount  bool               `yaml:"collect_container_count"`
	//CollectVolumCount      bool               `yaml:"collect_volume_count"`
	//CollectImagesStats     bool               `yaml:"collect_images_stats"`
	//CollectImageSize       bool               `yaml:"collect_image_size"`
	//CollectDistStats       bool               `yaml:"collect_disk_stats"`
	//CollectExitCodes       bool               `yaml:"collect_exit_codes"`
	//ExcludeContainers      []string           `yaml:"exclude"`
	//IncludeContainers      []string           `yaml:"include"`
	//Tags                   []string           `yaml:"tags"`
	//ECSTags                []string           `yaml:"ecs_tags"`
	//PerformanceTags        []string           `yaml:"performance_tags"`
	//ContainrTags           []string           `yaml:"container_tags"`
	//EventAttributesAsTags  []string           `yaml:"event_attributes_as_tags"`
	//CappedMetrics          map[string]float64 `yaml:"capped_metrics"`
}

func (c *dockerConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}

	return nil
}

// DockerCheck grabs docker metrics
type DockerCheck struct {
	lastWarnings []error
	instance     *dockerConfig
}

// Run executes the check
func (d *DockerCheck) Run() error {
	sender, err := aggregator.GetSender(d.ID())

	containers, err := docker.AllContainers()
	if err != nil {
		return fmt.Errorf("Could not list containers: %s", err)
	}

	for _, c := range containers {
		tags := []string{fmt.Sprintf("image:%s", c.Image), fmt.Sprintf("image_id:%s", c.ImageID)}
		sender.Rate("docker.cpu.system", float64(c.CPU.System), "", tags)
		sender.Rate("docker.cpu.user", float64(c.CPU.User), "", tags)
		sender.Rate("docker.cpu.usage", c.CPU.UsageTotal, "", tags)
		sender.Rate("docker.cpu.throttled", float64(c.CPUNrThrottled), "", tags)
		sender.Gauge("docker.mem.cache", float64(c.Memory.Cache), "", tags)
		sender.Gauge("docker.mem.rss", float64(c.Memory.RSS), "", tags)
		if c.Memory.SwapPresent == true {
			sender.Gauge("docker.mem.swap", float64(c.Memory.Swap), "", tags)
		}

		if c.Memory.HierarchicalMemoryLimit > 0 && c.Memory.HierarchicalMemoryLimit < uint64(math.Pow(2, 60)) {
			sender.Gauge("docker.mem.limit", float64(c.Memory.HierarchicalMemoryLimit), "", tags)
			if c.Memory.HierarchicalMemoryLimit != 0 {
				sender.Gauge("docker.mem.in_use", float64(c.Memory.RSS/c.Memory.HierarchicalMemoryLimit), "", tags)
			}
		}

		if c.Memory.HierarchicalMemSWLimit > 0 && c.Memory.HierarchicalMemSWLimit < uint64(math.Pow(2, 60)) {
			sender.Gauge("docker.mem.sw_limit", float64(c.Memory.HierarchicalMemSWLimit), "", tags)
			if c.Memory.HierarchicalMemSWLimit != 0 {
				sender.Gauge("docker.mem.sw_in_use",
					float64((c.Memory.Swap+c.Memory.RSS)/c.Memory.HierarchicalMemSWLimit), "", tags)
			}
		}

		sender.Rate("docker.io.read_bytes", float64(c.IO.ReadBytes), "", tags)
		sender.Rate("docker.io.write_bytes", float64(c.IO.WriteBytes), "", tags)

		sender.Rate("docker.net.bytes_sent", float64(c.Network.BytesSent), "", tags)
		sender.Rate("docker.net.bytes_rcvd", float64(c.Network.BytesRcvd), "", tags)

		if d.instance.CollectContainerSize {
			info, err := c.Inspect(true)
			if err != nil {
				log.Errorf("Failed to inspect container %s - %s", c.ID[:12], err)
			} else if info.SizeRw == nil || info.SizeRootFs == nil {
				log.Warnf("Docker inspect did not return the container size: %s", c.ID[:12])
			} else {
				sender.Gauge("docker.container.size_rw", float64(*info.SizeRw), "", tags)
				sender.Gauge("docker.container.size_rootfs", float64(*info.SizeRootFs), "", tags)
			}
		}
	}
	sender.Commit()
	return nil
}

// Stop does nothing
func (d *DockerCheck) Stop() {}

func (d *DockerCheck) String() string {
	return "docker"
}

// Configure parses the check configuration and init the check
func (d *DockerCheck) Configure(config, initConfig check.ConfigData) error {
	docker.InitDockerUtil(&docker.Config{
		CacheDuration:  10 * time.Second,
		CollectNetwork: true,
	})
	d.instance = &dockerConfig{}
	d.instance.Parse(config)
	return nil
}

// Interval returns the scheduling time for the check
func (d *DockerCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (d *DockerCheck) ID() check.ID {
	return check.ID(d.String())
}

// GetWarnings grabs the last warnings from the sender
func (d *DockerCheck) GetWarnings() []error {
	w := d.lastWarnings
	d.lastWarnings = []error{}
	return w
}

// GetMetricStats returns the stats from the last run of the check
func (d *DockerCheck) GetMetricStats() (map[string]int64, error) {
	sender, err := aggregator.GetSender(d.ID())
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve a Sender instance: %v", err)
	}
	return sender.GetMetricStats(), nil
}

func dockerFactory() check.Check {
	return &DockerCheck{}
}

func init() {
	core.RegisterCheck("docker", dockerFactory)
}
