// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// Ground truth for the testdata/kai fixtures, taken from the README's pod ->
// GPU table. That table was built by running `nvidia-smi -L` inside each pod,
// independently of the label chain this collector walks, so it is a real
// expectation rather than a restatement of the code under test. Recapturing the
// fixtures against another cluster changes these UUIDs, and this table has to
// be updated with them.
const (
	kaiGPUSharedByFracAAndB = "GPU-0b5693b2-d023-de74-f5c9-bb2c1cad110d"
	kaiGPUOfFracC           = "GPU-32a93c9b-38dc-933a-192a-58a0a2f92a19"
	kaiGPUSharedByCUDAMulti = "GPU-be38b139-9eb5-692d-593c-b1041f029e26"
	kaiGPUSecondOfFracMulti = "GPU-d7bea0fc-e039-0702-46b6-97510ff81958"
)

// kaiFixtureGPUUUIDs is every GPU of the fixture cluster that KAI reserved.
var kaiFixtureGPUUUIDs = []string{
	kaiGPUSharedByFracAAndB,
	kaiGPUOfFracC,
	kaiGPUSharedByCUDAMulti,
	kaiGPUSecondOfFracMulti,
}

// kaiFixtureExpectedGPUs is the ground-truth table above, keyed by pod name.
var kaiFixtureExpectedGPUs = map[string][]string{
	"frac-a":     {kaiGPUSharedByFracAAndB},
	"frac-b":     {kaiGPUSharedByFracAAndB},
	"frac-c":     {kaiGPUOfFracC},
	"frac-cuda":  {kaiGPUSharedByCUDAMulti},
	"frac-multi": {kaiGPUSharedByCUDAMulti, kaiGPUSecondOfFracMulti},
}

// kaiReservationPodOfFracMultiSecondGPU holds group 0247250f-..., the second of
// frac-multi's two GPUs. Removing it from a fixture set is how these tests
// produce a genuinely pending group, as distinct from a device NVML no longer
// reports.
const kaiReservationPodOfFracMultiSecondGPU = "gpu-reservation-c39b814206ba355f"

// fixturePodList mirrors just enough of a `kubectl get pods -o yaml` document
// to rebuild workloadmeta entities from the captured objects. Decoding into the
// real Kubernetes types would pull the kubelet collector's conversion into
// these tests; what matters here is that the labels, annotations and container
// names come from a real KAI cluster rather than from a hand-written literal.
type fixturePodList struct {
	Items []fixturePod `yaml:"items"`
}

type fixturePod struct {
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
		UID       string `yaml:"uid"`
		// Decoded as a string and parsed explicitly: the fixtures quote the
		// timestamp, which makes it a !!str rather than a !!timestamp, so
		// decoding straight into a time.Time cannot be relied on.
		CreationTimestamp string            `yaml:"creationTimestamp"`
		Labels            map[string]string `yaml:"labels"`
		Annotations       map[string]string `yaml:"annotations"`
	} `yaml:"metadata"`
	Status struct {
		Phase                 string                   `yaml:"phase"`
		ContainerStatuses     []fixtureContainerStatus `yaml:"containerStatuses"`
		InitContainerStatuses []fixtureContainerStatus `yaml:"initContainerStatuses"`
	} `yaml:"status"`
}

type fixtureContainerStatus struct {
	Name        string `yaml:"name"`
	ContainerID string `yaml:"containerID"`
}

// loadKAIFixturePods decodes the captured pod lists into workloadmeta pods,
// keyed by pod name.
func loadKAIFixturePods(t *testing.T, files ...string) map[string]*workloadmeta.KubernetesPod {
	t.Helper()

	pods := make(map[string]*workloadmeta.KubernetesPod)
	for _, file := range files {
		raw, err := os.ReadFile(filepath.Join("testdata", "kai", file))
		require.NoError(t, err, "reading fixture %s", file)

		var list fixturePodList
		require.NoError(t, yaml.Unmarshal(raw, &list), "decoding fixture %s", file)
		require.NotEmpty(t, list.Items, "fixture %s has no pods", file)

		for _, item := range list.Items {
			require.NotContains(t, pods, item.Metadata.Name, "duplicate pod %s across fixtures", item.Metadata.Name)
			pods[item.Metadata.Name] = item.toWorkloadmetaPod(t)
		}
	}
	return pods
}

func (p fixturePod) toWorkloadmetaPod(t *testing.T) *workloadmeta.KubernetesPod {
	t.Helper()
	require.NotEmpty(t, p.Metadata.UID, "fixture pod %s has no UID", p.Metadata.Name)

	// Built from the container statuses, and skipping entries without an ID,
	// exactly as the kubelet collector does in
	// comp/core/workloadmeta/collectors/util/kubelet.go. Building from
	// spec.containers instead would hand the code under test a pod shape
	// production never sees: in particular a container that the runtime has not
	// created yet is absent from the list, not present with an empty ID.
	toOrchestrator := func(statuses []fixtureContainerStatus) []workloadmeta.OrchestratorContainer {
		var out []workloadmeta.OrchestratorContainer
		for _, status := range statuses {
			_, id, found := strings.Cut(status.ContainerID, "://")
			require.True(t, found, "fixture container %s/%s has no runtime scheme in %q", p.Metadata.Name, status.Name, status.ContainerID)
			if id == "" {
				continue
			}
			out = append(out, workloadmeta.OrchestratorContainer{ID: id, Name: status.Name})
		}
		return out
	}

	createdAt, err := time.Parse(time.RFC3339, p.Metadata.CreationTimestamp)
	require.NoError(t, err, "fixture pod %s has an unparseable creationTimestamp %q", p.Metadata.Name, p.Metadata.CreationTimestamp)

	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   p.Metadata.UID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        p.Metadata.Name,
			Namespace:   p.Metadata.Namespace,
			Labels:      p.Metadata.Labels,
			Annotations: p.Metadata.Annotations,
		},
		Phase:             p.Status.Phase,
		CreationTimestamp: createdAt,
		Containers:        toOrchestrator(p.Status.ContainerStatuses),
		InitContainers:    toOrchestrator(p.Status.InitContainerStatuses),
	}
}

// podContainerID returns the ID of the pod's only container, failing the test
// if the fixture shape changed.
func podContainerID(t *testing.T, pod *workloadmeta.KubernetesPod) string {
	t.Helper()
	require.NotNil(t, pod, "fixture is missing a pod the test needs")
	require.Len(t, pod.Containers, 1, "fixture pod %s no longer has exactly one container", pod.Name)
	require.NotEmpty(t, pod.Containers[0].ID, "fixture pod %s has no container ID", pod.Name)
	return pod.Containers[0].ID
}

// kaiFixtureDeviceCache is a device cache reporting the given GPUs, or every
// fixture GPU when called with none.
//
// MinorNumber is -1 on purpose: the zero value is 0, which would make every one
// of these devices a candidate for the DRA whole-card resolution of
// /dev/nvidia0 and silently cross-wire the two features in
// TestCreateContainerGPUEventsMergesKAIWithDRA.
func kaiFixtureDeviceCache(uuids ...string) *fakeDeviceCache {
	if len(uuids) == 0 {
		uuids = kaiFixtureGPUUUIDs
	}
	devices := make([]ddnvml.Device, 0, len(uuids))
	for _, uuid := range uuids {
		devices = append(devices, &ddnvml.PhysicalDevice{
			DeviceInfo:  ddnvml.DeviceInfo{UUID: uuid},
			MinorNumber: -1,
		})
	}
	return &fakeDeviceCache{devices: devices}
}

// storeWithPods returns a workloadmeta mock holding the given pods and a
// container entity per container, as the kubelet and runtime collectors would.
func storeWithPods(t *testing.T, pods ...*workloadmeta.KubernetesPod) workloadmetamock.Mock {
	t.Helper()

	store := testutil.GetWorkloadMetaMock(t)
	for _, pod := range pods {
		store.Set(pod)
		for _, container := range slices.Concat(pod.Containers, pod.InitContainers) {
			store.Set(&workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   container.ID,
				},
				EntityMeta: workloadmeta.EntityMeta{Name: container.Name},
			})
		}
	}
	return store
}

func allKAIFixturePods(t *testing.T) map[string]*workloadmeta.KubernetesPod {
	t.Helper()
	return loadKAIFixturePods(t, "pods-kai-test.yaml", "pods-kai-resource-reservation.yaml")
}

func podSlice(pods map[string]*workloadmeta.KubernetesPod) []*workloadmeta.KubernetesPod {
	out := make([]*workloadmeta.KubernetesPod, 0, len(pods))
	for _, pod := range pods {
		out = append(out, pod)
	}
	return out
}

// newKAICollector returns a collector wired to the store with the KAI feature
// on and the per-pull state the event builder keeps.
func newKAICollector(store workloadmeta.Component) *collector {
	return &collector{
		store:                  store,
		kaiSchedulerEnabled:    true,
		seenContainerGPUs:      make(map[string]struct{}),
		publishedContainerGPUs: make(map[string][]string),
	}
}

// setGPUDeviceIDs indexes the GPUDeviceIDs published by a pull, by container ID.
func setGPUDeviceIDs(t *testing.T, events []workloadmeta.CollectorEvent) map[string][]string {
	t.Helper()

	byContainerID := make(map[string][]string, len(events))
	for _, event := range events {
		container, ok := event.Entity.(*workloadmeta.Container)
		require.True(t, ok, "container events carry Container entities")
		if event.Type == workloadmeta.EventTypeSet {
			byContainerID[container.ID] = container.GPUDeviceIDs
		}
	}
	return byContainerID
}

func TestKAIFractionalGPUsFromFixtures(t *testing.T) {
	// The whole chain against the captured cluster: a single fraction, two pods
	// sharing one GPU, a pod sharing a GPU with one of them, and a multi-GPU
	// fraction using the per-GPU label form.
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))

	fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache())

	require.Len(t, fractions.uuidsByContainer, len(kaiFixtureExpectedGPUs), "one mapping per fractional pod")
	for podName, wantUUIDs := range kaiFixtureExpectedGPUs {
		containerID := podContainerID(t, pods[podName])

		assert.ElementsMatch(t, wantUUIDs, fractions.uuidsByContainer[containerID], "GPUs of %s", podName)
		assert.Contains(t, fractions.claiming, containerID, "%s must claim its fraction", podName)
		assert.NotContains(t, fractions.partial, containerID, "%s resolved fully", podName)
	}
}

func TestKAIFractionalGPUsClaimsWithoutReservationPod(t *testing.T) {
	// The binder has not created the reservation pod yet, or the kubelet
	// collector has not caught up with it. The fraction cannot be resolved, but
	// the container must still count as claiming: retraction keys off the claim
	// set, and dropping it here would unset the mapping and set it again on the
	// next pull, flapping the pod tags on the device's metrics.
	pods := loadKAIFixturePods(t, "pods-kai-test.yaml")
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))

	fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache())

	require.Empty(t, fractions.uuidsByContainer, "no reservation pod means nothing resolves")
	require.Len(t, fractions.claiming, len(kaiFixtureExpectedGPUs), "every fractional pod still claims")
	require.Len(t, fractions.partial, len(kaiFixtureExpectedGPUs), "and every one of them is pending, not accounted for")
	for name, pod := range pods {
		assert.Contains(t, fractions.claiming, podContainerID(t, pod), "%s must claim even unresolved", name)
	}
}

func TestKAIFractionalGPUsSkipsUUIDUnknownToNVML(t *testing.T) {
	// A reservation pod naming a GPU this node's NVML does not report -- the
	// device is excluded from collection, or disappeared between the pod's
	// binding and this pull -- must not produce a device ID that no GPU entity
	// matches.
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))

	// Everything except the GPU shared by frac-a and frac-b.
	fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache(kaiGPUOfFracC, kaiGPUSharedByCUDAMulti, kaiGPUSecondOfFracMulti))

	for _, podName := range []string{"frac-a", "frac-b"} {
		containerID := podContainerID(t, pods[podName])
		assert.NotContains(t, fractions.uuidsByContainer, containerID, "%s must not map to an unknown GPU", podName)
		// Accounted for, not pending: the reservation pod was seen, so waiting
		// longer cannot change anything.
		assert.NotContains(t, fractions.partial, containerID, "%s has nothing left to wait for", podName)
		// And therefore not claiming either. These pods share a single GPU, so
		// nothing resolved at all; holding the claim would keep whatever was
		// published while the device existed, leaving the container pointing at
		// a GPU entity that no longer exists.
		assert.NotContains(t, fractions.claiming, containerID, "%s has nothing left to claim", podName)
	}

	// The other pods are unaffected by their neighbour's unresolvable GPU.
	assert.Equal(t, []string{kaiGPUOfFracC}, fractions.uuidsByContainer[podContainerID(t, pods["frac-c"])])
	assert.Len(t, fractions.uuidsByContainer[podContainerID(t, pods["frac-multi"])], 2)
}

func TestKAIFractionalGPUsMarksPartialMultiGPUResolution(t *testing.T) {
	// frac-multi holds half of two GPUs. When one of its reservation pods is not
	// in the store -- the binder has not finished, or the kubelet collector has
	// not caught up -- that group is pending, and the container must be flagged
	// partial so the caller keeps whatever it published before instead of
	// replacing it with the smaller set.
	pods := allKAIFixturePods(t)
	require.Contains(t, pods, kaiReservationPodOfFracMultiSecondGPU, "fixture is missing the reservation pod this test removes")
	containerID := podContainerID(t, pods["frac-multi"])
	delete(pods, kaiReservationPodOfFracMultiSecondGPU)

	c := newKAICollector(storeWithPods(t, podSlice(pods)...))
	fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache())

	assert.Equal(t, []string{kaiGPUSharedByCUDAMulti}, fractions.uuidsByContainer[containerID], "the resolvable half is still reported")
	assert.Contains(t, fractions.claiming, containerID)
	assert.Contains(t, fractions.partial, containerID, "frac-multi accounted for only one of its two groups")

	// The single-group pods are unaffected.
	assert.NotContains(t, fractions.partial, podContainerID(t, pods["frac-cuda"]))
	assert.NotContains(t, fractions.partial, podContainerID(t, pods["frac-a"]))
}

func TestKAIFractionalGPUsDoesNotMarkADepartedGPUAsPending(t *testing.T) {
	// The mirror image: a GPU NVML has stopped reporting is accounted for, not
	// pending. Flagging it partial would make the caller's shrink guard pin the
	// container's mapping for as long as the pod lives, so a GPU removed for
	// good would never leave its GPUDeviceIDs.
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))
	containerID := podContainerID(t, pods["frac-multi"])

	// Everything except frac-multi's second GPU.
	fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache(kaiGPUSharedByFracAAndB, kaiGPUOfFracC, kaiGPUSharedByCUDAMulti))

	assert.Equal(t, []string{kaiGPUSharedByCUDAMulti}, fractions.uuidsByContainer[containerID])
	assert.Contains(t, fractions.claiming, containerID)
	assert.NotContains(t, fractions.partial, containerID, "the reservation was seen; its device is simply gone")
}

// reservationPod builds a reservation pod for a GPU group, as KAI's binder
// would. Its containers are irrelevant: a reservation pod is only ever read for
// its group label and its GPU annotation.
func reservationPod(name, group, uuid, phase string, createdAt time.Time) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: name},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        name,
			Namespace:   "kai-resource-reservation",
			Labels:      map[string]string{kaiGPUGroupLabel: group},
			Annotations: map[string]string{kaiReservationGPUAnnotation: uuid},
		},
		Phase:             phase,
		CreationTimestamp: createdAt,
	}
}

func TestKAIFractionalGPUsSkipsTerminatedReservationPods(t *testing.T) {
	// A GPU was re-bound and the previous reservation pod has not gone away yet,
	// so it still reports a GPU for a group it no longer holds. It is
	// deliberately the NEWER of the two here: without the phase check it would
	// win the group on its timestamp and point frac-a and frac-b at the wrong
	// GPU, so the test fails if that check is removed.
	for _, phase := range []string{podPhaseSucceeded, podPhaseFailed} {
		t.Run(phase, func(t *testing.T) {
			pods := allKAIFixturePods(t)
			require.Contains(t, pods, "gpu-reservation-6d8e72c077c10ec9", "fixture is missing the reservation pod this test re-binds")
			live := pods["gpu-reservation-6d8e72c077c10ec9"]

			terminated := reservationPod(
				live.Name+"-old",
				live.Labels[kaiGPUGroupLabel],
				kaiGPUOfFracC, // a different GPU, so a win would be visible
				phase,
				live.CreationTimestamp.Add(time.Hour),
			)

			c := newKAICollector(storeWithPods(t, append(podSlice(pods), terminated)...))
			fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache())

			assert.Equal(t, []string{kaiGPUSharedByFracAAndB},
				fractions.uuidsByContainer[podContainerID(t, pods["frac-a"])],
				"the live reservation must win over a %s one", phase)
		})
	}
}

func TestKAIFractionalGPUsPrefersTheNewestReservation(t *testing.T) {
	// Two *live* reservation pods for one group: the GPU was re-bound and the
	// previous pod is still Running. The pod list iterates a map, so without a
	// deterministic rule the group's UUID would alternate between pulls and
	// make every pull look like a change.
	pods := allKAIFixturePods(t)
	require.Contains(t, pods, "gpu-reservation-6d8e72c077c10ec9", "fixture is missing the reservation pod this test re-binds")
	live := pods["gpu-reservation-6d8e72c077c10ec9"]
	group := live.Labels[kaiGPUGroupLabel]
	fracAContainerID := podContainerID(t, pods["frac-a"])

	t.Run("the newer timestamp wins", func(t *testing.T) {
		newer := reservationPod(live.Name+"-new", group, kaiGPUOfFracC, "Running", live.CreationTimestamp.Add(time.Hour))
		c := newKAICollector(storeWithPods(t, append(podSlice(pods), newer)...))

		for range 5 {
			fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache())
			require.Equal(t, []string{kaiGPUOfFracC}, fractions.uuidsByContainer[fracAContainerID])
		}
	})

	t.Run("equal timestamps fall back to the lower UUID", func(t *testing.T) {
		// The live pod's "GPU-0b56..." sorts before "GPU-d7be...", so it must
		// keep the group whichever order the pod list yields.
		tie := reservationPod(live.Name+"-tie", group, kaiGPUSecondOfFracMulti, "Running", live.CreationTimestamp)
		c := newKAICollector(storeWithPods(t, append(podSlice(pods), tie)...))

		for range 5 {
			fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache())
			require.Equal(t, []string{kaiGPUSharedByFracAAndB}, fractions.uuidsByContainer[fracAContainerID])
		}
	})
}

func TestKAIFractionalGPUsIsDeterministic(t *testing.T) {
	// Label maps iterate in random order. A multi-GPU fraction that returned
	// its UUIDs in a different order on each pull would make every pull look
	// like a change to workloadmeta, so the groups are sorted and the UUID order
	// follows from them. Sorting by group puts frac-multi's second device first.
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))

	containerID := podContainerID(t, pods["frac-multi"])

	// Repeated because an unsorted implementation would only be caught by about
	// half of the possible map orders.
	for range 5 {
		fractions := c.kaiFractionalGPUs(kaiFixtureDeviceCache())
		require.Equal(t,
			[]string{kaiGPUSecondOfFracMulti, kaiGPUSharedByCUDAMulti},
			fractions.uuidsByContainer[containerID])
	}
}

func TestKAIGPUGroups(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{
			name:   "no labels",
			labels: nil,
			want:   nil,
		},
		{
			name:   "plain label",
			labels: map[string]string{kaiGPUGroupLabel: "group-a"},
			want:   []string{"group-a"},
		},
		{
			name: "per-GPU labels are sorted",
			labels: map[string]string{
				kaiGPUGroupLabelPrefix + "group-b": "group-b",
				kaiGPUGroupLabelPrefix + "group-a": "group-a",
			},
			want: []string{"group-a", "group-b"},
		},
		{
			name: "both forms are deduplicated",
			labels: map[string]string{
				kaiGPUGroupLabel:                   "group-a",
				kaiGPUGroupLabelPrefix + "group-a": "group-a",
			},
			want: []string{"group-a"},
		},
		{
			name:   "an unrelated label is ignored",
			labels: map[string]string{"kai.scheduler/queue": "default-queue"},
			want:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{Labels: test.labels},
			}
			require.Equal(t, test.want, kaiGPUGroups(pod))
		})
	}
}

func TestKAIFractionContainerID(t *testing.T) {
	tests := []struct {
		name           string
		annotations    map[string]string
		containers     []workloadmeta.OrchestratorContainer
		initContainers []workloadmeta.OrchestratorContainer
		want           string
	}{
		{
			name: "picked by the per-container annotation",
			annotations: map[string]string{
				kaiContainerAnnotationPrefix + "gpu-workload.gpu-memory.request": "7680Mi",
			},
			containers: []workloadmeta.OrchestratorContainer{
				{ID: "sidecar-id", Name: "sidecar"},
				{ID: "gpu-id", Name: "gpu-workload"},
			},
			want: "gpu-id",
		},
		{
			name:        "picked by gpu-fraction-container-name",
			annotations: map[string]string{kaiFractionContainerNameAnnotation: "gpu-workload"},
			containers: []workloadmeta.OrchestratorContainer{
				{ID: "sidecar-id", Name: "sidecar"},
				{ID: "gpu-id", Name: "gpu-workload"},
			},
			want: "gpu-id",
		},
		{
			name: "the annotation can name an init container",
			annotations: map[string]string{
				kaiContainerAnnotationPrefix + "warmup.gpu-memory.request": "7680Mi",
			},
			containers:     []workloadmeta.OrchestratorContainer{{ID: "main-id", Name: "main"}},
			initContainers: []workloadmeta.OrchestratorContainer{{ID: "warmup-id", Name: "warmup"}},
			want:           "warmup-id",
		},
		{
			name: "the per-container annotation wins over the name annotation",
			annotations: map[string]string{
				kaiContainerAnnotationPrefix + "annotated.gpu-memory.request": "7680Mi",
				kaiFractionContainerNameAnnotation:                            "named",
			},
			containers: []workloadmeta.OrchestratorContainer{
				{ID: "annotated-id", Name: "annotated"},
				{ID: "named-id", Name: "named"},
			},
			want: "annotated-id",
		},
		{
			name: "a prefix of another container's name does not match",
			annotations: map[string]string{
				kaiContainerAnnotationPrefix + "gpu-workload.gpu-memory.request": "7680Mi",
			},
			containers: []workloadmeta.OrchestratorContainer{
				{ID: "sidecar-id", Name: "gpu-workload-sidecar"},
				{ID: "gpu-id", Name: "gpu-workload"},
			},
			want: "gpu-id",
		},
		{
			name: "annotations naming several containers attribute none of them",
			annotations: map[string]string{
				kaiContainerAnnotationPrefix + "beta.gpu-memory.request":  "7680Mi",
				kaiContainerAnnotationPrefix + "alpha.gpu-memory.request": "7680Mi",
			},
			containers: []workloadmeta.OrchestratorContainer{
				{ID: "beta-id", Name: "beta"},
				{ID: "alpha-id", Name: "alpha"},
			},
			// Since v0.18 KAI rejects such a pod at admission and binding, so
			// there is no container it would have picked.
			want: "",
		},
		{
			name:        "the only container needs no annotation",
			annotations: nil,
			containers:  []workloadmeta.OrchestratorContainer{{ID: "only-id", Name: "only"}},
			want:        "only-id",
		},
		{
			name:        "an init container does not make the only regular container ambiguous",
			annotations: nil,
			// KAI's default is the first regular container, so one regular
			// container is unambiguous however many init containers ran
			// alongside it. Counting both lists together would return the init
			// container while it was the only one started, and then nothing.
			containers:     []workloadmeta.OrchestratorContainer{{ID: "trainer-id", Name: "trainer"}},
			initContainers: []workloadmeta.OrchestratorContainer{{ID: "warmup-id", Name: "warmup"}},
			want:           "trainer-id",
		},
		{
			name:        "no regular container has started yet",
			annotations: nil,
			// Only the init container is in the store. KAI would pick the
			// regular container, which is not there to pick.
			initContainers: []workloadmeta.OrchestratorContainer{{ID: "warmup-id", Name: "warmup"}},
			want:           "",
		},
		{
			name:        "nothing names a container and there is more than one",
			annotations: nil,
			containers: []workloadmeta.OrchestratorContainer{
				{ID: "first-id", Name: "first"},
				{ID: "second-id", Name: "second"},
			},
			// The kubelet does not report containers in spec order, so KAI's
			// "first container" default cannot be reproduced. Guessing would
			// attribute the GPU to the wrong container half the time.
			want: "",
		},
		{
			name: "the annotated container is not in the store yet",
			annotations: map[string]string{
				kaiContainerAnnotationPrefix + "gpu-workload.gpu-memory.request": "7680Mi",
			},
			// The GPU container's image is still pulling, so the kubelet
			// collector dropped it; only the sidecar is present. Falling back to
			// it would publish a wrong mapping and retract it a pull later.
			containers: []workloadmeta.OrchestratorContainer{{ID: "sidecar-id", Name: "istio-proxy"}},
			want:       "",
		},
		{
			name: "an absent annotated container does not fall through to the name annotation",
			annotations: map[string]string{
				kaiContainerAnnotationPrefix + "gpu-workload.gpu-memory.request": "7680Mi",
				kaiFractionContainerNameAnnotation:                               "sidecar",
			},
			// KAI's rule is "the annotated container, else the named one", so a
			// per-container annotation that cannot be resolved yet must be
			// waited for rather than passed over for the lower-precedence name.
			containers: []workloadmeta.OrchestratorContainer{{ID: "sidecar-id", Name: "sidecar"}},
			want:       "",
		},
		{
			name:        "a name annotation pointing at no container",
			annotations: map[string]string{kaiFractionContainerNameAnnotation: "does-not-exist"},
			containers:  []workloadmeta.OrchestratorContainer{{ID: "first-id", Name: "first"}},
			want:        "",
		},
		{
			name:       "no containers",
			containers: nil,
			want:       "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := &workloadmeta.KubernetesPod{
				EntityMeta:     workloadmeta.EntityMeta{Annotations: test.annotations},
				Containers:     test.containers,
				InitContainers: test.initContainers,
			}
			require.Equal(t, test.want, kaiFractionContainerID(pod))
		})
	}
}

func TestNewCollectorReadsKAISchedulerSetting(t *testing.T) {
	// Goes through the real config key end to end, so that the schema node,
	// NewNVMLCollectorConfig and the collector field cannot drift apart.
	store := testutil.GetWorkloadMetaMock(t)

	cfg := config.NewMock(t)
	require.False(t, newCollector(store, cfg).kaiSchedulerEnabled, "gpu.kai_scheduler.enabled must default to off")

	cfg.SetInTest("gpu.kai_scheduler.enabled", true)
	require.True(t, newCollector(store, cfg).kaiSchedulerEnabled)
}

func TestCreateContainerGPUEventsPublishesKAIFractions(t *testing.T) {
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))

	published := setGPUDeviceIDs(t, c.createContainerGPUEvents(kaiFixtureDeviceCache()))

	// TestKAIFractionalGPUsFromFixtures asserts the mapping itself; this pins
	// that it reaches the store as one event per fractional container, carrying
	// the UUIDs rather than just the right count.
	require.Len(t, published, len(kaiFixtureExpectedGPUs), "one event per fractional container")
	assert.Equal(t, []string{kaiGPUSharedByFracAAndB}, published[podContainerID(t, pods["frac-a"])])

	// Excluding the reservation containers from the device tags is a separate
	// concern; this collector simply never maps them.
	for name, pod := range pods {
		if pod.Annotations[kaiReservationGPUAnnotation] == "" {
			continue
		}
		assert.NotContains(t, published, podContainerID(t, pod), "reservation pod %s", name)
	}
}

func TestCreateContainerGPUEventsSkipsKAIWhenDisabled(t *testing.T) {
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))
	c.kaiSchedulerEnabled = false

	require.Empty(t, c.createContainerGPUEvents(kaiFixtureDeviceCache()), "no events while the feature is off")
	require.Empty(t, c.seenContainerGPUs, "nothing claims while the feature is off")
}

func TestCreateContainerGPUEventsKeepsAPartialKAIMapping(t *testing.T) {
	// A multi-GPU fraction with a pending group must keep the mapping published
	// earlier rather than have it replaced by the smaller set, which would drop
	// the attribution of the other device and restore it on the next pull.
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))
	containerID := podContainerID(t, pods["frac-multi"])

	published := setGPUDeviceIDs(t, c.createContainerGPUEvents(kaiFixtureDeviceCache()))
	require.Len(t, published[containerID], 2, "both of frac-multi's GPUs resolve first")

	// One of the two reservation pods leaves the store, so its group goes back
	// to pending.
	degradedPods := allKAIFixturePods(t)
	delete(degradedPods, kaiReservationPodOfFracMultiSecondGPU)
	c.store = storeWithPods(t, podSlice(degradedPods)...)

	degraded := setGPUDeviceIDs(t, c.createContainerGPUEvents(kaiFixtureDeviceCache()))

	assert.NotContains(t, degraded, containerID, "the shrunken set must not replace the published one")
	assert.Equal(t, []string{kaiGPUSecondOfFracMulti, kaiGPUSharedByCUDAMulti}, c.publishedContainerGPUs[containerID],
		"the full mapping is retained")
	assert.Contains(t, c.seenContainerGPUs, containerID, "and the container still claims, so it is not retracted")
}

func TestCreateContainerGPUEventsShrinksWhenAGPULeavesNVML(t *testing.T) {
	// The mirror image of the test above. A device NVML no longer reports is
	// not coming back, and Pull has already unset its GPU entity, so keeping it
	// in GPUDeviceIDs would leave the container pointing at an entity nothing
	// matches -- for as long as the pod lives.
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))
	containerID := podContainerID(t, pods["frac-multi"])

	published := setGPUDeviceIDs(t, c.createContainerGPUEvents(kaiFixtureDeviceCache()))
	require.Len(t, published[containerID], 2, "both of frac-multi's GPUs resolve first")

	shrunk := setGPUDeviceIDs(t, c.createContainerGPUEvents(
		kaiFixtureDeviceCache(kaiGPUSharedByFracAAndB, kaiGPUOfFracC, kaiGPUSharedByCUDAMulti)))

	assert.Equal(t, []string{kaiGPUSharedByCUDAMulti}, shrunk[containerID], "the departed GPU must leave the mapping")
	assert.Equal(t, []string{kaiGPUSharedByCUDAMulti}, c.publishedContainerGPUs[containerID])
}

func TestCreateContainerGPUEventsRetractsASingleFractionWhoseGPULeftNVML(t *testing.T) {
	// A pod sharing ONE GPU that leaves NVML resolves to nothing, so there is
	// no smaller set to publish and the shrink guard never runs -- the path
	// TestCreateContainerGPUEventsShrinksWhenAGPULeavesNVML covers only works
	// because frac-multi has a second GPU to publish. The mapping must still
	// go: a container left pointing at a device no GPU entity matches is
	// counted by the GPU check as a missing container mapping on every run, for
	// as long as the pod lives.
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))
	containerID := podContainerID(t, pods["frac-a"])

	published := setGPUDeviceIDs(t, c.createContainerGPUEvents(kaiFixtureDeviceCache()))
	require.Equal(t, []string{kaiGPUSharedByFracAAndB}, published[containerID], "frac-a resolves first")

	// Its GPU leaves NVML for good. The reservation pod is still in the store,
	// so the group is accounted for rather than pending.
	events := c.createContainerGPUEvents(
		kaiFixtureDeviceCache(kaiGPUOfFracC, kaiGPUSharedByCUDAMulti, kaiGPUSecondOfFracMulti))

	retracted := make(map[string]struct{}, len(events))
	for _, event := range events {
		if event.Type != workloadmeta.EventTypeUnset {
			continue
		}
		container, ok := event.Entity.(*workloadmeta.Container)
		require.True(t, ok)
		retracted[container.ID] = struct{}{}
	}

	assert.Contains(t, retracted, containerID, "the stale mapping must be unset")
	assert.NotContains(t, c.seenContainerGPUs, containerID, "and the container must stop claiming")
	assert.NotContains(t, c.publishedContainerGPUs, containerID, "so its publication record goes with it")
}

func TestCreateContainerGPUEventsRetractsKAIFraction(t *testing.T) {
	// When the fractional pod goes away the mapping must be retracted:
	// workloadmeta keeps a SourceNVML entity alive after every other source has
	// unset it, so without this the container never leaves the store, the tagger
	// never expires its tags, and the GPU's metrics carry the dead pod's tags
	// once the fraction is handed to someone else.
	pods := allKAIFixturePods(t)
	c := newKAICollector(storeWithPods(t, podSlice(pods)...))

	require.NotEmpty(t, c.createContainerGPUEvents(kaiFixtureDeviceCache()))

	fracAContainerID := podContainerID(t, pods["frac-a"])
	require.Contains(t, c.seenContainerGPUs, fracAContainerID)
	require.Contains(t, c.publishedContainerGPUs, fracAContainerID)

	// The fractional pods are deleted; only the reservation pods remain.
	reservationsOnly := loadKAIFixturePods(t, "pods-kai-resource-reservation.yaml")
	c.store = storeWithPods(t, podSlice(reservationsOnly)...)

	events := c.createContainerGPUEvents(kaiFixtureDeviceCache())
	require.NotEmpty(t, events, "the gone containers must be retracted")

	retracted := make(map[string]struct{}, len(events))
	for _, event := range events {
		require.Equal(t, workloadmeta.EventTypeUnset, event.Type)
		container, ok := event.Entity.(*workloadmeta.Container)
		require.True(t, ok)
		retracted[container.ID] = struct{}{}
	}

	assert.Contains(t, retracted, fracAContainerID, "the gone container's mapping must be unset")
	assert.Empty(t, c.seenContainerGPUs, "nothing claims once the pods are gone")
	assert.Empty(t, c.publishedContainerGPUs, "the publication record is dropped with the claim")
}

func TestCreateContainerGPUEventsRetractsATerminatedKAIFraction(t *testing.T) {
	// A finished fractional pod stays visible through the kubelet for a while
	// (kubernetes_pod_expiration_duration), with its container ID and group
	// labels. KAI no longer counts it as using its fraction, so its mapping must
	// be retracted rather than keep the pod's tags on a GPU others still use.
	// frac-b keeps running on the same GPU, so the reservation pod stays: the
	// retraction must come from the terminated workload itself.
	for _, phase := range []string{podPhaseSucceeded, podPhaseFailed} {
		t.Run(phase, func(t *testing.T) {
			pods := allKAIFixturePods(t)
			c := newKAICollector(storeWithPods(t, podSlice(pods)...))
			require.NotEmpty(t, c.createContainerGPUEvents(kaiFixtureDeviceCache()))

			fracAContainerID := podContainerID(t, pods["frac-a"])
			fracBContainerID := podContainerID(t, pods["frac-b"])
			require.Contains(t, c.seenContainerGPUs, fracAContainerID)
			require.Contains(t, c.publishedContainerGPUs, fracAContainerID)

			pods["frac-a"].Phase = phase
			c.store = storeWithPods(t, podSlice(pods)...)

			var unset []string
			for _, event := range c.createContainerGPUEvents(kaiFixtureDeviceCache()) {
				if event.Type == workloadmeta.EventTypeUnset {
					unset = append(unset, event.Entity.GetID().ID)
				}
			}

			assert.Equal(t, []string{fracAContainerID}, unset, "only the terminated container's mapping must be unset")
			assert.NotContains(t, c.seenContainerGPUs, fracAContainerID)
			assert.NotContains(t, c.publishedContainerGPUs, fracAContainerID)
			assert.Contains(t, c.seenContainerGPUs, fracBContainerID, "the pod still sharing the GPU keeps its mapping")
		})
	}
}

func TestCreateContainerGPUEventsMergesKAIWithDRA(t *testing.T) {
	// A container can hold both a DRA-allocated device and a KAI fraction of
	// another GPU. Both must land in one event: workloadmeta replaces the
	// per-source entity on each Set, so two events would leave only the last
	// one's UUIDs.
	const (
		claimUID = "c8593c85-440d-4156-b199-aea592ff83df"
		draUUID  = "GPU-11111111-1111-1111-1111-111111111111"
	)
	// A whole-card claim pinning /dev/nvidia0, which resolves to the device
	// below by minor number.
	writeCDISpec(t, claimUID, cdiSpecWholeCard)

	pods := allKAIFixturePods(t)
	containerID := podContainerID(t, pods["frac-a"])

	store := testutil.GetWorkloadMetaMock(t)
	for _, pod := range podSlice(pods) {
		store.Set(pod)
	}
	// frac-a's container also holds a DRA device.
	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{{
			Name:       nvidiaDRADriver,
			CdiDevices: []string{"k8s.gpu.nvidia.com/claim=" + claimUID + "-gpu-0"},
		}},
	})

	deviceCache := kaiFixtureDeviceCache()
	deviceCache.devices = append(deviceCache.devices, &ddnvml.PhysicalDevice{
		DeviceInfo:  ddnvml.DeviceInfo{UUID: draUUID},
		MinorNumber: 0,
	})

	c := newKAICollector(store)
	published := setGPUDeviceIDs(t, c.createContainerGPUEvents(deviceCache))

	require.ElementsMatch(t, []string{draUUID, kaiGPUSharedByFracAAndB}, published[containerID],
		"the container's DRA device and its KAI fraction must both be published, in one event")
}
