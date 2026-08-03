// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"testing"

	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentOwnerForPod(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		got, ok := deploymentOwnerForPod(&pbgo.PodLanguageDetails{
			Namespace: "default",
			Name:      "nginx-7d4f8b9c6-2x9qd",
			Ownerref:  &pbgo.KubeOwnerInfo{Kind: "ReplicaSet", Name: "nginx-7d4f8b9c6"},
		})
		require.True(t, ok)
		want := langUtil.NewNamespacedOwnerReference("apps/v1", langUtil.KindDeployment, "nginx", "default")
		assert.Equal(t, want, got)
	})

	t.Run("reject owner that is not a ReplicaSet (e.g. direct Deployment)", func(t *testing.T) {
		_, ok := deploymentOwnerForPod(&pbgo.PodLanguageDetails{
			Namespace: "default",
			Name:      "nginx-7d4f8b9c6-2x9qd",
			Ownerref:  &pbgo.KubeOwnerInfo{Kind: "Deployment", Name: "nginx"},
		})
		assert.False(t, ok)
	})

	t.Run("reject missing owner reference", func(t *testing.T) {
		_, ok := deploymentOwnerForPod(&pbgo.PodLanguageDetails{
			Namespace: "default",
			Name:      "nginx-7d4f8b9c6-2x9qd",
		})
		assert.False(t, ok)
	})

	t.Run("reject when pod name does not match the owner reference (forged pod name)", func(t *testing.T) {
		_, ok := deploymentOwnerForPod(&pbgo.PodLanguageDetails{
			Namespace: "default",
			Name:      "forged-owner-pod",
			Ownerref:  &pbgo.KubeOwnerInfo{Kind: "ReplicaSet", Name: "nginx-7d4f8b9c6"},
		})
		assert.False(t, ok)
	})

	t.Run("reject when pod name matches a different deployment", func(t *testing.T) {
		_, ok := deploymentOwnerForPod(&pbgo.PodLanguageDetails{
			Namespace: "default",
			Name:      "victim-7d4f8b9c6-2x9qd",
			Ownerref:  &pbgo.KubeOwnerInfo{Kind: "ReplicaSet", Name: "nginx-7d4f8b9c6"},
		})
		assert.False(t, ok)
	})

	t.Run("reject when pod name shares the deployment prefix but a different ReplicaSet", func(t *testing.T) {
		// Same Deployment ("victim") but a different ReplicaSet hash: the pod is not a member
		// of the reported ReplicaSet and must be rejected.
		_, ok := deploymentOwnerForPod(&pbgo.PodLanguageDetails{
			Namespace: "default",
			Name:      "victim-bcdfg-2x9qd",
			Ownerref:  &pbgo.KubeOwnerInfo{Kind: "ReplicaSet", Name: "victim-7d4f8b9c6"},
		})
		assert.False(t, ok)
	})
}
