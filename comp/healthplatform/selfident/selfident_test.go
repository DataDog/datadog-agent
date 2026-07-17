// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package selfident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const testPodName = "dd-agent-abc12"
const testNamespace = "default"

func newMockStore(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func setSelfPod(mockStore workloadmetamock.Mock, owners []workloadmeta.KubernetesPodOwner) {
	mockStore.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "self-pod-uid",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      testPodName,
			Namespace: testNamespace,
		},
		Owners: owners,
	})
}

func TestDeploymentID_ResolvesFromDaemonSetOwner(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	s := New(option.New[workloadmeta.Component](mockStore))

	assert.Equal(t, "daemonset-uid-123", s.DeploymentID())
}

func TestDeploymentID_NoDaemonSetOwner(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "ReplicaSet", Name: "some-rs", ID: "rs-uid"},
	})

	s := New(option.New[workloadmeta.Component](mockStore))

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_PodNotFound(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)

	s := New(option.New[workloadmeta.Component](mockStore))

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_NoPodNameEnvVar(t *testing.T) {
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	s := New(option.New[workloadmeta.Component](mockStore))

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_NoWorkloadmeta(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)

	s := New(option.None[workloadmeta.Component]())

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_ResolvedOnce(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	s := New(option.New[workloadmeta.Component](mockStore))
	assert.Equal(t, "daemonset-uid-123", s.DeploymentID())

	mockStore.Unset(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "self-pod-uid"},
	})

	// Cached from the first resolution; does not re-query workloadmeta.
	assert.Equal(t, "daemonset-uid-123", s.DeploymentID())
}

func TestIssueDiscriminator_PrefersDeploymentID(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	s := New(option.New[workloadmeta.Component](mockStore))

	assert.Equal(t, "daemonset-uid-123", s.IssueDiscriminator("some-host-id"))
}

func TestIssueDiscriminator_FallsBackToHostID(t *testing.T) {
	s := New(option.None[workloadmeta.Component]())

	assert.Equal(t, "some-host-id", s.IssueDiscriminator("some-host-id"))
}
