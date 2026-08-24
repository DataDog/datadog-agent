// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && kubeapiserver

package storeimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

const testSelfPodName = "dd-agent-abc12"

// TestReportIssueEnrichesWithClusterIdentity verifies that enrichWithClusterIdentity
// actually stamps a resolved deployment_id into Extra/Tags on ReportIssue, since every
// other test in this file resolves selfIdent against a nil workloadmeta (always empty), which
// left this enrichment path itself untested.
func TestReportIssueEnrichesWithClusterIdentity(t *testing.T) {
	// selfident reads the pod name from DD_POD_NAME directly; there's no exported
	// constant to reference from this package.
	t.Setenv("DD_POD_NAME", testSelfPodName)
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
			Name:      testSelfPodName,
			Namespace: namespace.GetMyNamespace(),
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
		},
	})

	h := newTestStore(t)
	h.selfIdent = selfident.New(mockStore)

	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))

	_, issues := h.GetAllIssues()
	issue := issues["t:id"]
	require.NotNil(t, issue)

	require.NotNil(t, issue.Extra)
	assert.Equal(t, "daemonset-uid-123", issue.Extra.Fields["deployment_id"].GetStringValue())
	assert.Contains(t, issue.Tags, "deployment_id:daemonset-uid-123")
}
