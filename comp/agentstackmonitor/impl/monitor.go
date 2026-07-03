// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package agentstackmonitorimpl implements the agentstackmonitor component.
package agentstackmonitorimpl

import (
	"context"
	"sync"
	"time"

	agentstackmonitor "github.com/DataDog/datadog-agent/comp/agentstackmonitor/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	issuetemplates "github.com/DataDog/datadog-agent/comp/healthplatform/issues/agentstackmonitor"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	schedulerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	metrics "github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	configHealthPlatformKey = "health_platform.enabled"
	configClusterAgentKey   = "cluster_agent.enabled"
	defaultTickInterval     = 60 * time.Second
	defaultCacheValidity    = 60 * time.Second
	staleness               = 3 * defaultTickInterval
)

// Requires defines the dependencies for the agentstackmonitor component.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Log       log.Component
	Config    config.Component
	WMeta     workloadmeta.Component
	Telemetry telemetry.Component
	Scheduler schedulerdef.Component
	Store     storedef.Component
}

// Provides is the fx output.
type Provides struct {
	Comp agentstackmonitor.Component
}

type component struct {
	log       log.Component
	wmeta     workloadmeta.Component
	scheduler schedulerdef.Component
	store     storedef.Component
	gauges    *gauges
	provider  provider.Provider

	tickInterval  time.Duration
	cacheValidity time.Duration
	now           func() time.Time

	mu       sync.Mutex
	subjects map[stateKey]*subjectState
}

// NewComponent is the fx constructor.
func NewComponent(reqs Requires) Provides {
	c := &component{
		log:           reqs.Log,
		wmeta:         reqs.WMeta,
		scheduler:     reqs.Scheduler,
		store:         reqs.Store,
		gauges:        newGauges(reqs.Telemetry),
		tickInterval:  defaultTickInterval,
		cacheValidity: defaultCacheValidity,
		now:           time.Now,
		subjects:      make(map[stateKey]*subjectState),
	}

	switch {
	case !reqs.Config.GetBool(configHealthPlatformKey):
		c.log.Info("agentstackmonitor: health_platform.enabled=false, not running")
	case !reqs.Config.GetBool(configClusterAgentKey):
		c.log.Info("agentstackmonitor: cluster_agent.enabled=false, not running")
	case !env.IsFeaturePresent(env.Kubernetes):
		c.log.Info("agentstackmonitor: Kubernetes feature not detected, not running")
	default:
		reqs.Lifecycle.Append(compdef.Hook{
			OnStart: c.start,
			OnStop:  c.stop,
		})
	}
	return Provides{Comp: c}
}

func (c *component) start(_ context.Context) error {
	c.provider = metrics.GetProvider(option.New(c.wmeta))

	var initialIDs []string
	for _, name := range issuetemplates.AllIssueNames {
		initialIDs = append(initialIDs, c.store.GetActiveIssueIDsByIssueName(name)...)
	}

	if err := c.scheduler.Schedule(issuetemplates.Source, c.evaluateTick, c.tickInterval, initialIDs); err != nil {
		c.log.Warnf("agentstackmonitor: scheduler.Schedule failed: %v", err)
		return nil
	}
	c.log.Infof("agentstackmonitor: registered periodic health check (interval=%s)", c.tickInterval)
	return nil
}

func (c *component) stop(_ context.Context) error { return nil }

func (c *component) evaluateTick() ([]runnerdef.IssueReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	pods := c.wmeta.ListKubernetesPods()
	seen := make(map[stateKey]struct{})
	agg := newTickAggregate()
	var reports []runnerdef.IssueReport

	for _, pod := range pods {
		kind, ok := subjectKindFor(pod)
		if !ok {
			continue
		}
		// Fall back to the pod itself for bare / static pods with no owner.
		ctrl := controllerRef{Namespace: pod.Namespace, Kind: "Pod", Name: pod.Name}
		if len(pod.Owners) > 0 && pod.Owners[0].Name != "" {
			ctrl.Kind = pod.Owners[0].Kind
			ctrl.Name = pod.Owners[0].Name
		}

		for _, orch := range pod.GetAllContainers() {
			key := stateKey{podUID: string(pod.EntityID.ID), containerName: orch.Name}
			seen[key] = struct{}{}
			st := c.getOrCreateState(key, pod, kind, ctrl, orch.Name)
			st.lastSeenAt = c.now()

			c.refreshStats(st, orch.ID)
			c.applyContainerStatus(st, pod, orch.Name)
			st.observePod(pod)

			agg.addResource(st)
			agg.addStatus(st)
			agg.addPodStatus(st)

			reports = append(reports, evaluate(st)...)
		}
	}

	c.gauges.commit(agg)
	c.purgeStale(seen)
	return reports, nil
}

func (c *component) getOrCreateState(key stateKey, pod *workloadmeta.KubernetesPod, kind SubjectKind, ctrl controllerRef, containerName string) *subjectState {
	if st, ok := c.subjects[key]; ok {
		st.subjectKind = kind
		st.controller = ctrl
		st.observePod(pod)
		return st
	}
	st := &subjectState{
		subjectKind:   kind,
		controller:    ctrl,
		containerName: containerName,
	}
	st.observePod(pod)
	c.subjects[key] = st
	return st
}

func (c *component) refreshStats(st *subjectState, orchContainerID string) {
	if c.provider == nil || orchContainerID == "" {
		return
	}
	ctr, err := c.wmeta.GetContainer(orchContainerID)
	if err != nil || ctr == nil {
		return
	}
	collector := c.provider.GetCollector(provider.NewRuntimeMetadata(string(ctr.Runtime), string(ctr.RuntimeFlavor)))
	if collector == nil {
		return
	}
	stats, err := collector.GetContainerStats(ctr.Namespace, ctr.ID, c.cacheValidity)
	if err != nil || stats == nil {
		return
	}
	st.observeStats(stats)
}

func (c *component) applyContainerStatus(st *subjectState, pod *workloadmeta.KubernetesPod, containerName string) {
	if status := findStatus(pod.ContainerStatuses, containerName); status != nil {
		st.observeStatus(status)
		return
	}
	if status := findStatus(pod.InitContainerStatuses, containerName); status != nil {
		st.observeStatus(status)
		return
	}
	if status := findStatus(pod.EphemeralContainerStatuses, containerName); status != nil {
		st.observeStatus(status)
	}
}

func findStatus(statuses []workloadmeta.KubernetesContainerStatus, name string) *workloadmeta.KubernetesContainerStatus {
	for i := range statuses {
		if statuses[i].Name == name {
			return &statuses[i]
		}
	}
	return nil
}

// purgeStale drops subjects not seen this tick and past the staleness cutoff.
// Gauge cleanup is handled by gauges.commit's diff against the tick aggregate.
func (c *component) purgeStale(seen map[stateKey]struct{}) {
	cutoff := c.now().Add(-staleness)
	for key, st := range c.subjects {
		if _, alive := seen[key]; alive {
			continue
		}
		if st.lastSeenAt.After(cutoff) {
			continue
		}
		delete(c.subjects, key)
	}
}
