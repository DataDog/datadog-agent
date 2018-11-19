// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build containerd

package containers

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	util "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/containerd/cgroups"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/typeurl"
	"github.com/gogo/protobuf/types"
	"gopkg.in/yaml.v2"
)

const (
	containerdCheckName = "containerd"
)

// ContainerCheck grabs containerd metrics and events
type ContainerdCheck struct {
	core.CheckBase
	instance *ContainerdConfig
	cu       util.ContainerdItf
}

// ContainerdConfig contains the custom options and configurations set by the user.
type ContainerdConfig struct {
	Tags []string
}

func init() {
	corechecks.RegisterCheck(containerdCheckName, ContainerdFactory)
}

// ContainerdFactory is used to create register the check and initialize it.
func ContainerdFactory() check.Check {
	return &ContainerdCheck{
		CheckBase: corechecks.NewCheckBase(containerdCheckName),
		instance:  &ContainerdConfig{},
		cu:        util.InstanciateContainerdUtil(),
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
func (c *ContainerdCheck) Configure(config, initConfig integration.Data) error {
	err := c.CommonConfigure(config)
	if err != nil {
		return err
	}
	return c.instance.Parse(config)
}

// Run executes the check
func (c *ContainerdCheck) Run() error {
	// As we do not rely on a singleton, we ensure connectivity every check run.
	errHealth := c.cu.EnsureServing(context.Background())
	if errHealth != nil {
		log.Infof("Error ensuring connectivity with containerd daemon %v", errHealth)
		return errHealth
	}

	sender, err := aggregator.GetSender(c.ID())
	defer sender.Commit()
	if err != nil {
		return err
	}

	nsList, err := c.cu.GetNamespaces(context.Background())
	if err != nil {
		return err
	}

	for _, n := range nsList {
		nk := namespaces.WithNamespace(context.Background(), n)
		computeMetrics(sender, nk, c.cu, c.instance.Tags)
	}
	return nil
}

func computeMetrics(sender aggregator.Sender, nk context.Context, cu util.ContainerdItf, userTags []string) {
	containers, err := cu.Containers(nk)
	if err != nil {
		log.Errorf(err.Error())
		return
	}

	for _, ctn := range containers {
		t, err := ctn.Task(nk, nil)
		if err != nil {
			log.Errorf(err.Error())
			continue
		}

		tags, err := collectTags(ctn, nk)
		if err != nil {
			log.Errorf("Could not collect tags for container %s: %s", ctn.ID()[:12], err)
		}
		tags = append(tags, userTags...)
		// Tagger tags
		taggerTags, err := tagger.Tag(ctn.ID(), false)
		if err != nil {
			log.Errorf(err.Error())
			continue
		}
		tags = append(tags, taggerTags...)

		metrics, err := convertTasktoMetrics(t, nk)
		if err != nil {
			log.Errorf("Could not process the metrics from %s: %v", ctn.ID(), err.Error())
			continue
		}

		err = computeExtra(sender, ctn, nk, tags)
		if err != nil {
			log.Errorf("Could not process metadata related metrics for %s: %v", ctn.ID()[:12], err.Error())
		}

		err = computeMem(sender, metrics.Memory, tags)
		if err != nil {
			log.Errorf("Could not process memory related metrics for %s: %v", ctn.ID()[:12], err.Error())
		}

		err = computeCPU(sender, metrics.CPU, tags)
		if err != nil {
			log.Errorf("Could not process cpu related metrics for %s: %v", ctn.ID()[:12], err.Error())
		}

		err = computeBlkio(sender, metrics.Blkio, tags)
		if err != nil {
			log.Errorf("Could not process blkio related metrics for %s: %v", ctn.ID()[:12], err.Error())
		}

		err = computeHugetlb(sender, metrics.Hugetlb, tags)
		if err != nil {
			log.Errorf("Could not process hugetlb related metrics for %s: %v", ctn.ID()[:12], err.Error())
		}
	}
}

func convertTasktoMetrics(task containerd.Task, nsCtx context.Context) (*cgroups.Metrics, error) {
	metricTask, err := task.Metrics(nsCtx)
	if err != nil {
		return nil, err
	}

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

func computeExtra(sender aggregator.Sender, ctn containerd.Container, nsCtx context.Context, tags []string) error {
	img, err := ctn.Image(nsCtx)
	if err != nil {
		return err
	}
	size, err := img.Size(nsCtx)
	if err != nil {
		return err
	}
	sender.Gauge("containerd.image.size", float64(size), "", tags)
	return nil
}

func collectTags(ctn containerd.Container, nsCtx context.Context) ([]string, error) {
	tags := []string{}

	// Container image
	im, err := ctn.Image(nsCtx)
	if err != nil {
		return tags, err
	}
	imageName := fmt.Sprintf("image:%s", im.Name())
	tags = append(tags, imageName)

	// Container labels
	labels, err := ctn.Labels(nsCtx)
	if err != nil {
		return tags, err
	}
	for k, v := range labels {
		tag := fmt.Sprintf("%s:%s", k, v)
		tags = append(tags, tag)
	}

	// Container meta
	i, err := ctn.Info(nsCtx)
	if err != nil {
		return tags, err
	}
	runt := fmt.Sprintf("runtime:%s", i.Runtime.Name)
	tags = append(tags, runt)

	return tags, nil
}

func computeHugetlb(sender aggregator.Sender, huge []*cgroups.HugetlbStat, tags []string) error {
	if huge == nil {
		return fmt.Errorf("no hugetbl metrics available")
	}
	for _, h := range huge {
		sender.Gauge("containerd.hugetlb.max", float64(h.Max), "", tags)
		sender.Gauge("containerd.hugetlb.failcount", float64(h.Failcnt), "", tags)
		sender.Gauge("containerd.hugetlb.usage", float64(h.Usage), "", tags)
	}
	return nil
}

func computeMem(sender aggregator.Sender, mem *cgroups.MemoryStat, tags []string) error {
	if mem == nil {
		return fmt.Errorf("no mem metrics available")
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
	sender.Rate("containerd.mem.cache", float64(mem.Cache), "", tags)
	sender.Rate("containerd.mem.rss", float64(mem.RSS), "", tags)
	sender.Rate("containerd.mem.rsshuge", float64(mem.RSSHuge), "", tags)
	sender.Rate("containerd.mem.usage", float64(mem.Usage.Usage), "", tags)
	sender.Rate("containerd.mem.kernel.usage", float64(mem.Kernel.Usage), "", tags)
	sender.Rate("containerd.mem.dirty", float64(mem.Dirty), "", tags)

	return nil
}

func parseAndSubmitMem(metricName string, sender aggregator.Sender, stat *cgroups.MemoryEntry, tags []string) {
	if stat.Size() == 0 {
		return
	}
	sender.Gauge(fmt.Sprintf("%s.usage", metricName), float64(stat.Usage), "", tags)
	sender.Gauge(fmt.Sprintf("%s.failcnt", metricName), float64(stat.Failcnt), "", tags)
	sender.Gauge(fmt.Sprintf("%s.limit", metricName), float64(stat.Limit), "", tags)
	sender.Gauge(fmt.Sprintf("%s.max", metricName), float64(stat.Max), "", tags)

}

func computeCPU(sender aggregator.Sender, cpu *cgroups.CPUStat, tags []string) error {
	if cpu.Throttling == nil || cpu.Usage == nil {
		return fmt.Errorf("no cpu metrics available")
	}
	sender.Rate("containerd.cpu.system", float64(cpu.Usage.Kernel), "", tags)
	sender.Rate("containerd.cpu.total", float64(cpu.Usage.Total), "", tags)
	sender.Rate("containerd.cpu.user", float64(cpu.Usage.User), "", tags)
	sender.Rate("containerd.cpu.throttle.periods", float64(cpu.Throttling.Periods), "", tags)

	return nil
}

func computeBlkio(sender aggregator.Sender, blkio *cgroups.BlkIOStat, tags []string) error {
	if blkio.Size() == 0 {
		return fmt.Errorf("no blkio metrics available")
	}
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
	return nil
}

func parseAndSubmitBlkio(metricName string, sender aggregator.Sender, list []*cgroups.BlkIOEntry, tags []string) {
	for _, m := range list {
		if m.Size() == 0 {
			continue
		}
		blkiotags := []string{
			fmt.Sprintf("dev:%s", m.Device),
		}
		if m.Op != "" {
			blkiotags = append(blkiotags, fmt.Sprintf("operation:%s", m.Op))
		}

		tags = append(tags, blkiotags...)
		sender.Gauge(metricName, float64(m.Value), "", tags)
	}
}
