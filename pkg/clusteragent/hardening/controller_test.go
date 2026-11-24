// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
)

var deploymentsGVR = appsv1.SchemeGroupVersion.WithResource("deployments")

// testDeployment returns a Deployment whose rollout completed, with one
// probed container.
func testDeployment(name string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "shop", UID: types.UID("uid-" + name), Generation: 1},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(2)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "app", Image: name + ":1", ReadinessProbe: &corev1.Probe{}},
				}},
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1, Replicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2,
			Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"}},
		},
	}
}

// marked returns d with the policy marking its container, as the leader left it.
func marked(d *appsv1.Deployment) *appsv1.Deployment {
	d.Spec.Template.Labels[EnabledLabel] = "true"
	d.Spec.Template.Annotations = map[string]string{PoliciesAnnotation: `{"hp-caps":["app"]}`}
	return d
}

// replicaSet returns d's ReplicaSet running template at revision rev.
func replicaSet(d *appsv1.Deployment, template corev1.PodTemplateSpec, rev int) *appsv1.ReplicaSet {
	name := fmt.Sprintf("%s-%d00", d.Name, rev) // a suffix ParseDeploymentForReplicaSet accepts
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: d.Namespace, UID: types.UID("uid-" + name), Labels: d.Spec.Selector.MatchLabels,
			Annotations:     map[string]string{"deployment.kubernetes.io/revision": strconv.Itoa(rev)},
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: d.Name, UID: d.UID, Controller: ptr.To(true)}},
		},
		Spec: appsv1.ReplicaSetSpec{Template: *template.DeepCopy()},
	}
}

// hardenedPod returns a pod of rs on which the webhook applied the policy.
func hardenedPod(rs *appsv1.ReplicaSet, status corev1.ContainerStatus) *corev1.Pod {
	status.Name = "app"
	annotations := map[string]string{AppliedAnnotation: `{"hp-caps/app":"applied"}`}
	for k, v := range rs.Spec.Template.Annotations {
		annotations[k] = v
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: rs.Name + "-x", Namespace: rs.Namespace, Labels: rs.Spec.Template.Labels, Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: rs.Name, UID: rs.UID, Controller: ptr.To(true)}},
		},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{status}},
	}
}

func restartedAt(t time.Time) corev1.ContainerStatus {
	return corev1.ContainerStatus{RestartCount: 1, LastTerminationState: corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1, FinishedAt: metav1.NewTime(t)},
	}}
}

type harness struct {
	t           *testing.T
	client      *fake.Clientset
	store       *Store
	c           *Controller
	deployments cache.Indexer
	pods        cache.Indexer
}

// newHarness returns a leader whose fake API server, like a real one,
// persists nothing for dry-run patches.
func newHarness(t *testing.T, objs ...runtime.Object) *harness {
	client := fake.NewClientset(objs...)
	client.PrependReactor("patch", "deployments", func(a k8stesting.Action) (bool, runtime.Object, error) {
		patch := a.(k8stesting.PatchActionImpl)
		if len(patch.PatchOptions.DryRun) > 0 {
			obj, err := client.Tracker().Get(deploymentsGVR, patch.Namespace, patch.Name)
			return true, obj, err
		}
		return false, nil, nil
	})
	h := &harness{
		t:           t,
		client:      client,
		store:       NewStore(testCluster),
		deployments: cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}),
		pods:        cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}),
	}
	for _, o := range objs {
		if p, ok := o.(*corev1.Pod); ok {
			require.NoError(t, h.pods.Add(p))
		}
	}
	h.c = NewController(client, appslisters.NewDeploymentLister(h.deployments), corelisters.NewPodLister(h.pods), h.store, func() bool { return true })
	h.c.syncWait = 0
	h.c.now = func() time.Time { return testAddedAt.Add(24 * time.Hour) }
	return h
}

// reconcile runs one pass on the Deployments the fake API server holds.
func (h *harness) reconcile() {
	list, err := h.client.AppsV1().Deployments("").List(context.Background(), metav1.ListOptions{})
	require.NoError(h.t, err)
	items := make([]any, 0, len(list.Items))
	for i := range list.Items {
		items = append(items, &list.Items[i])
	}
	require.NoError(h.t, h.deployments.Replace(items, ""))
	h.c.reconcile(context.Background())
}

func (h *harness) get(name string) *appsv1.Deployment {
	d, err := h.client.AppsV1().Deployments("shop").Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(h.t, err)
	return d
}

func (h *harness) marks(name string) Marks {
	return ParseMarks(h.get(name).Spec.Template.Annotations[PoliciesAnnotation])
}

func (h *harness) exclusions(name string) []Exclusion {
	return parseExclusions(h.get(name).Annotations[ExcludedAnnotation])
}

func TestMarksAFirstBatch(t *testing.T) {
	h := newHarness(t, testDeployment("a"), testDeployment("b"), testDeployment("c"))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()

	assert.Equal(t, []string{"app"}, h.marks("a")["hp-caps"])
	assert.Equal(t, "true", h.get("a").Spec.Template.Labels[EnabledLabel])
	assert.Equal(t, []string{"app"}, h.marks("b")["hp-caps"])
	assert.Empty(t, h.marks("c"), "the first batch is 2")

	h.reconcile()
	assert.Empty(t, h.marks("c"), "the batch's pods do not carry the control yet")
}

func TestWaitsForExceptionsAndReplicas(t *testing.T) {
	h := newHarness(t, testDeployment("a"))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt))
	h.reconcile()
	assert.Empty(t, h.marks("a"), "no exceptions generation is loaded")

	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.c.syncWait = time.Hour
	h.reconcile()
	assert.Empty(t, h.marks("a"), "replicas may not have loaded the policy yet")
}

func TestTargetSelection(t *testing.T) {
	excluded := testDeployment("excluded")
	excluded.Annotations = map[string]string{ExcludedAnnotation: `[{"policy":"hp-caps","reason":"rolled_back","at":"2026-10-01T00:00:00Z"}]`}
	optedOut := testDeployment("opted-out")
	optedOut.Annotations = map[string]string{OptOutAnnotation: "true"}
	noProbe := testDeployment("no-probe")
	noProbe.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
	declared := testDeployment("declared")
	declared.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}
	recreate := testDeployment("recreate")
	recreate.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType

	h := newHarness(t, testDeployment("exempt"), excluded, optedOut, noProbe, declared, recreate)
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(`"workloads": [{"kind": "Deployment", "namespace": "shop", "name": "exempt", "reason": "user"}]`))
	h.reconcile()
	h.reconcile()

	for _, name := range []string{"exempt", "excluded", "opted-out", "no-probe", "declared", "recreate"} {
		assert.Empty(t, h.marks(name), name)
	}
	assert.Contains(t, h.c.status[statusKey{"hp-caps", "shop/no-probe"}], "rejected: app: no readiness or startup probe")
	assert.Contains(t, h.c.status[statusKey{"hp-caps", "shop/recreate"}], "rejected: deployment uses the Recreate strategy")
	assert.NotContains(t, h.c.status, statusKey{"hp-caps", "shop/exempt"})
}

func TestRevertsAFailedRolloutItStarted(t *testing.T) {
	before := testDeployment("web")
	d := marked(testDeployment("web"))
	d.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Reason: "ProgressDeadlineExceeded"}}
	d.Status.UpdatedReplicas = 1
	h := newHarness(t, d, replicaSet(d, before.Spec.Template, 1), replicaSet(d, d.Spec.Template, 2))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()

	got := h.get("web")
	assert.NotContains(t, got.Spec.Template.Annotations, PoliciesAnnotation)
	assert.NotContains(t, got.Spec.Template.Labels, EnabledLabel)
	require.Len(t, h.exclusions("web"), 1)
	assert.Equal(t, Exclusion{Policy: "hp-caps", Containers: []string{"app"}, Reason: ReasonRolledBack, At: h.c.now()}, h.exclusions("web")[0])
	assert.Contains(t, h.c.status[statusKey{"hp-caps", "shop/web"}], "rolled_back")
}

func TestCustomerRolloutFailureIsDegraded(t *testing.T) {
	ours := marked(testDeployment("web"))
	d := marked(testDeployment("web"))
	d.Spec.Template.Spec.Containers[0].Image = "web:2"
	d.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Reason: "ProgressDeadlineExceeded"}}
	h := newHarness(t, d, replicaSet(d, testDeployment("web").Spec.Template, 1), replicaSet(d, ours.Spec.Template, 2), replicaSet(d, d.Spec.Template, 3))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()

	assert.Equal(t, []string{"app"}, h.marks("web")["hp-caps"])
	assert.Contains(t, h.c.status[statusKey{"hp-caps", "shop/web"}], "degraded")
}

func TestCrashAfterSuccessPausesUntilTheClusterIsAddedAgain(t *testing.T) {
	d := marked(testDeployment("web"))
	rs := replicaSet(d, d.Spec.Template, 2)
	crash := testAddedAt.Add(time.Hour)
	h := newHarness(t, d, testDeployment("next"), rs, hardenedPod(rs, restartedAt(crash)))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()

	assert.Equal(t, []string{"app"}, h.marks("web")["hp-caps"], "only a person removes a control after its rollout succeeded")
	assert.Contains(t, h.c.status[statusKey{"hp-caps", "shop/web"}], "degraded")
	assert.Contains(t, h.c.status[statusKey{policy: "hp-caps"}], "paused")
	assert.Empty(t, h.marks("next"))

	setRC(t, h.store, testPolicy(StateEnforce, crash.Add(time.Minute)), testExceptions(""))
	h.reconcile()
	assert.NotContains(t, h.c.status, statusKey{policy: "hp-caps"})
	assert.Equal(t, []string{"app"}, h.marks("next")["hp-caps"])
}

func TestExemptingADegradedDeploymentResumes(t *testing.T) {
	d := marked(testDeployment("web"))
	d.Status.AvailableReplicas = 1
	rs := replicaSet(d, d.Spec.Template, 2)
	h := newHarness(t, d, testDeployment("next"), rs, hardenedPod(rs, restartedAt(testAddedAt.Add(time.Hour))))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(`"workloads": [{"kind": "Deployment", "namespace": "shop", "name": "web", "reason": "user"}]`))
	h.reconcile()
	assert.Equal(t, Marks{"hp-caps": {}}, h.marks("web"))

	h.reconcile()
	assert.NotContains(t, h.c.status, statusKey{policy: "hp-caps"})
	assert.Equal(t, []string{"app"}, h.marks("next")["hp-caps"])
}

func TestRevertsPauseTheCluster(t *testing.T) {
	revert := fmt.Sprintf(`{"policy":"hp-caps","reason":"rolled_back","at":%q}`, testAddedAt.Add(time.Hour).Format(time.RFC3339))
	a, b := testDeployment("a"), testDeployment("b")
	a.Annotations = map[string]string{ExcludedAnnotation: "[" + revert + "]"}
	b.Annotations = map[string]string{ExcludedAnnotation: "[" + revert + "]"}
	h := newHarness(t, a, b, testDeployment("c"))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()
	assert.Empty(t, h.marks("c"))
	assert.Contains(t, h.c.status[statusKey{policy: "hp-caps"}], "paused: 2 rollouts were reverted")

	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt.Add(2*time.Hour)), testExceptions(""))
	h.reconcile()
	assert.Equal(t, []string{"app"}, h.marks("c")["hp-caps"])
}

func TestRevertsInOnePassPauseBeforeTheNextMark(t *testing.T) {
	var objs []runtime.Object
	for _, name := range []string{"a", "b"} {
		before := testDeployment(name)
		d := marked(testDeployment(name))
		d.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Reason: "ProgressDeadlineExceeded"}}
		d.Status.UpdatedReplicas = 1
		objs = append(objs, d, replicaSet(d, before.Spec.Template, 1), replicaSet(d, d.Spec.Template, 2))
	}
	h := newHarness(t, append(objs, testDeployment("c"))...)
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()

	assert.Len(t, h.exclusions("a"), 1)
	assert.Len(t, h.exclusions("b"), 1)
	assert.Empty(t, h.marks("c"), "the pass that reaches the pause threshold marks nothing")
	assert.Contains(t, h.c.status[statusKey{policy: "hp-caps"}], "paused: 2 rollouts were reverted")
}

func TestRecordsAMarkSomeoneRemoved(t *testing.T) {
	d := testDeployment("web")
	h := newHarness(t, d, replicaSet(d, d.Spec.Template, 1), replicaSet(d, marked(testDeployment("web")).Spec.Template, 2), replicaSet(d, d.Spec.Template, 3))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()
	h.reconcile()

	assert.Empty(t, h.marks("web"))
	require.Len(t, h.exclusions("web"), 1)
	assert.Equal(t, ReasonRemoved, h.exclusions("web")[0].Reason)
}

func TestRemarksAfterItsOwnUnmark(t *testing.T) {
	d := testDeployment("web")
	d.Spec.Template.Annotations = map[string]string{PoliciesAnnotation: `{"hp-caps":[]}`}
	h := newHarness(t, d, replicaSet(d, marked(testDeployment("web")).Spec.Template, 1), replicaSet(d, d.Spec.Template, 2))
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()

	assert.Equal(t, []string{"app"}, h.marks("web")["hp-caps"])
	assert.Empty(t, h.exclusions("web"))
}

func TestOptOutRemovesEveryMark(t *testing.T) {
	d := marked(testDeployment("web"))
	d.Annotations = map[string]string{OptOutAnnotation: "true"}
	h := newHarness(t, d)
	h.reconcile()

	got := h.get("web")
	assert.NotContains(t, got.Spec.Template.Annotations, PoliciesAnnotation)
	require.Len(t, h.exclusions("web"), 1)
	assert.Equal(t, ReasonRemoved, h.exclusions("web")[0].Reason)
}

func TestExemptions(t *testing.T) {
	stopped := marked(testDeployment("stopped"))
	stopped.Namespace = "pay"
	exempt := marked(testDeployment("exempt"))
	h := newHarness(t, stopped, exempt)
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(`
		"workloads": [{"kind": "Deployment", "namespace": "shop", "name": "exempt", "reason": "user"}],
		"emergency_stops": [{"namespaces": ["pay"]}]`))
	h.reconcile()

	got, err := h.client.AppsV1().Deployments("pay").Get(context.Background(), "stopped", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, Marks{"hp-caps": {}}, ParseMarks(got.Spec.Template.Annotations[PoliciesAnnotation]))
	assert.NotContains(t, got.Spec.Template.Labels, EnabledLabel)
	assert.Equal(t, Marks{"hp-caps": {}}, h.marks("exempt"))
	assert.Contains(t, h.c.status[statusKey{"hp-caps", "shop/exempt"}], "exempted")
}

func TestStoppedPolicyUnmarks(t *testing.T) {
	h := newHarness(t, marked(testDeployment("web")))
	setRC(t, h.store, testPolicy(StateStopped, testAddedAt))
	h.reconcile()
	assert.Equal(t, Marks{"hp-caps": {}}, h.marks("web"))
}

func TestBatchGrowsOnceItsPodsCarryTheControl(t *testing.T) {
	var objs []runtime.Object
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		objs = append(objs, testDeployment(name))
	}
	h := newHarness(t, objs...)
	setRC(t, h.store, testPolicy(StateEnforce, testAddedAt), testExceptions(""))
	h.reconcile()
	for _, name := range []string{"a", "b"} {
		rs := replicaSet(h.get(name), h.get(name).Spec.Template, 2)
		require.NoError(t, h.pods.Add(hardenedPod(rs, corev1.ContainerStatus{})))
	}
	h.reconcile()
	h.reconcile()
	n := 0
	for _, name := range []string{"c", "d", "e", "f", "g"} {
		if len(h.marks(name)["hp-caps"]) > 0 {
			n++
		}
	}
	assert.Equal(t, 4, n, "the second batch doubles")
}
