// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package probe

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// Provider provides the metrics related to data collected from the `/metrics/probes` Kubelet endpoint
type Provider struct {
	filter *containers.Filter
	config *common.KubeletConfig
	store  workloadmeta.Store
}

func NewProvider(filter *containers.Filter, config *common.KubeletConfig) *Provider {
	return &Provider{
		filter: filter,
		config: config,
		store:  workloadmeta.GetGlobalStore(),
	}
}

func (p *Provider) Provide(kc kubelet.KubeUtilInterface, sender sender.Sender) error {
	// Collect raw data
	data, status, err := kc.QueryKubelet(context.TODO(), "/metrics/probes")
	if err != nil {
		log.Debugf("Unable to collect query probes endpoint: %s", err)
		return err
	}
	if status == 404 {
		return nil
	}

	metrics, err := prometheus.ParseMetrics(data)
	if err != nil {
		return err
	}

	// Report metrics
	for _, metric := range metrics {
		metricSuffix := ""

		probeType := metric.Metric["probe_type"]
		if probeType == "Liveness" {
			metricSuffix = "liveness_probe"
		} else if probeType == "Readiness" {
			metricSuffix = "readiness_probe"
		} else {
			log.Debugf("Unsupported probe type %s", probeType)
			continue
		}

		result := metric.Metric["result"]
		if result == "successful" {
			metricSuffix += ".success.total"
		} else if result == "failed" {
			metricSuffix += ".failure.total"
		} else if result == "unknown" {
			metricSuffix += ".unknown.total"
		} else {
			log.Debugf("Unsupported probe result %s", result)
			continue
		}

		podUid := string(metric.Metric["pod_uid"])
		containerName := string(metric.Metric["container"])
		pod, err := p.store.GetKubernetesPod(podUid)
		if err != nil {
			continue
		}

		var container *workloadmeta.OrchestratorContainer
		for _, c := range pod.Containers {
			if c.Name == containerName {
				container = &c
				break
			}
		}

		if container == nil {
			log.Debugf("container %s not found for pod with id %s", containerName, podUid)
			continue
		}

		if p.filter.IsExcluded(pod.EntityMeta.Annotations, container.Name, container.Image.Name, pod.Namespace) {
			continue
		}

		cId := containers.BuildTaggerEntityName(container.ID)

		tags, _ := tagger.Tag(cId, collectors.HighCardinality)
		if len(tags) == 0 {
			continue
		}
		tags = utils.ConcatenateTags(tags, p.config.Tags)

		sender.Gauge(common.KubeletMetricsPrefix+metricSuffix, float64(metric.Value), "", tags)
	}
	return nil
}
