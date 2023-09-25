// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package summary contains all metrics from the /stats/summary endpoint
package summary

import (
	"context"
	"regexp"
	"runtime"

	kubeletv1alpha1 "k8s.io/kubelet/pkg/apis/stats/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	txBytesMetricName = "network.tx_bytes"
	rxBytesMetricName = "network.rx_bytes"
)

// Provider provides the data collected from the `/stats/summary` Kubelet endpoint
type Provider struct {
	filter                *containers.Filter
	config                *common.KubeletConfig
	store                 workloadmeta.Store
	defaultRateFilterList []*regexp.Regexp
}

// NewProvider is created by filter, config and workloadmeta
func NewProvider(filter *containers.Filter,
	config *common.KubeletConfig,
	store workloadmeta.Store) *Provider {
	defaultRateFilterList := []*regexp.Regexp{
		regexp.MustCompile("diskio[.]io_service_bytes[.]stats[.]total"),
		regexp.MustCompile("network[.].._bytes"),
		regexp.MustCompile("cpu[.].*[.]total"),
	} //default enabled_rates

	return &Provider{
		filter:                filter,
		config:                config,
		store:                 store,
		defaultRateFilterList: defaultRateFilterList,
	}
}

// Provide processes metrics and reports
func (p *Provider) Provide(kc kubelet.KubeUtilInterface, sender sender.Sender) error {
	statsSummary, err := kc.GetLocalStatsSummary(context.TODO())
	if err != nil || statsSummary == nil {
		return err
	}

	useStatsAsSource := false
	if p.config.UseStatsSummaryAsSource == nil {
		if runtime.GOOS == "windows" {
			useStatsAsSource = true
		}
	} else {
		useStatsAsSource = *p.config.UseStatsSummaryAsSource
	}

	rateFilterList := p.defaultRateFilterList
	if len(p.config.EnabledRates) > 0 {
		rateFilterList = []*regexp.Regexp{}
		for i := range p.config.EnabledRates {
			pat := &p.config.EnabledRates[i]
			r, err := regexp.Compile(*pat)
			if err == nil {
				rateFilterList = append(rateFilterList, r)
			} else {
				log.Warnf("invalid regex found in enabled_rates '%s': %s", *pat, err)
			}
		}
	}

	p.processSystemStats(sender, statsSummary)
	for i := range statsSummary.Pods {
		podStats := &statsSummary.Pods[i]
		if len(podStats.Containers) == 0 {

			continue
		}
		if len(podStats.PodRef.Namespace) == 0 || len(podStats.PodRef.Name) == 0 || len(podStats.PodRef.UID) == 0 {
			log.Warnf("Got incomplete pod data from '/stats/summary', podNamespace = %s, podName = %s, podUid = %s",
				podStats.PodRef.Namespace, podStats.PodRef.Name, podStats.PodRef.UID)
			continue
		}
		//  Query to check whether a Kubernetes namespace should be excluded.
		if p.filter.IsExcluded(nil, "", "", podStats.PodRef.Namespace) {
			continue
		}

		podData, err := p.store.GetKubernetesPod(podStats.PodRef.UID) //from workloadmeta store
		if err != nil || podData == nil {
			log.Warnf("Couldn't get pod data from workloadmeta store, error = %v ", err)
			continue
		}
		if podData.Phase == "Running" {
			p.processPodStats(sender, podStats, useStatsAsSource, rateFilterList)
		}
		p.processContainerStats(sender, podStats, podData, useStatsAsSource)
	}
	return nil
}

func (p *Provider) processSystemStats(sender sender.Sender,
	statsSummary *kubeletv1alpha1.Summary) {
	//System metrics
	reportFsMetric(sender, statsSummary.Node.Fs, "node.", p.config.Tags)
	if statsSummary.Node.Runtime != nil {
		reportFsMetric(sender, statsSummary.Node.Runtime.ImageFs, "node.image.", p.config.Tags)
	}

	for _, ctr := range statsSummary.Node.SystemContainers {
		if ctr.Name == "runtime" || ctr.Name == "kubelet" {
			if ctr.Memory != nil {
				reportMetric(sender.Gauge, ctr.Name+".memory.rss",
					ctr.Memory.RSSBytes, p.config.Tags)
				reportMetric(sender.Gauge, ctr.Name+".memory.usage",
					ctr.Memory.UsageBytes, p.config.Tags)
			}
			if ctr.CPU != nil {
				reportMetric(sender.Gauge, ctr.Name+".cpu.usage",
					ctr.CPU.UsageNanoCores, p.config.Tags)
			}
		}
	}
}

func (p *Provider) processPodStats(sender sender.Sender,
	podStats *kubeletv1alpha1.PodStats,
	useStatsAsSource bool,
	rateFilterList []*regexp.Regexp) {
	if podStats == nil {
		return
	}

	podTags, _ := tagger.Tag(kubelet.PodUIDToTaggerEntityName(podStats.PodRef.UID),
		collectors.OrchestratorCardinality)

	if len(podTags) == 0 {
		log.Infof("Tags not found for pod: %s/%s - no metrics will be sent",
			podStats.PodRef.Namespace, podStats.PodRef.Name)
		return
	}

	podTags = utils.ConcatenateTags(podTags, p.config.Tags)
	ephemeralStorage := podStats.EphemeralStorage
	if ephemeralStorage != nil {
		reportMetric(sender.Gauge, "ephemeral_storage.usage",
			ephemeralStorage.UsedBytes, podTags)
	}
	if !useStatsAsSource {
		return
	}
	podNetwork := podStats.Network
	if podNetwork == nil {
		return
	}
	var rxBytes, txBytes *uint64
	rxBytes = podNetwork.InterfaceStats.RxBytes
	txBytes = podNetwork.InterfaceStats.TxBytes

	for _, r := range rateFilterList {
		if txBytes != nil && r.MatchString(txBytesMetricName) {
			reportMetric(sender.Rate, txBytesMetricName, txBytes, podTags)
		}
		if rxBytes != nil && r.MatchString(rxBytesMetricName) {
			reportMetric(sender.Rate, rxBytesMetricName, rxBytes, podTags)
		}
	}
}

func (p *Provider) processContainerStats(sender sender.Sender,
	podStats *kubeletv1alpha1.PodStats,
	podData *workloadmeta.KubernetesPod,
	useStatsAsSource bool) {
	if podStats == nil ||
		len(podStats.Containers) == 0 ||
		podData == nil ||
		!useStatsAsSource {
		return
	}
	containerData := make(map[string]*workloadmeta.OrchestratorContainer)
	for i := range podData.Containers {
		containerData[podData.Containers[i].Name] = &podData.Containers[i]
	}
	for idx := range podStats.Containers {
		containerStats := &podStats.Containers[idx]
		containerName := containerStats.Name
		if len(containerName) == 0 {
			log.Warnf("Kubelet reported stats without container name for pod: %s/%s",
				podStats.PodRef.Namespace, podStats.PodRef.Name)
			continue
		}
		ctr, found := containerData[containerName]
		if !found || ctr == nil && ctr.ID == "" {
			log.Debugf(
				"Container id not found from /pods for container: %s/%s/%s - no metrics will be sent",
				podStats.PodRef.Namespace, podStats.PodRef.Name, containerName)
			continue
		}
		if p.filter.IsExcluded(nil,
			containerName,
			ctr.Image.Name,
			podStats.PodRef.Namespace) {
			continue
		}
		tags, err := tagger.Tag(containers.BuildTaggerEntityName(ctr.ID), collectors.HighCardinality)
		if err != nil || len(tags) == 0 {
			log.Debugf("Tags not found for container: %s/%s/%s:%s - no metrics will be sent",
				podStats.PodRef.Namespace, podStats.PodRef.Name, containerName, ctr.ID)
			continue
		}
		tags = utils.ConcatenateTags(tags, p.config.Tags)
		//collecting metric
		if containerStats.CPU != nil {
			reportMetric(sender.Rate, "cpu.usage.total", containerStats.CPU.UsageCoreNanoSeconds, tags)
		}
		if containerStats.Memory != nil {
			reportMetric(sender.Rate, "memory.working_set", containerStats.Memory.WorkingSetBytes, tags)
			reportMetric(sender.Rate, "memory.usage", containerStats.Memory.UsageBytes, tags)
		}
		reportFsMetric(sender, containerStats.Rootfs, "", tags)
	}
}

func reportMetric[T float64 | uint64](senderFunc func(string, float64, string, []string),
	metricName string, value *T, tags []string) {
	if value == nil {
		return
	}
	if metricName != "" {
		senderFunc(common.KubeletMetricsPrefix+metricName, float64(*value), "", tags)
	}
}

func reportFsMetric(sender sender.Sender,
	fsStats *kubeletv1alpha1.FsStats,
	metricPrefix string,
	tags []string) {
	if fsStats == nil {
		return
	}
	reportMetric(sender.Gauge,
		metricPrefix+"filesystem.usage",
		fsStats.UsedBytes,
		tags)
	if fsStats.UsedBytes != nil && fsStats.CapacityBytes != nil && *fsStats.CapacityBytes != 0 {
		usagePct := float64(*fsStats.UsedBytes) / float64(*fsStats.CapacityBytes)
		reportMetric(sender.Gauge,
			metricPrefix+"filesystem.usage_pct",
			&usagePct,
			tags)
	}
}
