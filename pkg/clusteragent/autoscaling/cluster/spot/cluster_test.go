// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"context"
	"maps"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/spot"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

var deploymentsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

// fakeCluster simulates a Kubernetes cluster for testing.
// It maintains a set of nodes and a workloadmeta store,
// runs a fake pod scheduler and supports admission hooks.
type fakeCluster struct {
	t               *testing.T
	wlm             workloadmetamock.Mock
	subscribed      chan struct{}
	dynamicClient   dynamic.Interface
	podCreatedHooks []admissionHook
	podDeletedHooks []deletionHook

	mu          sync.Mutex
	nodes       []*corev1.Node
	pendingPods map[types.UID]*corev1.Pod
}

// admissionHook is called for each pod during admission and may mutate it.
// It returns true if the pod was modified, false otherwise.
type admissionHook func(pod *corev1.Pod) (bool, error)

// deletionHook is called immediately when a pod is deleted.
type deletionHook func(pod *corev1.Pod)

// fakeDeployment simulates a Kubernetes Deployment in tests.
type fakeDeployment struct {
	cluster            *fakeCluster
	namespace          string
	name               string
	existingReplicaSet string
	podSelector        map[string]string
}

// newFakeCluster creates a fakeCluster.
func newFakeCluster(t *testing.T) *fakeCluster {
	wlm := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	cluster := &fakeCluster{
		t:             t,
		wlm:           wlm,
		dynamicClient: dynamicfake.NewSimpleDynamicClient(k8sscheme.Scheme),
		subscribed:    make(chan struct{}),
		pendingPods:   make(map[types.UID]*corev1.Pod),
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
			Labels: map[string]string{spot.SpotNodeLabelKey: spot.SpotNodeLabelValue},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: spot.SpotNodeTaintKey, Value: spot.SpotNodeTaintValue, Effect: corev1.TaintEffectNoSchedule},
			},
		},
	})
}

// OnPodCreated registers a hook called for every admitted pod.
func (c *fakeCluster) OnPodCreated(hook admissionHook) {
	c.podCreatedHooks = append(c.podCreatedHooks, hook)
}

// OnPodDeleted registers a hook called for every deleted pod.
func (c *fakeCluster) OnPodDeleted(hook deletionHook) {
	c.podDeletedHooks = append(c.podDeletedHooks, hook)
}

// CreatePod runs all registered admission hooks on the pod then creates it as Pending.
func (c *fakeCluster) CreatePod(pod *corev1.Pod) {
	unmodifiedCopy := pod.DeepCopy()
	for _, hook := range c.podCreatedHooks {
		updated, err := hook(pod)
		require.NoError(c.t, err)
		if !updated {
			require.Equal(c.t, unmodifiedCopy, pod)
		}
	}
	c.createPending(pod)
}

func (c *fakeCluster) createPending(pod *corev1.Pod) {
	require.Empty(c.t, pod.Name)
	require.NotEmpty(c.t, pod.GenerateName)

	pod.Name = pod.GenerateName + randomSuffix(5)

	uid, err := uuid.NewRandom()
	require.NoError(c.t, err)

	pod.UID = types.UID(uid.String())
	pod.Status.Phase = corev1.PodPending

	c.mu.Lock()
	c.pendingPods[pod.UID] = pod
	c.mu.Unlock()

	async(c.wlm.Set, spot.CoreV1PodToWLM(pod))
}

// DeleteOwnerPods deletes all pods owned by the given ownerKind/namespace/ownerName.
func (c *fakeCluster) DeleteOwnerPods(ownerKind, namespace, ownerName string) {
	for _, pod := range c.ListOwnerPods(ownerKind, namespace, ownerName) {
		c.DeletePod(pod)
	}
}

// ListOwnerPods returns all pods owned by the given ownerKind/namespace/ownerName.
func (c *fakeCluster) ListOwnerPods(ownerKind, namespace, ownerName string) []*workloadmeta.KubernetesPod {
	var pods []*workloadmeta.KubernetesPod
	for _, pod := range c.wlm.ListKubernetesPods() {
		if pod.Namespace != namespace {
			continue
		}
		for _, owner := range pod.Owners {
			if owner.Kind == ownerKind && owner.Name == ownerName {
				pods = append(pods, pod)
				break
			}
		}
	}
	return pods
}

// EvictPodByName evicts a pod by namespace and name, simulating pod eviction for tests.
func (c *fakeCluster) EvictPodByName(namespace, name string) error {
	pod, err := c.wlm.GetKubernetesPodByName(name, namespace)
	if err != nil {
		return nil // pod not found, already gone
	}
	c.DeletePod(pod)
	return nil
}

// DeletePod removes a pod from the cluster.
func (c *fakeCluster) DeletePod(pod *workloadmeta.KubernetesPod) {
	corePod := wlmPodToCorePod(pod)
	for _, hook := range c.podDeletedHooks {
		hook(corePod)
	}

	podCopy := pod.DeepCopy().(*workloadmeta.KubernetesPod)
	podCopy.Phase = string(corev1.PodSucceeded)

	async(c.wlm.Set, podCopy)
}

// WLM returns the underlying workloadmeta mock store.
func (c *fakeCluster) WLM() workloadmetamock.Mock {
	return c.wlm
}

func (c *fakeCluster) DynamicClient() dynamic.Interface {
	return c.dynamicClient
}

// AssertOwnerPods checks that all pods owned by ownerKind/namespace/ownerName eventually satisfy check.
func (c *fakeCluster) AssertOwnerPods(ownerKind, namespace, ownerName string, check func(wlm []*workloadmeta.KubernetesPod) bool) {
	const assertWaitFor = 1 * time.Second
	require.Eventuallyf(c.t, func() bool {
		return check(c.ListOwnerPods(ownerKind, namespace, ownerName))
	}, assertWaitFor, assertWaitFor/10, "%s %s/%s", ownerKind, namespace, ownerName)
}

// runPodScheduler simulates a Kubernetes scheduler for testing.
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
					switch corev1.PodPhase(pod.Phase) {
					case corev1.PodPending:
						c.trySchedule(pod.ID)
					case corev1.PodSucceeded, corev1.PodFailed:
						async(c.wlm.Unset, pod)
					}
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

	async(c.wlm.Set, spot.CoreV1PodToWLM(pod))
}

func (c *fakeCluster) CreateDeployment(namespace, name string, labels, annotations map[string]string, replicas int) *fakeDeployment {
	// Use "deployment": name as the pod selector so the pod fetcher can match pods by label.
	podSelector := map[string]string{"deployment": name}

	u := &unstructured.Unstructured{}
	u.SetAPIVersion("apps/v1")
	u.SetKind("Deployment")
	u.SetName(name)
	u.SetNamespace(namespace)
	u.SetLabels(labels)
	u.SetAnnotations(annotations)
	// Set spec.selector.matchLabels so the pod fetcher can extract the selector on opt-in.
	require.NoError(c.t, unstructured.SetNestedStringMap(u.Object, podSelector, "spec", "selector", "matchLabels"))

	d := &fakeDeployment{
		cluster:     c,
		namespace:   namespace,
		name:        name,
		podSelector: podSelector,
	}

	_, err := c.dynamicClient.Resource(deploymentsGVR).Namespace(namespace).Create(context.Background(), u, metav1.CreateOptions{})
	require.NoError(c.t, err)

	d.rolloutWithDelay(replicas)
	return d
}

// ReplicaSet returns the name of the current ReplicaSet.
func (d *fakeDeployment) ReplicaSet() string {
	return d.existingReplicaSet
}

// Rollout simulates a Deployment rollout by creating a new ReplicaSet with the given replicas,
// then deleting all pods of the previous ReplicaSet (if any).
// Returns the name of the new ReplicaSet.
func (d *fakeDeployment) Rollout(labels, annotations map[string]string, replicas int) string {
	u, err := d.cluster.dynamicClient.Resource(deploymentsGVR).Namespace(d.namespace).Get(context.Background(), d.name, metav1.GetOptions{})
	require.NoError(d.cluster.t, err)

	if labels != nil {
		lbl := u.GetLabels()
		if lbl == nil {
			lbl = make(map[string]string)
		}
		maps.Copy(lbl, labels)
		u.SetLabels(lbl)
	}
	if annotations != nil {
		ann := u.GetAnnotations()
		if ann == nil {
			ann = make(map[string]string)
		}
		maps.Copy(ann, annotations)
		u.SetAnnotations(ann)
	}
	_, err = d.cluster.dynamicClient.Resource(deploymentsGVR).Namespace(d.namespace).Update(context.Background(), u, metav1.UpdateOptions{})
	require.NoError(d.cluster.t, err)

	return d.rolloutWithDelay(replicas)
}

// UpdateMetadata merges the given labels and annotations into the Deployment without creating new pods.
func (d *fakeDeployment) UpdateMetadata(newLabels, newAnnotations map[string]string) {
	u, err := d.cluster.dynamicClient.Resource(deploymentsGVR).Namespace(d.namespace).Get(context.Background(), d.name, metav1.GetOptions{})
	require.NoError(d.cluster.t, err)

	if len(newLabels) > 0 {
		lbl := u.GetLabels()
		if lbl == nil {
			lbl = make(map[string]string)
		}
		maps.Copy(lbl, newLabels)
		u.SetLabels(lbl)
	}

	if len(newAnnotations) > 0 {
		ann := u.GetAnnotations()
		if ann == nil {
			ann = make(map[string]string)
		}
		maps.Copy(ann, newAnnotations)
		u.SetAnnotations(ann)
	}

	_, err = d.cluster.dynamicClient.Resource(deploymentsGVR).Namespace(d.namespace).Update(context.Background(), u, metav1.UpdateOptions{})
	require.NoError(d.cluster.t, err)
}

// RemoveLabels removes the given label keys from the Deployment without creating new pods.
func (d *fakeDeployment) RemoveLabels(keys ...string) {
	u, err := d.cluster.dynamicClient.Resource(deploymentsGVR).Namespace(d.namespace).Get(context.Background(), d.name, metav1.GetOptions{})
	require.NoError(d.cluster.t, err)
	lbl := u.GetLabels()
	for _, k := range keys {
		delete(lbl, k)
	}
	u.SetLabels(lbl)

	_, err = d.cluster.dynamicClient.Resource(deploymentsGVR).Namespace(d.namespace).Update(context.Background(), u, metav1.UpdateOptions{})
	require.NoError(d.cluster.t, err)
}

// Delete deletes the Deployment and all its pods.
func (d *fakeDeployment) Delete() {
	if d.existingReplicaSet != "" {
		d.cluster.DeleteOwnerPods(kubernetes.ReplicaSetKind, d.namespace, d.existingReplicaSet)
	}
	err := d.cluster.dynamicClient.Resource(deploymentsGVR).Namespace(d.namespace).Delete(context.Background(), d.name, metav1.DeleteOptions{})
	require.NoError(d.cluster.t, err)
}

func (d *fakeDeployment) rolloutWithDelay(replicas int) string {
	// TODO: fixme
	// Let workload config store pick up the change so test do not rely on rebalancing.
	// Real Deployment creates ReplicaSet which in turn creates pods.
	time.Sleep(100 * time.Millisecond)

	// A new ReplicaSet created
	newReplicaSet := replicaSetName(d.name)
	for range replicas {
		d.cluster.CreatePod(newPod(d.namespace, kubernetes.ReplicaSetKind, newReplicaSet, d.podSelector))
	}
	// Existing ReplicaSet is scaled down
	if d.existingReplicaSet != "" {
		d.cluster.DeleteOwnerPods(kubernetes.ReplicaSetKind, d.namespace, d.existingReplicaSet)
	}
	d.existingReplicaSet = newReplicaSet
	return newReplicaSet
}

func async(f func(workloadmeta.Entity), e workloadmeta.Entity) {
	go func() {
		time.Sleep(time.Duration(10+rand.N(40)) * time.Millisecond)
		f(e)
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
func replicaSetName(deployment string) string {
	return deployment + "-" + randomSuffix(10)
}

// randomSuffix adds suffix that uses [kubernetes.KubeAllowedEncodeStringAlphaNums] characters so that
// ParseDeploymentForReplicaSet and ParseDeploymentForPodName correctly resolve the name back to the deployment.
func randomSuffix(n int) string {
	var b strings.Builder
	const chars = "bcdfghjklmnpqrstvwxz2456789"
	for range n {
		b.WriteByte(chars[rand.N(len(chars))])
	}
	return b.String()
}

// newPod builds a corev1.Pod with the given owner and labels.
func newPod(namespace, ownerKind, ownerName string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: ownerName + "-",
			Labels:       maps.Clone(labels),
			OwnerReferences: []metav1.OwnerReference{
				{Kind: ownerKind, Name: ownerName},
			},
		},
	}
}

// wlmPodToCorePod reconstructs a minimal corev1.Pod from a workloadmeta KubernetesPod.
func wlmPodToCorePod(pod *workloadmeta.KubernetesPod) *corev1.Pod {
	owners := make([]metav1.OwnerReference, 0, len(pod.Owners))
	for _, owner := range pod.Owners {
		owners = append(owners, metav1.OwnerReference{Kind: owner.Kind, Name: owner.Name})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            pod.Name,
			Namespace:       pod.Namespace,
			UID:             types.UID(pod.ID),
			Annotations:     maps.Clone(pod.Annotations),
			Labels:          maps.Clone(pod.Labels),
			OwnerReferences: owners,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPhase(pod.Phase),
		},
	}
}
