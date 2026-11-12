// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package tracer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const testRootNetNS = 4026531840

// newTestNetNSResolver returns a resolver whose namespace lookup uses pidToNetNS instead of procfs.
func newTestNetNSResolver(pidToNetNS map[int]uint32) *netnsContainerResolver {
	return &netnsContainerResolver{
		readNetNS: func(pid int) (uint32, error) {
			if ns, ok := pidToNetNS[pid]; ok {
				return ns, nil
			}
			return 0, errors.New("no such pid")
		},
		rootNetNS:         testRootNetNS,
		ownerImages:       toSet([]string{"ztunnel"}),
		ignoredContainers: toSet([]string{"vault-agent"}),
		now:               time.Now,
		containers:        make(map[*intern.Value]trackedContainer),
		byNetNS:           make(map[uint32]map[*intern.Value]netnsMember),
		pending:           make(map[*intern.Value]*workloadmeta.Container),
	}
}

func testContainerEvent(eventType workloadmeta.EventType, id, image string, pid int) workloadmeta.Event {
	return workloadmeta.Event{
		Type: eventType,
		Entity: &workloadmeta.Container{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: id},
			EntityMeta: workloadmeta.EntityMeta{Name: id},
			Image:      workloadmeta.ContainerImage{ShortName: image},
			PID:        pid,
			State:      workloadmeta.ContainerState{Running: true},
		},
	}
}

func TestNetNSContainerResolver(t *testing.T) {
	const (
		ztunnelNS  = 1001
		appNS      = 1002
		multiNS    = 1003
		unknownNS  = 1999
		ztunnelPID = 10
		appPID     = 20
		multiPID1  = 30
		multiPID2  = 31
		hostPID    = 40
	)

	r := newTestNetNSResolver(map[int]uint32{
		ztunnelPID: ztunnelNS,
		appPID:     appNS,
		multiPID1:  multiNS,
		multiPID2:  multiNS,
		hostPID:    testRootNetNS,
	})
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", ztunnelPID),
		testContainerEvent(workloadmeta.EventTypeSet, "app", "router", appPID),
		testContainerEvent(workloadmeta.EventTypeSet, "multi-a", "web", multiPID1),
		testContainerEvent(workloadmeta.EventTypeSet, "multi-b", "helper", multiPID2),
		testContainerEvent(workloadmeta.EventTypeSet, "hostnet", "node-exporter", hostPID),
	})

	ztunnel := intern.GetByString("ztunnel")
	app := intern.GetByString("app")

	t.Run("owner socket in a single-container pod is reattributed", func(t *testing.T) {
		cid, ok := r.resolve(appNS, ztunnel)
		require.True(t, ok)
		assert.Equal(t, app, cid)
	})

	t.Run("owner socket in its own namespace is unchanged", func(t *testing.T) {
		_, ok := r.resolve(ztunnelNS, ztunnel)
		assert.False(t, ok)
	})

	t.Run("non-owner sockets are unchanged", func(t *testing.T) {
		_, ok := r.resolve(ztunnelNS, app)
		assert.False(t, ok)
		_, ok = r.resolve(multiNS, app)
		assert.False(t, ok)
	})

	t.Run("multi-container pods are unchanged", func(t *testing.T) {
		_, ok := r.resolve(multiNS, ztunnel)
		assert.False(t, ok)
	})

	t.Run("host and unknown namespaces are unchanged", func(t *testing.T) {
		_, ok := r.resolve(testRootNetNS, ztunnel)
		assert.False(t, ok)
		_, ok = r.resolve(unknownNS, ztunnel)
		assert.False(t, ok)
		_, ok = r.resolve(0, ztunnel)
		assert.False(t, ok)
	})

	t.Run("no owner container is unchanged", func(t *testing.T) {
		_, ok := r.resolve(appNS, nil)
		assert.False(t, ok)
	})
}

func TestNetNSContainerResolverRemovesDeletedContainers(t *testing.T) {
	const reusedNS = 2002

	r := newTestNetNSResolver(map[int]uint32{10: 2001, 20: reusedNS, 21: reusedNS})
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		testContainerEvent(workloadmeta.EventTypeSet, "old-app", "router", 20),
	})

	ztunnel := intern.GetByString("ztunnel")

	cid, ok := r.resolve(reusedNS, ztunnel)
	require.True(t, ok)
	assert.Equal(t, intern.GetByString("old-app"), cid)

	now := time.Now()
	r.now = func() time.Time { return now }

	// the old pod goes away; its connections closed just before can still be read afterwards
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeUnset, "old-app", "router", 20),
	})
	cid, ok = r.resolve(reusedNS, ztunnel)
	require.True(t, ok, "a just-deleted container still owns connections closed before it went")
	assert.Equal(t, intern.GetByString("old-app"), cid)

	// the kernel hands its namespace inode to a new pod within the grace period
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "new-app", "router", 21),
	})
	_, ok = r.resolve(reusedNS, ztunnel)
	assert.False(t, ok, "while the old pod still counts, a reused namespace is ambiguous rather than the new pod's")

	// once the grace period passes, the old pod is purged and the namespace is the new pod's
	now = now.Add(removedContainerGrace + time.Second)
	r.purgeRemoved()
	cid, ok = r.resolve(reusedNS, ztunnel)
	require.True(t, ok)
	assert.Equal(t, intern.GetByString("new-app"), cid)

	// a namespace emptied by deletion is dropped after the grace period
	r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeUnset, "new-app", "router", 21)})
	now = now.Add(removedContainerGrace + time.Second)
	r.purgeRemoved()
	assert.NotContains(t, r.byNetNS, uint32(reusedNS))
}

func TestNetNSContainerResolverSkipsStoppedAndUnreadable(t *testing.T) {
	r := newTestNetNSResolver(map[int]uint32{10: 3001})

	stopped := testContainerEvent(workloadmeta.EventTypeSet, "stopped", "router", 0)
	stopped.Entity.(*workloadmeta.Container).State.Running = false

	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		stopped,
		testContainerEvent(workloadmeta.EventTypeSet, "unreadable", "router", 99),
	})

	assert.Len(t, r.containers, 1)
	assert.Len(t, r.byNetNS, 1, "only the running, readable container's namespace is mapped")
}

func TestNetNSContainerResolverConcurrentAccess(t *testing.T) {
	r := newTestNetNSResolver(map[int]uint32{10: 4001, 20: 4002})
	r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10)})

	ztunnel := intern.GetByString("ztunnel")
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20)})
			r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeUnset, "app", "router", 20)})
		}
	}()
	for i := 0; i < 1000; i++ {
		r.resolve(4002, ztunnel)
	}
	<-done

	// the writer's last event unset the app, so its namespace goes once the grace period passes
	later := time.Now().Add(removedContainerGrace + time.Second)
	r.now = func() time.Time { return later }
	r.purgeRemoved()
	_, ok := r.resolve(4002, ztunnel)
	assert.False(t, ok)
	assert.NotContains(t, r.byNetNS, uint32(4002))
}

func TestNetNSContainerResolverIgnoresUnchangedUpdates(t *testing.T) {
	reads := 0
	r := newTestNetNSResolver(nil)
	r.readNetNS = func(pid int) (uint32, error) {
		reads++
		return uint32(5000 + pid), nil
	}

	app := testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20)
	r.process([]workloadmeta.Event{app})
	require.Equal(t, 1, reads)

	// a readiness flip arrives as another Set event for the same running container
	unhealthy := testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20)
	unhealthy.Entity.(*workloadmeta.Container).State.Health = workloadmeta.ContainerHealthUnhealthy
	r.process([]workloadmeta.Event{unhealthy})
	assert.Equal(t, 1, reads, "an update that keeps the PID must not re-read the namespace")
	assert.Contains(t, r.byNetNS, uint32(5020), "an unhealthy running container stays mapped")

	// a restart gives the container a new process
	r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 21)})
	assert.Equal(t, 2, reads)
	assert.False(t, r.byNetNS[5020][intern.GetByString("app")].removedAt.IsZero(), "the old namespace only keeps the container as removed")
	assert.Contains(t, r.byNetNS, uint32(5021))
}

func TestNetNSContainerResolverIgnoresHelperContainers(t *testing.T) {
	const (
		ztunnelNS = 6001
		helperNS  = 6002
		multiNS   = 6003
	)
	r := newTestNetNSResolver(map[int]uint32{10: ztunnelNS, 20: helperNS, 21: helperNS, 30: multiNS, 31: multiNS})
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20),
		testContainerEvent(workloadmeta.EventTypeSet, "vault-agent", "vault", 21),
		testContainerEvent(workloadmeta.EventTypeSet, "web", "web", 30),
		testContainerEvent(workloadmeta.EventTypeSet, "worker", "worker", 31),
	})
	ztunnel := intern.GetByString("ztunnel")

	cid, ok := r.resolve(helperNS, ztunnel)
	require.True(t, ok, "a pod whose only other container is a helper counts as single-container")
	assert.Equal(t, intern.GetByString("app"), cid)

	_, ok = r.resolve(multiNS, ztunnel)
	assert.False(t, ok, "two application containers stay ambiguous")
}

func TestNetNSContainerResolverFallsBackToASoleHelper(t *testing.T) {
	const (
		soloNS = 6101
		twoNS  = 6102
	)
	r := newTestNetNSResolver(map[int]uint32{10: 6100, 20: soloNS, 30: twoNS, 31: twoNS})
	r.ignoredContainers = toSet([]string{"vault-agent", "cloud-sql-proxy"})
	secondHelper := testContainerEvent(workloadmeta.EventTypeSet, "csp-2", "cloud-sql-proxy", 31)
	secondHelper.Entity.(*workloadmeta.Container).Labels = map[string]string{kubernetesContainerNameLabel: "cloud-sql-proxy"}
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		// cloud-sql-proxy deployed as its own pod
		testContainerEvent(workloadmeta.EventTypeSet, "cloud-sql-proxy", "cloud-sql-proxy", 20),
		testContainerEvent(workloadmeta.EventTypeSet, "vault-agent", "vault", 30),
		secondHelper,
	})
	ztunnel := intern.GetByString("ztunnel")

	cid, ok := r.resolve(soloNS, ztunnel)
	require.True(t, ok, "a helper that is its namespace's only container is the target")
	assert.Equal(t, intern.GetByString("cloud-sql-proxy"), cid)

	_, ok = r.resolve(twoNS, ztunnel)
	assert.False(t, ok, "two helpers and no application stay ambiguous")
}

func TestNetNSContainerResolverReclassifiesOnSamePIDUpdates(t *testing.T) {
	const podNS = 7001
	r := newTestNetNSResolver(map[int]uint32{10: 7000, 20: podNS, 21: podNS})
	ztunnel := intern.GetByString("ztunnel")

	// containerd reports no container name; the kubelet fills it in later with the same PID
	unnamedHelper := testContainerEvent(workloadmeta.EventTypeSet, "helper-id", "vault", 21)
	unnamedHelper.Entity.(*workloadmeta.Container).Name = ""
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20),
		unnamedHelper,
	})
	_, ok := r.resolve(podNS, ztunnel)
	require.False(t, ok, "before the helper is named, the pod has two candidate containers")

	namedHelper := testContainerEvent(workloadmeta.EventTypeSet, "helper-id", "vault", 21)
	namedHelper.Entity.(*workloadmeta.Container).Name = ""
	namedHelper.Entity.(*workloadmeta.Container).Labels = map[string]string{kubernetesContainerNameLabel: "vault-agent"}
	r.process([]workloadmeta.Event{namedHelper})
	cid, ok := r.resolve(podNS, ztunnel)
	require.True(t, ok, "a same-PID update that names the helper must reclassify it")
	assert.Equal(t, intern.GetByString("app"), cid)

	// an owner image change is picked up without a PID change too
	r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "not-ztunnel", 10)})
	_, ok = r.resolve(podNS, ztunnel)
	assert.False(t, ok, "a container that no longer runs an owner image can't reattribute")
}

func TestNetNSContainerResolverSkipsPauseContainers(t *testing.T) {
	const podNS = 7101
	r := newTestNetNSResolver(map[int]uint32{10: 7100, 20: podNS, 21: podNS})
	pause := testContainerEvent(workloadmeta.EventTypeSet, "pause-id", "pause", 21)
	pause.Entity.(*workloadmeta.Container).Labels = map[string]string{"io.kubernetes.pod.name": "router-core-abc"}
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20),
		pause,
	})
	cid, ok := r.resolve(podNS, intern.GetByString("ztunnel"))
	require.True(t, ok, "the pause container holds the namespace but isn't a candidate")
	assert.Equal(t, intern.GetByString("app"), cid)
}

func TestNetNSContainerResolverRetriesFailedReads(t *testing.T) {
	const podNS = 7201
	readable := map[int]uint32{10: 7200}
	r := newTestNetNSResolver(readable)
	r.process([]workloadmeta.Event{
		testContainerEvent(workloadmeta.EventTypeSet, "ztunnel", "ztunnel", 10),
		testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20),
	})
	ztunnel := intern.GetByString("ztunnel")
	_, ok := r.resolve(podNS, ztunnel)
	require.False(t, ok)
	require.Contains(t, r.pending, intern.GetByString("app"))

	readable[20] = podNS
	r.retryPending()
	cid, ok := r.resolve(podNS, ztunnel)
	require.True(t, ok, "a namespace read that failed must be retried")
	assert.Equal(t, intern.GetByString("app"), cid)
	assert.Empty(t, r.pending)

	// a container that fails again and then stops isn't retried forever
	delete(readable, 20)
	r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 22)})
	require.Contains(t, r.pending, intern.GetByString("app"))
	r.process([]workloadmeta.Event{testContainerEvent(workloadmeta.EventTypeUnset, "app", "router", 22)})
	assert.Empty(t, r.pending)
}

func TestNetNSContainerResolverStart(t *testing.T) {
	const podNS = 7301
	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Supply(config.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	r := newTestNetNSResolver(map[int]uint32{10: 7300, 20: podNS, 30: podNS})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	r.start(ctx, wmeta)

	mapped := func(netns uint32, id string) bool {
		r.mtx.RLock()
		defer r.mtx.RUnlock()
		m, ok := r.byNetNS[netns][intern.GetByString(id)]
		return ok && m.removedAt.IsZero()
	}
	app := testContainerEvent(workloadmeta.EventTypeSet, "app", "router", 20).Entity.(*workloadmeta.Container)
	wmeta.Set(app)
	require.Eventually(t, func() bool { return mapped(podNS, "app") }, 5*time.Second, 10*time.Millisecond)

	wmeta.Unset(app)
	require.Eventually(t, func() bool { return !mapped(podNS, "app") }, 5*time.Second, 10*time.Millisecond)

	// after cancel, the subscription is gone and new containers aren't mapped
	cancel()
	time.Sleep(100 * time.Millisecond)
	wmeta.Set(testContainerEvent(workloadmeta.EventTypeSet, "late", "router", 30).Entity.(*workloadmeta.Container))
	assert.Never(t, func() bool { return mapped(podNS, "late") }, 300*time.Millisecond, 20*time.Millisecond)
}

func TestEnableNetNSContainerAttributionGuards(t *testing.T) {
	off := &Tracer{config: &netconfig.Config{}}
	require.NoError(t, off.EnableNetNSContainerAttribution(context.Background(), nil))
	assert.Nil(t, off.netnsResolver.Load(), "the feature is off by default")

	// on, but without process event monitoring there's no owner container to start from
	noProcessCache := &Tracer{config: &netconfig.Config{EnableNetNSContainerAttribution: true}}
	require.NoError(t, noProcessCache.EnableNetNSContainerAttribution(context.Background(), nil))
	assert.Nil(t, noProcessCache.netnsResolver.Load())
}

func TestNetNSContainerResolverOnTunnel(t *testing.T) {
	r := &netnsContainerResolver{tunnelPorts: map[uint16]struct{}{15008: {}}}
	assert.True(t, r.onTunnel(15008, 41000), "the proxy's tunnel listener")
	assert.True(t, r.onTunnel(41000, 15008), "the proxy dialing a remote tunnel")
	assert.False(t, r.onTunnel(41000, 8080), "delivering into the pod")
	assert.False(t, r.onTunnel(15001, 41000), "capturing the app's outbound traffic")

	none := &netnsContainerResolver{}
	assert.False(t, none.onTunnel(15008, 8080), "with no tunnel ports nothing is reattributed")
}

func TestMajorityNetNS(t *testing.T) {
	assert.Equal(t, uint32(1), majorityNetNS(2, map[uint32]int{1: 9, 2: 1}), "a main thread left in a pod's namespace is outvoted")
	assert.Equal(t, uint32(2), majorityNetNS(2, map[uint32]int{1: 1, 2: 1}), "ties go to the main thread")
	assert.Equal(t, uint32(2), majorityNetNS(2, nil), "without task counts the main thread's namespace is used")
}

func TestReadProcessNetNSUsesMostThreads(t *testing.T) {
	// regular files stand in for namespaces: hard links share an inode, like threads sharing a namespace
	root := t.TempDir()
	own, pod := filepath.Join(root, "own"), filepath.Join(root, "pod")
	require.NoError(t, os.WriteFile(own, nil, 0o600))
	require.NoError(t, os.WriteFile(pod, nil, 0o600))
	inode := func(path string) uint32 {
		var st syscall.Stat_t
		require.NoError(t, syscall.Stat(path, &st))
		return uint32(st.Ino)
	}
	link := func(target, path string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.Link(target, path))
	}

	procRoot := filepath.Join(root, "proc")
	// the main thread is in the pod's namespace, like istio-cni after setting a pod up; its other threads aren't
	link(pod, filepath.Join(procRoot, "42", "ns", "net"))
	link(pod, filepath.Join(procRoot, "42", "task", "42", "ns", "net"))
	for _, tid := range []string{"43", "44", "45"} {
		link(own, filepath.Join(procRoot, "42", "task", tid, "ns", "net"))
	}
	ns, err := readProcessNetNS(procRoot, 42)
	require.NoError(t, err)
	assert.Equal(t, inode(own), ns)

	// a single-threaded process uses its main thread's namespace
	link(pod, filepath.Join(procRoot, "50", "ns", "net"))
	link(pod, filepath.Join(procRoot, "50", "task", "50", "ns", "net"))
	ns, err = readProcessNetNS(procRoot, 50)
	require.NoError(t, err)
	assert.Equal(t, inode(pod), ns)
}
