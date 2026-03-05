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

var metricsNameMapping = map[string]string{
	"container.cpu.usage":              "kubernetes.cpu.usage.total",
	"container.memory.usage":           "kubernetes.memory.usage",
	"container.memory.rss":             "kubernetes.memory.rss",
	"container.memory.working_set":     "kubernetes.memory.working_set",
	"container.net.sent":               "kubernetes.network.tx_bytes",
	"container.net.rcvd":               "kubernetes.network.rx_bytes",
	"kubernetes.network.tx_bytes":      "kubernetes.network.tx_bytes", // Passthrough for network extension
	"kubernetes.network.rx_bytes":      "kubernetes.network.rx_bytes", // Passthrough for network extension
	"container.uptime":                 "",
	"container.cpu.user":               "",
	"container.cpu.system":             "",
	"container.cpu.throttled":          "",
	"container.cpu.throttled.periods":  "",
	"container.cpu.partial_stall":      "",
	"container.cpu.limit":              "",
	"container.memory.kernel":          "",
	"container.memory.limit":           "",
	"container.memory.soft_limit":      "",
	"container.memory.cache":           "",
	"container.memory.swap":            "",
	"container.memory.oom_events":      "",
	"container.memory.commit":          "",
	"container.memory.commit.peak":     "",
	"container.memory.usage.peak":      "",
	"container.memory.partial_stall":   "",
	"container.memory.page_faults":     "",
	"container.memory.major_page_faults": "",
	"container.io.read":                "",
	"container.io.read.operations":     "",
	"container.io.write":               "",
	"container.io.write.operations":    "",
	"container.io.partial_stall":       "",
	"container.pid.thread_count":       "",
	"container.pid.thread_limit":       "",
	"container.pid.open_files":         "",
	"container.net.sent.packets":       "",
	"container.net.rcvd.packets":       "",
	"container.restarts":               "",
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
