// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package issues

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

const testPodName = "dd-agent-abc12"

// Only the Kubernetes selfident can resolve a DaemonSet, so this is the one
// assertion the no-op variant cannot cover: when a uid is available it must win
// over the host id, which is what collapses one cluster-distributed template
// failure into a single backend issue instead of one per node.
func TestIssueDiscriminator_DeploymentIDWinsOverHostID(t *testing.T) {
	t.Setenv("DD_POD_NAME", testPodName)
	env.SetFeatures(t, env.Kubernetes)

	mockStore := workloadmetaimpl.NewWorkloadMetaMock(workloadmetaimpl.Dependencies{
		Lc:     compdef.NewTestLifecycle(t),
		Log:    logmock.New(t),
		Config: config.NewMock(t),
		Params: workloadmeta.NewParams(),
	})
	mockStore.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "self-pod-uid",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: testPodName,
			// Mirror what selfident actually queries rather than assuming
			// "default": GetMyNamespace falls back to "default" only when the
			// serviceaccount namespace file is absent, which isn't true on a
			// runner that itself lives in a Kubernetes pod.
			Namespace: namespace.GetMyNamespace(),
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
		},
	})

	assert.Equal(t, "daemonset-uid-123", IssueDiscriminator(selfident.New(mockStore), "some-host-id"))
}
