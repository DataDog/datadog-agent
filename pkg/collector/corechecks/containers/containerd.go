// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build containerd

package containers

import (
	"fmt"
	"strings"
	"time"

	"github.com/containerd/cgroups"
	containerdTypes "github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	ddContainers "github.com/DataDog/datadog-agent/pkg/util/containers"
	cgroup "github.com/DataDog/datadog-agent/pkg/util/containers/providers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	containerdCheckName = "containerd"
)

// ContainerCheck grabs containerd metrics and events
type ContainerdCheck struct {
	core.CheckBase
	instance *ContainerdConfig
	sub      *subscriber
	filters  *ddContainers.Filter
}

// ContainerdConfig contains the custom options and configurations set by the user.
type ContainerdConfig struct {
	ContainerdFilters []string `yaml:"filters"`
	CollectEvents     bool     `yaml:"collect_events"`
}

func init() {
	corechecks.RegisterCheck(containerdCheckName, ContainerdFactory)
}

// ContainerdFactory is used to create register the check and initialize it.
func ContainerdFactory() check.Check {
	return &ContainerdCheck{
		CheckBase: corechecks.NewCheckBase(containerdCheckName),
		instance:  &ContainerdConfig{},
		sub:       &subscriber{},
	}
}

// Parse is used to get the configuration set by the user
func (co *ContainerdConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, co); err != nil {
		return err
	}
	return nil
}

// Configure parses the check configuration and init the check
func (c *ContainerdCheck) Configure(config, initConfig integration.Data, source string) error {
	var err error
	if err = c.CommonConfigure(config, source); err != nil {
		return err
	}

	if err = c.instance.Parse(config); err != nil {
		return err
	}
	c.sub.Filters = c.instance.ContainerdFilters
	// GetSharedFilter should not return a nil instance of *Filter if there is an error during its setup.
	fil, err := ddContainers.GetSharedFilter()
	if err != nil {
		return err
	}
	c.filters = fil

	return nil
}

// Run executes the check
func (c *ContainerdCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}
	defer sender.Commit()

	// As we do not rely on a singleton, we ensure connectivity every check run.
	cu, errHealth := cutil.GetContainerdUtil()
	if errHealth != nil {
		sender.ServiceCheck("containerd.health", metrics.ServiceCheckCritical, "", nil, fmt.Sprintf("Connectivity error %v", errHealth))
		log.Infof("Error ensuring connectivity with Containerd daemon %v", errHealth)
		return errHealth
	}
	sender.ServiceCheck("containerd.health", metrics.ServiceCheckOK, "", nil, "")
	ns := cu.Namespace()

	if c.instance.CollectEvents {
		if c.sub == nil {
			c.sub = CreateEventSubscriber("ContainerdCheck", ns, c.instance.ContainerdFilters)
		}

		if !c.sub.IsRunning() {
			// Keep track of the health of the Containerd socket
			c.sub.CheckEvents(cu)
		}
		events := c.sub.Flush(time.Now().Unix())
		// Process events
		computeEvents(events, sender, c.filters)
	}

	computeMetrics(sender, cu, c.filters)
	return nil
}

// compute events converts Containerd events into Datadog events
func computeEvents(events []containerdEvent, sender aggregator.Sender, fil *ddContainers.Filter) {
	for _, e := range events {
		split := strings.Split(e.Topic, "/")
		if len(split) != 3 {
			// sanity checking the event, to avoid submitting
			log.Debugf("Event topic %s does not have the expected format", e.Topic)
			continue
		}
		if split[1] == "images" {
			if fil.IsExcluded("", e.ID) {
				continue
			}
		}
		output := metrics.Event{
			Priority:       metrics.EventPriorityNormal,
			SourceTypeName: containerdCheckName,
			EventType:      containerdCheckName,
			AggregationKey: fmt.Sprintf("containerd:%s", e.Topic),
		}
		output.Text = e.Message
		if len(e.Extra) > 0 {
			for k, v := range e.Extra {
				output.Tags = append(output.Tags, fmt.Sprintf("%s:%s", k, v))
			}
		}
		output.Ts = e.Timestamp.Unix()
		output.Title = fmt.Sprintf("Event on %s from Containerd", split[1])
		if split[1] == "containers" || split[1] == "tasks" {
			// For task events, we use the container ID in order to query the Tagger's API
			tags, err := tagger.Tag(ddContainers.ContainerEntityPrefix+e.ID, collectors.HighCardinality)
			if err != nil {
				// If there is an error retrieving tags from the Tagger, we can still submit the event as is.
				log.Errorf("Could not retrieve tags for the container %s: %v", e.ID, err)
			}
			output.Tags = append(output.Tags, tags...)
		}
		sender.Event(output)
	}
}

func computeMetrics(sender aggregator.Sender, cu cutil.ContainerdItf, fil *ddContainers.Filter) {
	containers, err := cu.Containers()
	if err != nil {
		log.Errorf(err.Error())
		return
	}

	for _, ctn := range containers {
		info, err := cu.Info(ctn)
		if err != nil {
			log.Errorf("Could not retrieve the metadata of the container: %s", ctn.ID()[:12])
			continue
		}
		if isExcluded(info, fil) {
			continue
		}

		tags, err := collectTags(info)
		if err != nil {
			log.Errorf("Could not collect tags for container %s: %s", ctn.ID()[:12], err)
		}
		// Tagger tags
		taggerTags, err := tagger.Tag(ddContainers.ContainerEntityPrefix+ctn.ID(), collectors.HighCardinality)
		if err != nil {
			log.Errorf(err.Error())
			continue
		}
		tags = append(tags, taggerTags...)

		metricTask, errTask := cu.TaskMetrics(ctn)
		if errTask != nil {
			log.Tracef("Could not retrieve metrics from task %s: %s", ctn.ID()[:12], errTask.Error())
			continue
		}

		metrics, err := convertTasktoMetrics(metricTask)
		if err != nil {
			log.Errorf("Could not process the metrics from %s: %v", ctn.ID(), err.Error())
			continue
		}

		currentTime := time.Now()
		computeUptime(sender, info, currentTime, tags)
		computeMem(sender, metrics.Memory, tags)

		if metrics.CPU.Throttling != nil && metrics.CPU.Usage != nil {
			computeCPU(sender, metrics.CPU, tags)
		}

		if metrics.Blkio.Size() > 0 {
			computeBlkio(sender, metrics.Blkio, tags)
		}

		if len(metrics.Hugetlb) > 0 {
			computeHugetlb(sender, metrics.Hugetlb, tags)
		}

		size, err := cu.ImageSize(ctn)
		if err != nil {
			log.Errorf("Could not retrieve the size of the image of %s: %v", ctn.ID(), err.Error())
			continue
		}
		sender.Gauge("containerd.image.size", float64(size), "", tags)

		// Collect open file descriptor counts
		processes, err := cu.TaskPids(ctn)
		if err != nil {
			log.Tracef("Could not retrieve pids from task %s: %s", ctn.ID()[:12], errTask.Error())
			continue
		}
		fileDescCount := 0
		for _, p := range processes {
			pid := p.Pid
			fdCount, err := cgroup.GetFileDescriptorLen(int(pid))
			if err != nil {
				log.Warnf("Failed to get file desc length for pid %d, container %s: %s", pid, ctn.ID()[:12], err)
				continue
			}
			fileDescCount += fdCount
		}
		sender.Gauge("containerd.proc.open_fds", float64(fileDescCount), "", tags)
	}
}

func isExcluded(ctn containers.Container, fil *ddContainers.Filter) bool {
	// The container name is not available in Containerd, we only rely on image name based exclusion
	return fil.IsExcluded("", ctn.Image)
}

func convertTasktoMetrics(metricTask *containerdTypes.Metric) (*cgroups.Metrics, error) {
	metricAny, err := typeurl.UnmarshalAny(&types.Any{
		TypeUrl: metricTask.Data.TypeUrl,
		Value:   metricTask.Data.Value,
	})
	if err != nil {
		log.Errorf(err.Error())
		return nil, err
	}
	return metricAny.(*cgroups.Metrics), nil
}

// TODO when creating a dedicated collector for the tagger, unify the local tagging logic and the Tagger.
func collectTags(ctn containers.Container) ([]string, error) {
	tags := []string{}

	// Container image
	imageName := fmt.Sprintf("image:%s", ctn.Image)
	tags = append(tags, imageName)

	// Container labels
	for k, v := range ctn.Labels {
		tag := fmt.Sprintf("%s:%s", k, v)
		tags = append(tags, tag)
	}
	runt := fmt.Sprintf("runtime:%s", ctn.Runtime.Name)
	tags = append(tags, runt)

	return tags, nil
}

func computeHugetlb(sender aggregator.Sender, huge []*cgroups.HugetlbStat, tags []string) {
	for _, h := range huge {
		sender.Gauge("containerd.hugetlb.max", float64(h.Max), "", tags)
		sender.Gauge("containerd.hugetlb.failcount", float64(h.Failcnt), "", tags)
		sender.Gauge("containerd.hugetlb.usage", float64(h.Usage), "", tags)
	}
}

func computeUptime(sender aggregator.Sender, ctn containers.Container, currentTime time.Time, tags []string) {
	uptime := currentTime.Sub(ctn.CreatedAt).Seconds()
	if uptime > 0 {
		sender.Gauge("containerd.uptime", uptime, "", tags)
	}
}

func computeMem(sender aggregator.Sender, mem *cgroups.MemoryStat, tags []string) {
	if mem == nil {
		return
	}

	memList := map[string]*cgroups.MemoryEntry{
		"containerd.mem.current":    mem.Usage,
		"containerd.mem.kernel_tcp": mem.KernelTCP,
		"containerd.mem.kernel":     mem.Kernel,
		"containerd.mem.swap":       mem.Swap,
	}
	for metricName, memStat := range memList {
		parseAndSubmitMem(metricName, sender, memStat, tags)
	}
	sender.Gauge("containerd.mem.cache", float64(mem.Cache), "", tags)
	sender.Gauge("containerd.mem.rss", float64(mem.RSS), "", tags)
	sender.Gauge("containerd.mem.rsshuge", float64(mem.RSSHuge), "", tags)
	sender.Gauge("containerd.mem.dirty", float64(mem.Dirty), "", tags)
}

func parseAndSubmitMem(metricName string, sender aggregator.Sender, stat *cgroups.MemoryEntry, tags []string) {
	if stat == nil || stat.Size() == 0 {
		return
	}
	sender.Gauge(fmt.Sprintf("%s.usage", metricName), float64(stat.Usage), "", tags)
	sender.Gauge(fmt.Sprintf("%s.failcnt", metricName), float64(stat.Failcnt), "", tags)
	sender.Gauge(fmt.Sprintf("%s.limit", metricName), float64(stat.Limit), "", tags)
	sender.Gauge(fmt.Sprintf("%s.max", metricName), float64(stat.Max), "", tags)

}

func computeCPU(sender aggregator.Sender, cpu *cgroups.CPUStat, tags []string) {
	sender.Rate("containerd.cpu.system", float64(cpu.Usage.Kernel), "", tags)
	sender.Rate("containerd.cpu.total", float64(cpu.Usage.Total), "", tags)
	sender.Rate("containerd.cpu.user", float64(cpu.Usage.User), "", tags)
	sender.Rate("containerd.cpu.throttled.periods", float64(cpu.Throttling.ThrottledPeriods), "", tags)

}

func computeBlkio(sender aggregator.Sender, blkio *cgroups.BlkIOStat, tags []string) {
	blkioList := map[string][]*cgroups.BlkIOEntry{
		"containerd.blkio.merged_recursive":        blkio.IoMergedRecursive,
		"containerd.blkio.queued_recursive":        blkio.IoQueuedRecursive,
		"containerd.blkio.sectors_recursive":       blkio.SectorsRecursive,
		"containerd.blkio.service_recursive_bytes": blkio.IoServiceBytesRecursive,
		"containerd.blkio.time_recursive":          blkio.IoTimeRecursive,
		"containerd.blkio.serviced_recursive":      blkio.IoServicedRecursive,
		"containerd.blkio.wait_time_recursive":     blkio.IoWaitTimeRecursive,
		"containerd.blkio.service_time_recursive":  blkio.IoServiceTimeRecursive,
	}
	for metricName, ioStats := range blkioList {
		parseAndSubmitBlkio(metricName, sender, ioStats, tags)
	}
}

func parseAndSubmitBlkio(metricName string, sender aggregator.Sender, list []*cgroups.BlkIOEntry, tags []string) {
	for _, m := range list {
		if m.Size() == 0 {
			continue
		}

		tags = append(tags, fmt.Sprintf("device:%s", m.Device))
		if m.Op != "" {
			tags = append(tags, fmt.Sprintf("operation:%s", m.Op))
		}

		sender.Rate(metricName, float64(m.Value), "", tags)
	}
}
