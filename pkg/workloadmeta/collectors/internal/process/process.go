// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package process

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID   = "process"
	componentName = "workloadmeta-process"
)

type collector struct {
	store workloadmeta.Store
	probe procutil.Probe
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{
			probe: procutil.NewProcessProbe(),
		}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	c.store = store
	return nil
}

func (c *collector) Pull(ctx context.Context) error {

	pmap, err := c.probe.ProcessesByPID(time.Now(), false)
	if err != nil {
		return err
	}

	var res []workloadmeta.CollectorEvent
	for pID, process := range pmap {

		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindProcessMetadata,
			ID:   collectorID,
		}
		entityMeta := workloadmeta.EntityMeta{
			Name:        componentName,
			Namespace:   "",
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		}
		p := workloadmeta.ProcessMetadata{
			EntityID:   entityID,
			EntityMeta: entityMeta,
			PID:        pID,
			Command:    strings.Join(process.Cmdline, ""),
			Language:   "",
		}
		res = append(res, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeSet,
			Source: componentName,
			Entity: &p,
		})
	}
	c.store.Notify(res)

	return nil
}
