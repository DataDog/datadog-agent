// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package cri

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	criTypes "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type criCustomMetricsExtension struct {
	sender            generic.SenderFunc
	aggSender         aggregator.Sender
	criGetter         func() (cri.CRIClient, error)
	criContainerStats map[string]*criTypes.ContainerStats
}

func (cext *criCustomMetricsExtension) PreProcess(sender generic.SenderFunc, aggSender aggregator.Sender) {
	cext.sender = sender
	cext.aggSender = aggSender

	client, err := cext.criGetter()
	if err != nil {
		log.Infof("Unable to reach CRI socket, err: %v", err)
		return
	}

	cext.criContainerStats, err = client.ListContainerStats()
	if err != nil {
		log.Infof("Unable to get CRI stats, err: %v", err)
	}
}

func (cext *criCustomMetricsExtension) Process(tags []string, container *workloadmeta.Container, collector metrics.Collector, cacheValidity time.Duration) {
	if cext.criContainerStats == nil {
		return
	}

	criStats, found := cext.criContainerStats[container.ID]
	if !found {
		log.Debugf("Container id '%s' not found in CRI stats, metrics will be missing", container.ID)
		return
	}

	cext.sender(cext.aggSender.Gauge, "cri.disk.used", pointer.Ptr(float64(criStats.GetWritableLayer().GetUsedBytes().GetValue())), tags)
	cext.sender(cext.aggSender.Gauge, "cri.disk.inodes", pointer.Ptr(float64(criStats.GetWritableLayer().GetInodesUsed().GetValue())), tags)
}

// PostProcess is called once during each check run, after all calls to `Process`
func (cext *criCustomMetricsExtension) PostProcess() {
	cext.criContainerStats = nil
}
