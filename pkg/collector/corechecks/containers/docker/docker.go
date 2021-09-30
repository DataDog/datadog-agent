// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && !darwin
// +build docker,!darwin

package docker

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	cmetrics "github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const dockerCheckName = "docker"

type containerPerImage struct {
	tags    []string
	running int64
	stopped int64
}

// DockerCheck grabs docker metrics
type DockerCheck struct {
	core.CheckBase
	instance                    *DockerConfig
	lastEventTime               time.Time
	dockerHostname              string
	cappedSender                *cappedSender
	collectContainerSizeCounter uint64
}

func updateContainerRunningCount(images map[string]*containerPerImage, c *containers.Container) {
	var containerTags []string
	var err error

	// Containers that are not running (either pending or stopped) are not
	// collected by the tagger, so they won't have any tags and will cause
	// an expensive tagger fetch.  To have *some* tags for stopped
	// containers, we treat them as excluded containers and make a
	// synthetic list of tags from basic container information.
	if c.Excluded || c.State != containers.ContainerRunningState {
		// TODO we can do SplitImageName because we are in the docker corecheck and the image name is not a sha[...]
		// We should resolve the image tags in the tagger as a real entity.
		long, short, tag, err := containers.SplitImageName(c.Image)
		if err != nil {
			log.Errorf("Cannot split the image name %s: %v", c.Image, err)
			return
		}
		containerTags = []string{
			fmt.Sprintf("docker_image:%s", c.Image),
			fmt.Sprintf("image_name:%s", long),
			fmt.Sprintf("image_tag:%s", tag),
			fmt.Sprintf("short_image:%s", short),
		}
	} else {
		containerTags, err = tagger.Tag(c.EntityID, collectors.LowCardinality)
		if err != nil {
			log.Errorf("Could not collect tags for container %s: %s", c.ID[:12], err)
			return
		}
		sort.Strings(containerTags)
	}

	key := strings.Join(containerTags, "|")
	if _, found := images[key]; !found {
		images[key] = &containerPerImage{tags: containerTags, running: 0, stopped: 0}
	}

	if c.State == containers.ContainerRunningState {
		images[key].running++
	} else if c.State == containers.ContainerExitedState {
		images[key].stopped++
	}
}

func (d *DockerCheck) countAndWeightImages(sender aggregator.Sender, imageTagsByImageID map[string][]string, du *docker.DockerUtil) error {
	if d.instance.CollectImagesStats == false {
		return nil
	}

	availableImages, err := du.Images(context.TODO(), false)
	if err != nil {
		return err
	}
	allImages, err := du.Images(context.TODO(), true)
	if err != nil {
		return err
	}

	if d.instance.CollectImageSize {
		for _, i := range availableImages {
			if tags, ok := imageTagsByImageID[i.ID]; ok {
				sender.Gauge("docker.image.virtual_size", float64(i.VirtualSize), "", tags)
				sender.Gauge("docker.image.size", float64(i.Size), "", tags)
			} else {
				log.Tracef("Skipping image %s, no repo tags", i.ID)
			}
		}
	}
	sender.Gauge("docker.images.available", float64(len(availableImages)), "", nil)
	sender.Gauge("docker.images.intermediate", float64(len(allImages)-len(availableImages)), "", nil)
	return nil
}

// Run executes the check
func (d *DockerCheck) Run() error {
	sender, err := d.GetSender()
	if err != nil {
		return err
	}

	du, err := docker.GetDockerUtil()
	if err != nil {
		sender.ServiceCheck(DockerServiceUp, metrics.ServiceCheckCritical, "", nil, err.Error())
		d.Warnf("Error initialising check: %s", err) //nolint:errcheck
		return err
	}
	cList, err := du.ListContainers(context.TODO(), &docker.ContainerListConfig{IncludeExited: true, FlagExcluded: true})
	if err != nil {
		sender.ServiceCheck(DockerServiceUp, metrics.ServiceCheckCritical, "", nil, err.Error())
		d.Warnf("Error collecting containers: %s", err) //nolint:errcheck
		return err
	}

	collectingContainerSizeDuringThisRun := d.instance.CollectContainerSize && d.collectContainerSizeCounter == 0

	imageTagsByImageID := make(map[string][]string)
	images := map[string]*containerPerImage{}
	for _, c := range cList {
		updateContainerRunningCount(images, c)
		if c.State != containers.ContainerRunningState || c.Excluded {
			continue
		}
		tags, err := tagger.Tag(c.EntityID, collectors.HighCardinality)
		if err != nil {
			log.Errorf("Could not collect tags for container %s: %s", c.ID[:12], err)
		}
		// Track image_name and image_tag tags by image for use in countAndWeightImages
		for _, t := range tags {
			if strings.HasPrefix(t, "image_name:") || strings.HasPrefix(t, "image_tag:") {
				if _, found := imageTagsByImageID[c.ImageID]; !found {
					imageTagsByImageID[c.ImageID] = []string{t}
				} else {
					imageTagsByImageID[c.ImageID] = append(imageTagsByImageID[c.ImageID], t)
				}
			}
		}

		currentUnixTime := time.Now().Unix()
		d.reportUptime(c.StartedAt, currentUnixTime, tags, sender)

		if c.CPU != nil {
			d.reportCPUMetrics(c.CPU, &c.Limits, c.StartedAt, tags, sender)
		} else {
			log.Debugf("Empty CPU metrics for container %s", c.ID[:12])
		}

		if c.Memory != nil {
			sender.Gauge("docker.mem.cache", float64(c.Memory.Cache), "", tags)
			sender.Gauge("docker.mem.rss", float64(c.Memory.RSS), "", tags)
			if c.Memory.SwapPresent == true {
				sender.Gauge("docker.mem.swap", float64(c.Memory.Swap), "", tags)
			}

			if c.Memory.HierarchicalMemoryLimit > 0 && c.Memory.HierarchicalMemoryLimit < uint64(math.Pow(2, 60)) {
				sender.Gauge("docker.mem.limit", float64(c.Memory.HierarchicalMemoryLimit), "", tags)
				sender.Gauge("docker.mem.in_use", float64(c.Memory.RSS)/float64(c.Memory.HierarchicalMemoryLimit), "", tags)
			} else if c.Limits.MemLimit > 0 && c.Memory.CommitBytes > 0 {
				// On Windows the mem limit is in container limits
				sender.Gauge("docker.mem.limit", float64(c.Limits.MemLimit), "", tags)
				sender.Gauge("docker.mem.in_use", float64(c.Memory.CommitBytes)/float64(c.Limits.MemLimit), "", tags)
			}

			sender.Gauge("docker.mem.failed_count", float64(c.Memory.MemFailCnt), "", tags)
			if c.Memory.HierarchicalMemSWLimit > 0 && c.Memory.HierarchicalMemSWLimit < uint64(math.Pow(2, 60)) {
				sender.Gauge("docker.mem.sw_limit", float64(c.Memory.HierarchicalMemSWLimit), "", tags)
				if c.Memory.HierarchicalMemSWLimit != 0 {
					sender.Gauge("docker.mem.sw_in_use",
						float64(c.Memory.Swap+c.Memory.RSS)/float64(c.Memory.HierarchicalMemSWLimit), "", tags)
				}
			}

			sender.Gauge("docker.kmem.usage", float64(c.Memory.KernMemUsage), "", tags)
			if c.Memory.SoftMemLimit > 0 && c.Memory.SoftMemLimit < uint64(math.Pow(2, 60)) {
				sender.Gauge("docker.mem.soft_limit", float64(c.Memory.SoftMemLimit), "", tags)
			}

			if c.Memory.PrivateWorkingSet > 0 {
				sender.Gauge("docker.mem.private_working_set", float64(c.Memory.PrivateWorkingSet), "", tags)
			}
			if c.Memory.CommitBytes > 0 {
				sender.Gauge("docker.mem.commit_bytes", float64(c.Memory.CommitBytes), "", tags)
			}
			if c.Memory.CommitPeakBytes > 0 {
				sender.Gauge("docker.mem.commit_peak_bytes", float64(c.Memory.CommitPeakBytes), "", tags)
			}
		} else {
			log.Debugf("Empty memory metrics for container %s", c.ID[:12])
		}

		if c.IO != nil {
			d.reportIOMetrics(c.IO, tags, sender)
		} else {
			log.Debugf("Empty IO metrics for container %s", c.ID[:12])
		}

		if c.Limits.ThreadLimit != 0 {
			sender.Gauge("docker.thread.limit", float64(c.Limits.ThreadLimit), "", tags)
		}

		if c.Network != nil {
			for _, netStat := range c.Network {
				if netStat.NetworkName == "" {
					log.Debugf("Ignore network stat with empty name for container %s", c.ID[:12])
					continue
				}
				ifaceTags := append(tags, fmt.Sprintf("docker_network:%s", netStat.NetworkName))
				sender.Rate("docker.net.bytes_sent", float64(netStat.BytesSent), "", ifaceTags)
				sender.Rate("docker.net.bytes_rcvd", float64(netStat.BytesRcvd), "", ifaceTags)
			}
		} else {
			log.Debugf("Empty network metrics for container %s", c.ID[:12])
		}

		if collectingContainerSizeDuringThisRun {
			info, err := du.Inspect(context.TODO(), c.ID, true)
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

	if d.instance.CollectContainerSize {
		// Update the container size counter, used to collect them less often as they are costly
		d.collectContainerSizeCounter =
			(d.collectContainerSizeCounter + 1) % d.instance.CollectContainerSizeFreq
	}

	var totalRunning, totalStopped int64
	for _, image := range images {
		sender.Gauge("docker.containers.running", float64(image.running), "", image.tags)
		totalRunning += image.running
		sender.Gauge("docker.containers.stopped", float64(image.stopped), "", image.tags)
		totalStopped += image.stopped
	}
	sender.Gauge("docker.containers.running.total", float64(totalRunning), "", nil)
	sender.Gauge("docker.containers.stopped.total", float64(totalStopped), "", nil)

	if err := d.countAndWeightImages(sender, imageTagsByImageID, du); err != nil {
		log.Error(err.Error())
		sender.ServiceCheck(DockerServiceUp, metrics.ServiceCheckCritical, "", nil, err.Error())
		d.Warnf("Error collecting images: %s", err) //nolint:errcheck
		return err
	}
	sender.ServiceCheck(DockerServiceUp, metrics.ServiceCheckOK, "", nil, "")

	if d.instance.CollectEvent || d.instance.CollectExitCodes {
		events, err := d.retrieveEvents(du)
		if err != nil {
			d.Warnf("Error collecting events: %s", err) //nolint:errcheck
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
		stats, err := du.GetStorageStats(context.TODO())
		if err != nil {
			d.Warnf("Error collecting disk stats: %s", err) //nolint:errcheck
		} else {
			for _, stat := range stats {
				if stat.Name != docker.DataStorageName && stat.Name != docker.MetadataStorageName {
					log.Debugf("Ignoring unknown disk stats: %s", stat.Name)
					continue
				}
				if stat.Free != nil {
					sender.Gauge(fmt.Sprintf("docker.%s.free", stat.Name), float64(*stat.Free), "", nil)
				}
				if stat.Used != nil {
					sender.Gauge(fmt.Sprintf("docker.%s.used", stat.Name), float64(*stat.Used), "", nil)
				}
				if stat.Total != nil {
					sender.Gauge(fmt.Sprintf("docker.%s.total", stat.Name), float64(*stat.Total), "", nil)
				}
				percent := stat.GetPercentUsed()
				if !math.IsNaN(percent) {
					sender.Gauge(fmt.Sprintf("docker.%s.percent", stat.Name), percent, "", nil)
				}
			}
		}
	}

	if d.instance.CollectVolumeCount {
		attached, dangling, err := du.CountVolumes(context.TODO())
		if err != nil {
			d.Warnf("Error collecting volume stats: %s", err) //nolint:errcheck
		} else {
			sender.Gauge("docker.volume.count", float64(attached), "", []string{"volume_state:attached"})
			sender.Gauge("docker.volume.count", float64(dangling), "", []string{"volume_state:dangling"})
		}
	}

	sender.Commit()
	return nil
}

func (d *DockerCheck) reportUptime(startTime, currentUnixTime int64, tags []string, sender aggregator.Sender) {
	if startTime != 0 && currentUnixTime-startTime > 0 {
		sender.Gauge("docker.uptime", float64(currentUnixTime-startTime), "", tags)
	}
}

func (d *DockerCheck) reportCPUMetrics(cpu *cmetrics.ContainerCPUStats, limits *cmetrics.ContainerLimits, startTime int64, tags []string, sender aggregator.Sender) {
	if cpu == nil {
		return
	}

	if cpu.System != -1 {
		sender.Rate("docker.cpu.system", cpu.System, "", tags)
	}

	if cpu.User != -1 {
		sender.Rate("docker.cpu.user", cpu.User, "", tags)
	}

	if cpu.UsageTotal != -1 {
		sender.Rate("docker.cpu.usage", cpu.UsageTotal, "", tags)
	}

	if cpu.Shares != 0 {
		sender.Gauge("docker.cpu.shares", cpu.Shares, "", tags)
	}

	sender.Rate("docker.cpu.throttled", float64(cpu.NrThrottled), "", tags)
	sender.Rate("docker.cpu.throttled.time", cpu.ThrottledTime, "", tags)
	if cpu.ThreadCount != 0 {
		sender.Gauge("docker.thread.count", float64(cpu.ThreadCount), "", tags)
	}

	// limits.CPULimit is a percentage (i.e. 100.0%, not 1.0)
	timeDiff := cpu.Timestsamp.Unix() - startTime
	if limits.CPULimit > 0 && timeDiff > 0 {
		availableCPUTimeHz := 100 * float64(timeDiff) // Converted to Hz to be consistent with UsageTotal
		sender.Rate("docker.cpu.limit", limits.CPULimit/100*availableCPUTimeHz, "", tags)
	}
}

func (d *DockerCheck) reportIOMetrics(io *cmetrics.ContainerIOStats, tags []string, sender aggregator.Sender) {
	if io == nil {
		return
	}

	reportDeviceStat := func(metricName string, deviceMap map[string]uint64, fallbackValue uint64) {
		if len(deviceMap) > 0 {
			for dev, value := range deviceMap {
				sender.Rate(metricName, float64(value), "", append(tags, "device:"+dev, "device_name:"+dev))
			}
		} else {
			sender.Rate(metricName, float64(fallbackValue), "", tags)
		}
	}

	// Throughput
	reportDeviceStat("docker.io.read_bytes", io.DeviceReadBytes, io.ReadBytes)
	reportDeviceStat("docker.io.write_bytes", io.DeviceWriteBytes, io.WriteBytes)

	// IOPS
	reportDeviceStat("docker.io.read_operations", io.DeviceReadOperations, io.ReadOperations)
	reportDeviceStat("docker.io.write_operations", io.DeviceWriteOperations, io.WriteOperations)

	// Collect open file descriptor counts
	sender.Gauge("docker.container.open_fds", float64(io.OpenFiles), "", tags)
}

// Configure parses the check configuration and init the check
func (d *DockerCheck) Configure(config, initConfig integration.Data, source string) error {
	err := d.CommonConfigure(config, source)
	if err != nil {
		return err
	}

	d.instance.Parse(config) //nolint:errcheck

	if len(d.instance.FilteredEventType) == 0 {
		d.instance.FilteredEventType = []string{"top", "exec_create", "exec_start", "exec_die"}
	}

	// Use the same hostname as the agent so that host tags (like `availability-zone:us-east-1b`)
	// are attached to Docker events from this host. The hostname from the docker api may be
	// different than the agent hostname depending on the environment (like EC2 or GCE).
	d.dockerHostname, err = util.GetHostname(context.TODO())
	if err != nil {
		log.Warnf("Can't get hostname from docker, events will not have it: %s", err)
	}
	return nil
}

// DockerFactory is exported for integration testing
func DockerFactory() check.Check {
	return &DockerCheck{
		CheckBase: core.NewCheckBase(dockerCheckName),
		instance:  &DockerConfig{},
	}
}

func init() {
	core.RegisterCheck("docker", DockerFactory)
}
