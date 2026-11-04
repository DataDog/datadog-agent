// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"go4.org/intern"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	utilcontainers "github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	netnsAttributionModuleName = tracerModuleName + "__netns_attribution"

	// kubernetesContainerNameLabel holds the Kubernetes container name; the runtime-reported name is
	// empty on containerd and k8s_<container>_<pod>_... on docker
	kubernetesContainerNameLabel = "io.kubernetes.container.name"

	// netnsRetryInterval is how often failed namespace reads are retried, since the next workloadmeta
	// update for a container can be minutes away
	netnsRetryInterval = 30 * time.Second

	// removedContainerGrace is how long a removed container still counts in its namespace. Closed connections
	// are resolved when the tracer reads them, which can be well after the pod is gone; meanwhile a new pod that
	// gets the same namespace inode makes the namespace ambiguous rather than taking the old pod's connections.
	removedContainerGrace = 2 * time.Minute
)

var netnsAttributionTelemetry = struct {
	reattributed   telemetryComponent.Counter
	unknownNetNS   telemetryComponent.Counter
	multiContainer telemetryComponent.Counter
	readErrors     telemetryComponent.Counter
	trackedNetNS   telemetryComponent.Gauge
	owners         telemetryComponent.Gauge
	pendingReads   telemetryComponent.Gauge
}{
	telemetryimpl.GetCompatComponent().NewCounter(netnsAttributionModuleName, "reattributed", nil, "Connections attributed to the container owning their network namespace"),
	telemetryimpl.GetCompatComponent().NewCounter(netnsAttributionModuleName, "unknown_netns", nil, "Owner-allowlisted connections in a network namespace with no known container"),
	telemetryimpl.GetCompatComponent().NewCounter(netnsAttributionModuleName, "multi_container_skipped", nil, "Owner-allowlisted connections left unchanged because their network namespace has several containers"),
	telemetryimpl.GetCompatComponent().NewCounter(netnsAttributionModuleName, "netns_read_errors", nil, "Failures reading a container's network namespace"),
	telemetryimpl.GetCompatComponent().NewGauge(netnsAttributionModuleName, "tracked_netns", nil, "Number of network namespaces mapped to containers"),
	telemetryimpl.GetCompatComponent().NewGauge(netnsAttributionModuleName, "owner_containers", nil, "Number of tracked containers running an allowlisted owner image; zero means nothing can be reattributed"),
	telemetryimpl.GetCompatComponent().NewGauge(netnsAttributionModuleName, "pending_netns_reads", nil, "Running containers whose network namespace couldn't be read yet"),
}

// netnsContainerResolver maps network namespaces to the containers that live in them, so a socket
// created by a proxy inside another pod's network namespace (Istio ambient's ztunnel calls setns
// before bind and connect) can be attributed to that pod rather than to the proxy.
type netnsContainerResolver struct {
	readNetNS   func(pid int) (uint32, error)
	rootNetNS   uint32
	ownerImages map[string]struct{}
	// ignoredContainers are helper containers (for example vault-agent or cloud-sql-proxy) that share a
	// pod's network namespace and carry no traffic of their own, only the application's, so they don't
	// count toward the pod's containers unless one is the namespace's only container
	ignoredContainers map[string]struct{}
	// tunnelPorts are the mesh tunnel's ports; only sockets on them are reattributed
	tunnelPorts map[uint16]struct{}
	now         func() time.Time

	mtx        sync.RWMutex
	containers map[*intern.Value]trackedContainer
	byNetNS    map[uint32]map[*intern.Value]netnsMember
	// pending holds running containers whose namespace read failed, retried every netnsRetryInterval
	pending map[*intern.Value]*workloadmeta.Container
}

type trackedContainer struct {
	pid   int
	netns uint32
	owner bool
}

// netnsMember is a container in a namespace. A removed one keeps counting until removedContainerGrace passes.
type netnsMember struct {
	helper    bool
	removedAt time.Time
}

func (m netnsMember) expired(now time.Time) bool {
	return !m.removedAt.IsZero() && now.Sub(m.removedAt) > removedContainerGrace
}

func newNetNSContainerResolver(ownerImages, ignoredContainers []string, tunnelPorts []int) (*netnsContainerResolver, error) {
	procRoot := kernel.HostProc()
	rootNetNS, err := netnsutil.GetNetNsInoFromPid(procRoot, 1)
	if err != nil {
		return nil, err
	}

	r := &netnsContainerResolver{
		readNetNS: func(pid int) (uint32, error) {
			return readProcessNetNS(procRoot, pid)
		},
		rootNetNS:         rootNetNS,
		ownerImages:       toSet(ownerImages),
		ignoredContainers: toSet(ignoredContainers),
		tunnelPorts:       make(map[uint16]struct{}, len(tunnelPorts)),
		now:               time.Now,
		containers:        make(map[*intern.Value]trackedContainer),
		byNetNS:           make(map[uint32]map[*intern.Value]netnsMember),
		pending:           make(map[*intern.Value]*workloadmeta.Container),
	}
	for _, port := range tunnelPorts {
		if port > 0 && port <= 65535 {
			r.tunnelPorts[uint16(port)] = struct{}{}
		}
	}
	return r, nil
}

// readProcessNetNS returns the network namespace most of a process's threads are in. /proc/<pid>/ns/net is the
// main thread's, and a process that enters pods' namespaces (istio-cni, for one) can leave its main thread in a
// pod's, which would count it as a container of that pod. Ties go to the main thread.
func readProcessNetNS(procRoot string, pid int) (uint32, error) {
	leader, err := netnsutil.GetNetNsInoFromPid(procRoot, pid)
	if err != nil {
		return 0, err
	}
	taskDir := filepath.Join(procRoot, strconv.Itoa(pid), "task")
	tasks, err := os.ReadDir(taskDir)
	if err != nil || len(tasks) <= 1 {
		return leader, nil
	}
	counts := make(map[uint32]int, 2)
	for _, task := range tasks {
		var st syscall.Stat_t
		if syscall.Stat(filepath.Join(taskDir, task.Name(), "ns", "net"), &st) == nil {
			counts[uint32(st.Ino)]++
		}
	}
	return majorityNetNS(leader, counts), nil
}

func majorityNetNS(leader uint32, counts map[uint32]int) uint32 {
	best, bestCount := leader, counts[leader]
	for netns, n := range counts {
		if n > bestCount {
			best, bestCount = netns, n
		}
	}
	return best
}

// onTunnel reports whether a socket with these ports is on the mesh tunnel. A proxy's other legs inside the pod
// duplicate the app's own sockets, so they stay on the proxy.
func (r *netnsContainerResolver) onTunnel(sport, dport uint16) bool {
	_, src := r.tunnelPorts[sport]
	_, dst := r.tunnelPorts[dport]
	return src || dst
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return set
}

func (r *netnsContainerResolver) start(ctx context.Context, wmeta workloadmeta.Component) {
	// No source filter: system-probe's workloadmeta is remote, so every event arrives with source
	// remote_workloadmeta, carrying the core agent's entity merged across all sources on one ordered
	// stream. Filtering on SourceRuntime would match nothing.
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		SetEventType(workloadmeta.EventTypeAll).
		Build()
	ch := wmeta.Subscribe("CNM NetNS Container Attribution", workloadmeta.NormalPriority, filter)
	go func() {
		retry := time.NewTicker(netnsRetryInterval)
		defer retry.Stop()
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
			case <-retry.C:
				r.retryPending()
				r.purgeRemoved()
			}
		}
	}()
}

// retryPending re-applies running containers whose namespace read failed.
func (r *netnsContainerResolver) retryPending() {
	r.mtx.RLock()
	events := make([]workloadmeta.Event, 0, len(r.pending))
	for _, container := range r.pending {
		events = append(events, workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: container})
	}
	r.mtx.RUnlock()
	if len(events) > 0 {
		r.process(events)
	}
}

// process applies a bundle of container events. Only the subscription goroutine calls it, so the
// state read in the first pass can't change before the last; resolve only takes the read lock.
func (r *netnsContainerResolver) process(events []workloadmeta.Event) {
	type update struct {
		id        *intern.Value
		container *workloadmeta.Container
		running   bool
		read      bool
		netns     uint32
		readErr   bool
	}
	updates := make([]update, 0, len(events))

	r.mtx.RLock()
	for _, event := range events {
		container, ok := event.Entity.(*workloadmeta.Container)
		if !ok {
			continue
		}
		u := update{
			id:        intern.GetByString(container.ID),
			container: container,
			running:   event.Type == workloadmeta.EventTypeSet && container.State.Running && container.PID > 0,
		}
		if u.running {
			// most updates (health, readiness, a later source filling in the name) keep the process,
			// so the namespace is unchanged; only the classification is recomputed
			if tc, ok := r.containers[u.id]; ok && tc.pid == container.PID {
				u.netns = tc.netns
			} else {
				u.read = true
			}
		}
		updates = append(updates, u)
	}
	r.mtx.RUnlock()

	// read namespaces without holding the lock: each is an open and fstat of /proc/<pid>/ns/net
	for i := range updates {
		u := &updates[i]
		if !u.read {
			continue
		}
		netns, err := r.readNetNS(u.container.PID)
		if err != nil {
			netnsAttributionTelemetry.readErrors.Inc()
			log.Debugf("netns attribution: could not read network namespace of container %s (pid %d): %s", u.container.ID, u.container.PID, err)
			u.readErr = true
			continue
		}
		u.netns = netns
	}

	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, u := range updates {
		r.removeLocked(u.id)
		delete(r.pending, u.id)
		if !u.running {
			continue
		}
		if u.readErr {
			r.pending[u.id] = u.container
			continue
		}
		r.addLocked(u.id, u.container, u.netns)
	}

	r.purgeRemovedLocked(r.now())

	owners := 0
	for _, tc := range r.containers {
		if tc.owner {
			owners++
		}
	}
	netnsAttributionTelemetry.trackedNetNS.Set(float64(len(r.byNetNS)))
	netnsAttributionTelemetry.owners.Set(float64(owners))
	netnsAttributionTelemetry.pendingReads.Set(float64(len(r.pending)))
}

func (r *netnsContainerResolver) addLocked(id *intern.Value, container *workloadmeta.Container, netns uint32) {
	_, owner := r.ownerImages[container.Image.ShortName]
	r.containers[id] = trackedContainer{pid: container.PID, netns: netns, owner: owner}

	// containers in the host network namespace are never reattribution targets, and neither are pause
	// containers, which only hold the pod's namespace; a running one leaves no removed entry behind
	if netns == r.rootNetNS || utilcontainers.IsPauseContainer(container.Labels) {
		r.dropMemberLocked(netns, id)
		return
	}
	// helpers' traffic, if any, belongs to the pod's application, so they're only a target on their own
	_, helper := r.ignoredContainers[kubernetesContainerName(container)]
	if r.byNetNS[netns] == nil {
		r.byNetNS[netns] = make(map[*intern.Value]netnsMember)
	}
	r.byNetNS[netns][id] = netnsMember{helper: helper}
}

func (r *netnsContainerResolver) dropMemberLocked(netns uint32, id *intern.Value) {
	if members := r.byNetNS[netns]; members != nil {
		delete(members, id)
		if len(members) == 0 {
			delete(r.byNetNS, netns)
		}
	}
}

// kubernetesContainerName returns the container's Kubernetes name, falling back to the reported name.
func kubernetesContainerName(container *workloadmeta.Container) string {
	if name := container.Labels[kubernetesContainerNameLabel]; name != "" {
		return name
	}
	return container.Name
}

// removeLocked forgets a container but keeps it in its namespace, marked removed, until the grace period
// passes. An update that keeps the container running re-adds it straight away.
func (r *netnsContainerResolver) removeLocked(id *intern.Value) {
	tc, ok := r.containers[id]
	if !ok {
		return
	}
	delete(r.containers, id)
	if m, ok := r.byNetNS[tc.netns][id]; ok && m.removedAt.IsZero() {
		m.removedAt = r.now()
		r.byNetNS[tc.netns][id] = m
	}
}

func (r *netnsContainerResolver) purgeRemoved() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.purgeRemovedLocked(r.now())
	netnsAttributionTelemetry.trackedNetNS.Set(float64(len(r.byNetNS)))
}

func (r *netnsContainerResolver) purgeRemovedLocked(now time.Time) {
	for netns, members := range r.byNetNS {
		for id, m := range members {
			if m.expired(now) {
				delete(members, id)
			}
		}
		if len(members) == 0 {
			delete(r.byNetNS, netns)
		}
	}
}

// resolve returns the container to attribute a socket to, given the socket's network namespace and
// the container of the process that owns it. It only reattributes sockets owned by an allowlisted
// container, in a namespace that holds exactly one application container which isn't the owner, or no
// application container and exactly one helper.
func (r *netnsContainerResolver) resolve(netns uint32, ownerContainer *intern.Value) (*intern.Value, bool) {
	if ownerContainer == nil || netns == 0 || netns == r.rootNetNS {
		return nil, false
	}

	r.mtx.RLock()
	defer r.mtx.RUnlock()

	if !r.containers[ownerContainer].owner {
		return nil, false
	}

	members := r.byNetNS[netns]
	if _, ownNamespace := members[ownerContainer]; ownNamespace {
		return nil, false
	}

	now := r.now()
	var apps, helpers int
	var app, helper *intern.Value
	for id, m := range members {
		switch {
		case m.expired(now):
		case m.helper:
			helpers++
			helper = id
		default:
			apps++
			app = id
		}
	}

	switch {
	case apps == 1:
		netnsAttributionTelemetry.reattributed.Inc()
		return app, true
	case apps == 0 && helpers == 1:
		netnsAttributionTelemetry.reattributed.Inc()
		return helper, true
	case apps == 0 && helpers == 0:
		netnsAttributionTelemetry.unknownNetNS.Inc()
		return nil, false
	}
	netnsAttributionTelemetry.multiContainer.Inc()
	return nil, false
}
