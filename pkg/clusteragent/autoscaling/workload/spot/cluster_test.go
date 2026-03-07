// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"context"
	"fmt"
	"maps"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/spot"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// fakeCluster simulates a Kubernetes cluster for testing.
// It maintains a set of nodes and a workloadmeta store, runs a fake pod scheduler
// that transitions Pending pods to Running, and supports admission hooks.
type fakeCluster struct {
	t          *testing.T
	wlm        workloadmetamock.Mock
	subscribed chan struct{}
	hooks      []admissionHook

	mu          sync.Mutex
	nodes       []*corev1.Node
	pendingPods map[types.UID]*corev1.Pod
}

// admissionHook is called for each pod during admission and may mutate it.
// It returns true if the pod was modified, false otherwise.
type admissionHook func(pod *corev1.Pod) (bool, error)

// newFakeCluster creates a fakeCluster.
func newFakeCluster(t *testing.T) *fakeCluster {
	wlm := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	cluster := &fakeCluster{
		t:           t,
		wlm:         wlm,
		subscribed:  make(chan struct{}),
		pendingPods: make(map[types.UID]*corev1.Pod),
	}
	go cluster.runPodScheduler()
	<-cluster.subscribed
	return cluster
}

// AddOnDemandNode adds an on-demand node.
func (c *fakeCluster) AddOnDemandNode(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodes = append(c.nodes, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	})
}

// AddSpotNode adds a spot node with the Karpenter capacity-type label and NoSchedule taint.
func (c *fakeCluster) AddSpotNode(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodes = append(c.nodes, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{spot.KarpenterCapacityTypeLabel: spot.KarpenterCapacityTypeSpot},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: spot.KarpenterCapacityTypeLabel, Value: spot.KarpenterCapacityTypeSpot, Effect: corev1.TaintEffectNoSchedule},
			},
		},
	})
}

// AddAdmissionHook registers a hook called on every admitted pod.
func (c *fakeCluster) AddAdmissionHook(hook admissionHook) {
	c.hooks = append(c.hooks, hook)
}

// CreatePod runs all registered admission hooks on the pod then creates it as Pending.
func (c *fakeCluster) CreatePod(pod *corev1.Pod) {
	unmodifiedCopy := pod.DeepCopy()
	for _, hook := range c.hooks {
		updated, err := hook(pod)
		require.NoError(c.t, err)
		if !updated {
			require.Equal(c.t, unmodifiedCopy, pod)
		}
	}

	c.createPending(pod)
}

func (c *fakeCluster) createPending(pod *corev1.Pod) {
	uid, err := uuid.NewRandom()
	require.NoError(c.t, err)

	pod.UID = types.UID(uid.String())
	pod.Status.Phase = corev1.PodPending

	c.mu.Lock()
	c.pendingPods[pod.UID] = pod
	c.mu.Unlock()

	c.setPod(pod)
}

func (c *fakeCluster) setPod(pod *corev1.Pod) {
	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, ref := range pod.OwnerReferences {
		owners = append(owners, workloadmeta.KubernetesPodOwner{Kind: ref.Kind, Name: ref.Name})
	}
	c.wlm.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   string(pod.UID),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Namespace:   pod.Namespace,
			Name:        pod.Name,
			Annotations: pod.Annotations,
			Labels:      pod.Labels,
		},
		Owners:   owners,
		Phase:    string(pod.Status.Phase),
		NodeName: pod.Spec.NodeName,
	})
}

// DeleteOwnerPods deletes all pods owned by the given ownerKind/namespace/ownerName.
func (c *fakeCluster) DeleteOwnerPods(ownerKind, namespace, ownerName string) {
	for _, pod := range c.wlm.ListKubernetesPods() {
		if pod.Namespace != namespace {
			continue
		}
		for _, owner := range pod.Owners {
			if owner.Kind == ownerKind && owner.Name == ownerName {
				c.deletePod(pod.ID)
				break
			}
		}
	}
}

// deletePod removes a pod from the cluster and unsets it in the WLM store.
func (c *fakeCluster) deletePod(uid string) {
	pod, err := c.wlm.GetKubernetesPod(uid)
	if err == nil {
		c.wlm.Unset(pod)
	}
}

// WLM returns the underlying workloadmeta mock store.
func (c *fakeCluster) WLM() workloadmetamock.Mock {
	return c.wlm
}

const (
	assertWaitFor = 1 * time.Second
	assertTick    = 50 * time.Millisecond
)

// AssertOwnerPods waits until all pods owned by ownerKind/namespace/ownerName satisfy check.
func (c *fakeCluster) AssertOwnerPods(ownerKind, namespace, ownerName string, check func(wlm []*workloadmeta.KubernetesPod) bool) {
	assert.Eventually(c.t, func() bool {
		var filtered []*workloadmeta.KubernetesPod
		for _, pod := range c.wlm.ListKubernetesPods() {
			if pod.Namespace != namespace {
				continue
			}
			for _, owner := range pod.Owners {
				if owner.Kind == ownerKind && owner.Name == ownerName {
					filtered = append(filtered, pod)
					break
				}
			}
		}
		return check(filtered)
	}, assertWaitFor, assertTick)
}

// runPodScheduler simulates a Kubernetes scheduler for testing.
// It watches for Pending pods and transitions them to Running after a short delay.
func (c *fakeCluster) runPodScheduler() {
	filter := workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindKubernetesPod).Build()
	ch := c.wlm.Subscribe("fake-scheduler", workloadmeta.NormalPriority, filter)
	close(c.subscribed)
	defer c.wlm.Unsubscribe(ch)

	ctx := c.t.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case bundle, more := <-ch:
			if !more {
				return
			}
			for _, event := range bundle.Events {
				pod, ok := event.Entity.(*workloadmeta.KubernetesPod)
				if !ok {
					continue
				}
				switch event.Type {
				case workloadmeta.EventTypeSet:
					if pod.Phase == string(corev1.PodPending) {
						c.trySchedule(pod.ID)
					}
				case workloadmeta.EventTypeUnset:
					c.mu.Lock()
					delete(c.pendingPods, types.UID(pod.ID))
					c.mu.Unlock()
				}
			}
			bundle.Acknowledge()
		}
	}
}

// trySchedule attempts to bind a pending pod to a node.
// It selects the first node whose labels match the pod's nodeSelector and whose taints are all tolerated.
// If no node matches the pod stays Pending. On success the pod transitions to Running after a short delay.
func (c *fakeCluster) trySchedule(uid string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	pod, ok := c.pendingPods[types.UID(uid)]
	require.True(c.t, ok, "pending pod %s not found", uid)

	var nodeName string
	for _, node := range c.nodes {
		if len(pod.Spec.NodeSelector) > 0 && !selectorMatchesNodeLabels(pod.Spec.NodeSelector, node) {
			continue
		}
		if podToleratesNode(pod, node) {
			nodeName = node.Name
			break
		}
	}
	if nodeName == "" {
		return
	}

	delete(c.pendingPods, types.UID(uid))

	pod.Spec.NodeName = nodeName
	pod.Status.Phase = corev1.PodRunning

	go func() {
		time.Sleep(100 * time.Millisecond)
		c.setPod(pod)
	}()
}

func selectorMatchesNodeLabels(selector map[string]string, node *corev1.Node) bool {
	return labels.SelectorFromSet(selector).Matches(labels.Set(node.Labels))
}

func podToleratesNode(pod *corev1.Pod, node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		tolerated := false
		for _, toleration := range pod.Spec.Tolerations {
			if toleration.ToleratesTaint(&taint) {
				tolerated = true
				break
			}
		}
		if !tolerated {
			return false
		}
	}
	return true
}

// replicaSetName returns a valid ReplicaSet name for the given deployment name with a random suffix.
// The suffix uses KubeAllowedEncodeStringAlphaNums characters so that
// ParseDeploymentForReplicaSet correctly resolves the name back to the deployment.
func replicaSetName(deployment string) string {
	var b strings.Builder
	b.WriteString(deployment)
	b.WriteByte('-')
	const chars = "bcdfghjklmnpqrstvwxz2456789"
	for range 5 {
		b.WriteByte(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

// newPods returns n pods owned by the given kind/namespace/name, each with a cloned copy of annotations.
func newPods(ownerKind, namespace, ownerName string, replicas int, annotations map[string]string) []*corev1.Pod {
	pods := make([]*corev1.Pod, replicas)
	for i := range replicas {
		pods[i] = newPod(namespace, fmt.Sprintf("%s-%d", ownerName, i+1), ownerKind, ownerName, maps.Clone(annotations))
	}
	return pods
}

// newPod builds a corev1.Pod with the given owner and annotations.
func newPod(namespace, name, ownerKind, ownerName string, annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: ownerKind, Name: ownerName},
			},
		},
	}
}
