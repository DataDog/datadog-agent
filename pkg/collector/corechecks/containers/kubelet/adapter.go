// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
)

// metricsNameMapping maps generic container.* metric names to kubernetes.* equivalents.
// In a typical kubelet environment, the containerd collector (priority 1) or system
// collector (priority 0) reads cgroup data directly, providing the same underlying
// data that cadvisor scrapes from /metrics/cadvisor. This mapping renames those
// metrics to the kubernetes.* namespace.
//
// Metrics mapped to "" are suppressed — either because no kubernetes.* equivalent
// exists, or because the data is nil from the active collector.
var metricsNameMapping = map[string]string{
	// CPU metrics — from cgroup cpuacct/cpu.stat, same data as cadvisor
	"container.cpu.usage":             "kubernetes.cpu.usage.total",
	"container.cpu.user":              "kubernetes.cpu.user.total",
	"container.cpu.system":            "kubernetes.cpu.system.total",
	"container.cpu.throttled":         "kubernetes.cpu.cfs.throttled.seconds",
	"container.cpu.throttled.periods": "kubernetes.cpu.cfs.throttled.periods",
	"container.cpu.partial_stall":     "", // PSI data, no cadvisor equivalent
	"container.cpu.limit":             "", // Percentage format, not emitted by cadvisor

	// Memory metrics — from cgroup memory stats, same data as cadvisor
	"container.memory.usage":             "kubernetes.memory.usage",
	"container.memory.rss":               "kubernetes.memory.rss",
	"container.memory.cache":             "kubernetes.memory.cache",
	"container.memory.swap":              "kubernetes.memory.swap",
	"container.memory.limit":             "kubernetes.memory.limits",
	"container.memory.working_set":       "kubernetes.memory.working_set",
	"container.memory.kernel":            "", // No cadvisor equivalent
	"container.memory.soft_limit":        "", // No cadvisor equivalent
	"container.memory.oom_events":        "", // No cadvisor equivalent
	"container.memory.commit":            "", // Windows only
	"container.memory.commit.peak":       "", // Windows only
	"container.memory.usage.peak":        "", // No cadvisor equivalent
	"container.memory.partial_stall":     "", // PSI data, no cadvisor equivalent
	"container.memory.page_faults":       "", // No cadvisor equivalent (cadvisor doesn't emit this)
	"container.memory.major_page_faults": "", // No cadvisor equivalent

	// IO metrics — from cgroup blkio/io stats, same data as cadvisor
	"container.io.read":             "kubernetes.io.read_bytes",
	"container.io.write":            "kubernetes.io.write_bytes",
	"container.io.read.operations":  "", // No cadvisor equivalent
	"container.io.write.operations": "", // No cadvisor equivalent
	"container.io.partial_stall":    "", // PSI data, no cadvisor equivalent

	// Network metrics — from network interface stats
	"container.net.sent":         "kubernetes.network.tx_bytes",
	"container.net.rcvd":         "kubernetes.network.rx_bytes",
	"container.net.sent.packets": "", // No cadvisor equivalent (cadvisor has errors/drops, not packets)
	"container.net.rcvd.packets": "", // No cadvisor equivalent

	// Passthrough for network extension (already renamed)
	"kubernetes.network.tx_bytes": "kubernetes.network.tx_bytes",
	"kubernetes.network.rx_bytes": "kubernetes.network.rx_bytes",

	// No kubernetes.* equivalent
	"container.uptime":           "",
	"container.pid.thread_count": "",
	"container.pid.thread_limit": "",
	"container.pid.open_files":   "",
	"container.restarts":         "",
}

// kubeletMetricsAdapter implements the generic.MetricsAdapter interface for the kubelet check.
// It renames container.* metrics to kubernetes.* equivalents and suppresses metrics
// that are not available from the kubelet stats/summary collector or are handled
// by other kubelet providers (cadvisor, summary, etc.).
type kubeletMetricsAdapter struct {
	config *common.KubeletConfig
	store  workloadmeta.Component
}

// AdaptTags adds config tags and the kube_static_cpus tag to match the tagging
// behavior of the existing cadvisor provider.
func (a *kubeletMetricsAdapter) AdaptTags(tags []string, c *workloadmeta.Container) []string {
	if len(a.config.Tags) > 0 {
		tags = utils.ConcatenateTags(tags, a.config.Tags)
	}

	pod, _ := a.store.GetKubernetesPodForContainer(c.ID)
	if pod != nil {
		containerID := types.NewEntityID(types.ContainerID, c.ID)
		tags = common.AppendKubeStaticCPUsTag(a.store, pod.QOSClass, containerID, tags)
	}

	return tags
}

// AdaptMetrics renames container.* metrics to kubernetes.* equivalents.
// Metrics with no kubelet equivalent are suppressed by mapping to "".
func (a *kubeletMetricsAdapter) AdaptMetrics(metricName string, value float64) (string, float64) {
	return metricsNameMapping[metricName], value
}
