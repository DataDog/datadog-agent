// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"context"
	"sync"

	"go4.org/intern"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const netnsAttributionModuleName = tracerModuleName + "__netns_attribution"

var netnsAttributionTelemetry = struct {
	reattributed   telemetryComponent.Counter
	unknownNetNS   telemetryComponent.Counter
	multiContainer telemetryComponent.Counter
	readErrors     telemetryComponent.Counter
	trackedNetNS   telemetryComponent.Gauge
}{
	telemetryimpl.GetCompatComponent().NewCounter(netnsAttributionModuleName, "reattributed", nil, "Connections attributed to the container owning their network namespace"),
	telemetryimpl.GetCompatComponent().NewCounter(netnsAttributionModuleName, "unknown_netns", nil, "Owner-allowlisted connections in a network namespace with no known container"),
	telemetryimpl.GetCompatComponent().NewCounter(netnsAttributionModuleName, "multi_container_skipped", nil, "Owner-allowlisted connections left unchanged because their network namespace has several containers"),
	telemetryimpl.GetCompatComponent().NewCounter(netnsAttributionModuleName, "netns_read_errors", nil, "Failures reading a container's network namespace"),
	telemetryimpl.GetCompatComponent().NewGauge(netnsAttributionModuleName, "tracked_netns", nil, "Number of network namespaces mapped to containers"),
}

// netnsContainerResolver maps network namespaces to the containers that live in them, so a socket
// created by a proxy inside another pod's network namespace (Istio ambient's ztunnel calls setns
// before bind and connect) can be attributed to that pod rather than to the proxy.
type netnsContainerResolver struct {
	readNetNS   func(pid int) (uint32, error)
	rootNetNS   uint32
	ownerImages map[string]struct{}

	mtx        sync.RWMutex
	containers map[*intern.Value]trackedContainer
	byNetNS    map[uint32]map[*intern.Value]struct{}
}

type trackedContainer struct {
	netns uint32
	owner bool
}

func newNetNSContainerResolver(ownerImages []string) (*netnsContainerResolver, error) {
	procRoot := kernel.HostProc()
	rootNetNS, err := netnsutil.GetNetNsInoFromPid(procRoot, 1)
	if err != nil {
		return nil, err
	}

	r := &netnsContainerResolver{
		readNetNS: func(pid int) (uint32, error) {
			return netnsutil.GetNetNsInoFromPid(procRoot, pid)
		},
		rootNetNS:   rootNetNS,
		ownerImages: make(map[string]struct{}, len(ownerImages)),
		containers:  make(map[*intern.Value]trackedContainer),
		byNetNS:     make(map[uint32]map[*intern.Value]struct{}),
	}
	for _, image := range ownerImages {
		r.ownerImages[image] = struct{}{}
	}
	return r, nil
}

func (r *netnsContainerResolver) start(ctx context.Context, wmeta workloadmeta.Component) {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		SetEventType(workloadmeta.EventTypeAll).
		Build()
	ch := wmeta.Subscribe("CNM NetNS Container Attribution", workloadmeta.NormalPriority, filter)
	go func() {
		for {
			select {
			case <-ctx.Done():
				wmeta.Unsubscribe(ch)
				return
			case eventBundle, ok := <-ch:
				if !ok {
					return
				}
				r.process(eventBundle.Events)
				eventBundle.Acknowledge()
			}
		}
	}()
}

func (r *netnsContainerResolver) process(events []workloadmeta.Event) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	for _, event := range events {
		container, ok := event.Entity.(*workloadmeta.Container)
		if !ok {
			continue
		}

		id := intern.GetByString(container.ID)
		r.removeLocked(id)

		if event.Type != workloadmeta.EventTypeSet || !container.State.Running || container.PID <= 0 {
			continue
		}

		netns, err := r.readNetNS(container.PID)
		if err != nil {
			netnsAttributionTelemetry.readErrors.Inc()
			log.Debugf("netns attribution: could not read network namespace of container %s (pid %d): %s", container.ID, container.PID, err)
			continue
		}

		_, owner := r.ownerImages[container.Image.ShortName]
		r.containers[id] = trackedContainer{netns: netns, owner: owner}

		// containers in the host network namespace are never reattribution targets
		if netns == r.rootNetNS {
			continue
		}
		if r.byNetNS[netns] == nil {
			r.byNetNS[netns] = make(map[*intern.Value]struct{})
		}
		r.byNetNS[netns][id] = struct{}{}
	}

	netnsAttributionTelemetry.trackedNetNS.Set(float64(len(r.byNetNS)))
}

func (r *netnsContainerResolver) removeLocked(id *intern.Value) {
	tc, ok := r.containers[id]
	if !ok {
		return
	}
	delete(r.containers, id)
	if ids := r.byNetNS[tc.netns]; ids != nil {
		delete(ids, id)
		if len(ids) == 0 {
			delete(r.byNetNS, tc.netns)
		}
	}
}

// resolve returns the container to attribute a socket to, given the socket's network namespace and
// the container of the process that owns it. It only reattributes sockets owned by an allowlisted
// container, in a namespace that holds exactly one container which isn't the owner.
func (r *netnsContainerResolver) resolve(netns uint32, ownerContainer *intern.Value) (*intern.Value, bool) {
	if ownerContainer == nil || netns == 0 || netns == r.rootNetNS {
		return nil, false
	}

	r.mtx.RLock()
	defer r.mtx.RUnlock()

	if !r.containers[ownerContainer].owner {
		return nil, false
	}

	candidates := r.byNetNS[netns]
	if _, ownNamespace := candidates[ownerContainer]; ownNamespace {
		return nil, false
	}

	switch len(candidates) {
	case 0:
		netnsAttributionTelemetry.unknownNetNS.Inc()
		return nil, false
	case 1:
		for id := range candidates {
			netnsAttributionTelemetry.reattributed.Inc()
			return id, true
		}
	}

	netnsAttributionTelemetry.multiContainer.Inc()
	return nil, false
}
