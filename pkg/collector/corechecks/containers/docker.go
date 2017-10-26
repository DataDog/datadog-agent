// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package containers

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

const dockerCheckName = "docker"

type DockerConfig struct {
	CollectContainerSize bool               `yaml:"collect_container_size"`
	CollectExitCodes     bool               `yaml:"collect_exit_codes"`
	CollectImagesStats   bool               `yaml:"collect_images_stats"`
	CollectImageSize     bool               `yaml:"collect_image_size"`
	CollectDiskStats     bool               `yaml:"collect_disk_stats"`
	CollectVolumeCount   bool               `yaml:"collect_volume_count"`
	Tags                 []string           `yaml:"tags"`
	CollectEvent         bool               `yaml:"collect_events"`
	FilteredEventType    []string           `yaml:"filtered_event_types"`
	CappedMetrics        map[string]float64 `yaml:"capped_metrics"`
	//CustomCGroup           bool               `yaml:"custom_cgroups"`
}

const (
	DockerServiceUp string = "docker.service_up"
	DockerExit      string = "docker.exit"
)

type containerPerImage struct {
	tags    []string
	running int64
	stopped int64
}

func (c *DockerConfig) Parse(data []byte) error {
	// default values
	c.CollectEvent = true

	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	return nil
}

// DockerCheck grabs docker metrics
type DockerCheck struct {
	lastWarnings   []error
	instance       *DockerConfig
	lastEventTime  time.Time
	dockerHostname string
	cappedSender   *cappedSender
}

func updateContainerRunningCount(images map[string]*containerPerImage, c *docker.Container) {
	imageTags, err := tagger.Tag(c.EntityID, false)
	if err != nil {
		log.Errorf("Could not collect tags for container %s: %s", c.ID[:12], err)
		return
	}

	sort.Strings(imageTags)
	key := strings.Join(imageTags, "|")
	if _, found := images[key]; !found {
		images[key] = &containerPerImage{tags: imageTags, running: 0, stopped: 0}
	}

	if c.State == docker.ContainerRunningState {
		images[key].running++
	} else if c.State == docker.ContainerExitedState {
		images[key].stopped++
	}
}

func (d *DockerCheck) countAndWeightImages(sender aggregator.Sender) error {
	if d.instance.CollectImagesStats == false {
		return nil
	}

	availableImages, err := docker.AllImages(false)
	if err != nil {
		return err
	}
	allImages, err := docker.AllImages(true)
	if err != nil {
		return err
	}

	if d.instance.CollectImageSize {
		for _, i := range availableImages {
			name, tag, err := docker.SplitImageName(i.RepoTags[0])
			if err != nil {
				log.Errorf("could not parse image name and tag, RepoTag is: %s", i.RepoTags[0])
				continue
			}
			tags := append(d.instance.Tags, fmt.Sprintf("image_name:%s", name), fmt.Sprintf("image_tag:%s", tag))

			sender.Gauge("docker.image.virtual_size", float64(i.VirtualSize), "", tags)
			sender.Gauge("docker.image.size", float64(i.Size), "", tags)
		}
	}
	sender.Gauge("docker.images.available", float64(len(availableImages)), "", d.instance.Tags)
	sender.Gauge("docker.images.intermediate", float64(len(allImages)-len(availableImages)), "", d.instance.Tags)
	return nil
}

// Run executes the check
func (d *DockerCheck) Run() error {
	sender, err := d.GetSender()

	containers, err := docker.AllContainers(&docker.ContainerListConfig{IncludeExited: true, FlagExcluded: true})
	if err != nil {
		sender.ServiceCheck(DockerServiceUp, metrics.ServiceCheckCritical, "", d.instance.Tags, err.Error())
		return err
	}

	images := map[string]*containerPerImage{}
	for _, c := range containers {
		updateContainerRunningCount(images, c)
		if c.State != docker.ContainerRunningState || c.Excluded {
			continue
		}
		tags, err := tagger.Tag(c.EntityID, true)
		if err != nil {
			log.Errorf("Could not collect tags for container %s: %s", c.ID[:12], err)
			tags = []string{}
		}
		tags = append(tags, d.instance.Tags...)

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

		if c.Network != nil {
			for _, netStat := range c.Network {
				if netStat.NetworkName == "" {
					log.Debugf("ignore network stat with empty name for container %s: %s", c.ID[:12], netStat)
					continue
				}
				ifaceTags := append(tags, fmt.Sprintf("docker_network:%s", netStat.NetworkName))
				sender.Rate("docker.net.bytes_sent", float64(netStat.BytesSent), "", ifaceTags)
				sender.Rate("docker.net.bytes_rcvd", float64(netStat.BytesRcvd), "", ifaceTags)
			}
		}

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

	for _, image := range images {
		sender.Gauge("docker.containers.running", float64(image.running), "", append(d.instance.Tags, image.tags...))
		sender.Gauge("docker.containers.stopped", float64(image.stopped), "", append(d.instance.Tags, image.tags...))
	}

	if err := d.countAndWeightImages(sender); err != nil {
		log.Error(err.Error())
		sender.ServiceCheck(DockerServiceUp, metrics.ServiceCheckCritical, "", d.instance.Tags, err.Error())
		return err
	}
	sender.ServiceCheck(DockerServiceUp, metrics.ServiceCheckOK, "", d.instance.Tags, "")

	if d.instance.CollectEvent || d.instance.CollectExitCodes {
		events, err := d.retrieveEvents()
		if err != nil {
			log.Warn(err.Error())
		} else {
			if d.instance.CollectEvent {
				err = d.reportEvents(events, sender)
				if err != nil {
					log.Warn(err.Error())
				}
			}
			if d.instance.CollectExitCodes {
				err = d.reportExitCodes(events, sender)
				if err != nil {
					log.Warn(err.Error())
				}
			}
		}
	}

	if d.instance.CollectDiskStats {
		stats, err := docker.GetStorageStats()
		if err != nil {
			log.Errorf("Failed to get disk stats: %s", err)
		} else {
			for _, stat := range stats {
				if stat.Name != docker.DataStorageName && stat.Name != docker.MetadataStorageName {
					log.Debugf("ignoring unknown disk stats: %s", stat)
					continue
				}
				if stat.Free != nil {
					sender.Gauge(fmt.Sprintf("docker.%s.free", stat.Name), float64(*stat.Free), "", d.instance.Tags)
				}
				if stat.Used != nil {
					sender.Gauge(fmt.Sprintf("docker.%s.used", stat.Name), float64(*stat.Used), "", d.instance.Tags)
				}
				if stat.Total != nil {
					sender.Gauge(fmt.Sprintf("docker.%s.total", stat.Name), float64(*stat.Total), "", d.instance.Tags)
				}
				percent := stat.GetPercentUsed()
				if !math.IsNaN(percent) {
					sender.Gauge(fmt.Sprintf("docker.%s.percent", stat.Name), percent, "", d.instance.Tags)
				}
			}
		}
	}

	if d.instance.CollectVolumeCount {
		attached, dangling, err := docker.CountVolumes()
		if err != nil {
			log.Errorf("failed to get volume stats: %s", err)
		} else {
			sender.Gauge("docker.volume.count", float64(attached), "", append(d.instance.Tags, "volume_state:attached"))
			sender.Gauge("docker.volume.count", float64(dangling), "", append(d.instance.Tags, "volume_state:dangling"))
		}
	}

	sender.Commit()
	return nil
}

// Stop does nothing
func (d *DockerCheck) Stop() {}

func (d *DockerCheck) String() string {
	return dockerCheckName
}

// Configure parses the check configuration and init the check
func (d *DockerCheck) Configure(config, initConfig check.ConfigData) error {
	d.instance.Parse(config)

	if len(d.instance.FilteredEventType) == 0 {
		d.instance.FilteredEventType = []string{"top", "exec_create", "exec_start"}
	}

	var err error
	d.dockerHostname, err = docker.GetHostname()
	if err != nil {
		log.Warnf("can't get hostname from docker, events will not have it: %s", err)
	}
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

// DockerFactory is exported for integration testing
func DockerFactory() check.Check {
	return &DockerCheck{
		instance: &DockerConfig{},
	}
}

func init() {
	core.RegisterCheck("docker", DockerFactory)
}
