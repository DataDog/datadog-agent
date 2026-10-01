// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const eventuallyTimeout = 2 * time.Second
const eventuallyInterval = 10 * time.Millisecond

func mockedWorkloadmeta(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func podWithContainer(name string, uid types.UID, containerID string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-namespace", UID: uid},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c1"}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "c1", ContainerID: "docker://" + containerID},
			},
		},
	}
}

// TestPodStore_AddDelete verifies that Add sets both the pod and its derived
// container, and that Delete unsets both again.
func TestPodStore_AddDelete(t *testing.T) {
	wlm := mockedWorkloadmeta(t)
	store := newPodStore(wlm, "test-node", false)

	pod := podWithContainer("test-pod", "pod-uid", "container-id")

	require.NoError(t, store.Add(pod))

	require.Eventually(t, func() bool {
		_, err := wlm.GetKubernetesPod("pod-uid")
		return err == nil
	}, eventuallyTimeout, eventuallyInterval)

	_, err := wlm.GetContainer("container-id")
	require.NoError(t, err)

	require.NoError(t, store.Delete(pod))

	require.Eventually(t, func() bool {
		_, err := wlm.GetKubernetesPod("pod-uid")
		return err != nil
	}, eventuallyTimeout, eventuallyInterval)

	_, err = wlm.GetContainer("container-id")
	require.Error(t, err)
}

// TestPodStore_UpdateContainerRestart verifies that, when a container within
// a pod is replaced by the runtime under a new ID (e.g. on restart), Update
// unsets the old container entity rather than leaking it.
func TestPodStore_UpdateContainerRestart(t *testing.T) {
	wlm := mockedWorkloadmeta(t)
	store := newPodStore(wlm, "test-node", false)

	pod := podWithContainer("test-pod", "pod-uid", "old-container-id")
	require.NoError(t, store.Add(pod))

	require.Eventually(t, func() bool {
		_, err := wlm.GetContainer("old-container-id")
		return err == nil
	}, eventuallyTimeout, eventuallyInterval)

	restarted := podWithContainer("test-pod", "pod-uid", "new-container-id")
	require.NoError(t, store.Update(restarted))

	require.Eventually(t, func() bool {
		_, err := wlm.GetContainer("new-container-id")
		return err == nil
	}, eventuallyTimeout, eventuallyInterval)

	_, err := wlm.GetContainer("old-container-id")
	require.Error(t, err)

	_, err = wlm.GetKubernetesPod("pod-uid")
	require.NoError(t, err)
}

// TestPodStore_Update verifies that Update (== Add) refreshes the pod's
// fields in place.
func TestPodStore_Update(t *testing.T) {
	wlm := mockedWorkloadmeta(t)
	store := newPodStore(wlm, "test-node", false)

	pod := podWithContainer("test-pod", "pod-uid", "container-id")
	pod.Labels = map[string]string{"a": "b"}
	require.NoError(t, store.Add(pod))

	require.Eventually(t, func() bool {
		_, err := wlm.GetKubernetesPod("pod-uid")
		return err == nil
	}, eventuallyTimeout, eventuallyInterval)

	updated := pod.DeepCopy()
	updated.Labels["a"] = "c"
	require.NoError(t, store.Update(updated))

	require.Eventually(t, func() bool {
		p, err := wlm.GetKubernetesPod("pod-uid")
		return err == nil && p.Labels["a"] == "c"
	}, eventuallyTimeout, eventuallyInterval)
}

// TestPodStore_Replace verifies that Replace unsets pods (and their derived
// containers) that dropped out of the list, while keeping/adding the rest.
func TestPodStore_Replace(t *testing.T) {
	wlm := mockedWorkloadmeta(t)
	store := newPodStore(wlm, "test-node", false)

	pod1 := podWithContainer("pod1", "uid1", "container1")
	pod2 := podWithContainer("pod2", "uid2", "container2")

	require.NoError(t, store.Add(pod1))
	require.Eventually(t, func() bool {
		_, err := wlm.GetKubernetesPod("uid1")
		return err == nil
	}, eventuallyTimeout, eventuallyInterval)

	require.NoError(t, store.Replace([]interface{}{pod2}, ""))

	require.Eventually(t, func() bool {
		_, err1 := wlm.GetKubernetesPod("uid1")
		_, err2 := wlm.GetKubernetesPod("uid2")
		return err1 != nil && err2 == nil
	}, eventuallyTimeout, eventuallyInterval)

	_, err := wlm.GetContainer("container1")
	require.Error(t, err)
	_, err = wlm.GetContainer("container2")
	require.NoError(t, err)
}

// TestPodStore_ReplaceContainerRestart verifies that Replace unsets a
// container that dropped out even for a pod that itself is present in both
// the previous and the new list (e.g. the runtime restarted one of its
// containers under a new ID between two Replace calls).
func TestPodStore_ReplaceContainerRestart(t *testing.T) {
	wlm := mockedWorkloadmeta(t)
	store := newPodStore(wlm, "test-node", false)

	pod := podWithContainer("test-pod", "pod-uid", "old-container-id")
	require.NoError(t, store.Replace([]interface{}{pod}, ""))

	require.Eventually(t, func() bool {
		_, err := wlm.GetContainer("old-container-id")
		return err == nil
	}, eventuallyTimeout, eventuallyInterval)

	restarted := podWithContainer("test-pod", "pod-uid", "new-container-id")
	require.NoError(t, store.Replace([]interface{}{restarted}, ""))

	require.Eventually(t, func() bool {
		_, err := wlm.GetContainer("new-container-id")
		return err == nil
	}, eventuallyTimeout, eventuallyInterval)

	_, err := wlm.GetContainer("old-container-id")
	require.Error(t, err)

	_, err = wlm.GetKubernetesPod("pod-uid")
	require.NoError(t, err)
}
