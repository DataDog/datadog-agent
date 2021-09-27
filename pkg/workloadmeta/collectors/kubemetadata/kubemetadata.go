// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver,kubelet

package kubemetadata

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID = "kube_metadata"
)

type collector struct {
	store *workloadmeta.Store
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
	})
}

func (c *collector) Start(_ context.Context, store *workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.Kubernetes) {
		return errors.New("the Agent is not running in Kubernetes")
	}

	c.store = store

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	events := []workloadmeta.Event{}

	c.store.Notify(events)

	return nil
}
