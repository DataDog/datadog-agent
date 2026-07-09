// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package com_datadoghq_kubernetes_kubeactions

import (
	"testing"

	"github.com/stretchr/testify/assert"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestGetAction(t *testing.T) {
	// A nil component is fine here: GetAction only looks up handlers, it does
	// not invoke them.
	b := NewKubernetesKubeActions(nil)

	for _, name := range []string{
		kubeactions.ActionNameDeletePod,
		kubeactions.ActionNameRestartDeployment,
		kubeactions.ActionNamePatchDeployment,
		kubeactions.ActionNameRollbackDeployment,
		kubeactions.ActionNameGetResource,
	} {
		assert.NotNil(t, b.GetAction(name), "expected a handler for %q", name)
	}

	// snake_case (the EVP action_type form) must NOT resolve — dispatch is by
	// the camelCase action name.
	assert.Nil(t, b.GetAction("delete_pod"))
	assert.Nil(t, b.GetAction("unknown_action"))
}

func TestNewReport(t *testing.T) {
	ref := kubeactions.ResourceRef{
		Kind:       "Pod",
		Name:       "my-pod",
		Namespace:  "default",
		ResourceID: "pod-uid",
	}

	t.Run("populates from task", func(t *testing.T) {
		task := &types.Task{}
		task.Data.Attributes = &types.Attributes{OrgId: 7, JobId: "job-9"}

		r := newReport(kubeactions.ActionTypeDeletePod, ref, task)
		assert.Equal(t, kubeactions.ActionTypeDeletePod, r.ActionType)
		assert.Equal(t, int64(7), r.OrgID)
		assert.Equal(t, "job-9", r.ActionID)
		assert.Equal(t, "Pod", r.ResourceKind)
		assert.Equal(t, "my-pod", r.ResourceName)
		assert.Equal(t, "default", r.ResourceNamespace)
		assert.Equal(t, "pod-uid", r.ResourceID)
	})

	t.Run("nil task is safe", func(t *testing.T) {
		r := newReport(kubeactions.ActionTypeGetResource, ref, nil)
		assert.Equal(t, kubeactions.ActionTypeGetResource, r.ActionType)
		assert.Zero(t, r.OrgID)
		assert.Empty(t, r.ActionID)
		assert.Equal(t, "Pod", r.ResourceKind)
	})

	t.Run("nil attributes is safe", func(t *testing.T) {
		r := newReport(kubeactions.ActionTypeDeletePod, ref, &types.Task{})
		assert.Zero(t, r.OrgID)
		assert.Empty(t, r.ActionID)
	})
}
