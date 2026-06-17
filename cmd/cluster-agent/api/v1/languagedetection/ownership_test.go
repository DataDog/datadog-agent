// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestAuthoritativeDeploymentOwnerForPodLanguages(t *testing.T) {
	const (
		ns       = "default"
		podName  = "my-pod"
		podUID   = "pod-uid-1"
		rsName   = "nginx-7d4f8b9c6"
		hash     = "7d4f8b9c6"
		deploy   = "nginx"
		deployID = ns + "/" + deploy
	)

	t.Run("happy path", func(t *testing.T) {
		store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))

		store.Set(&workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: podUID},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      podName,
				Namespace: ns,
				Labels:    map[string]string{podTemplateHashLabelKey: hash},
			},
			Owners: []workloadmeta.KubernetesPodOwner{{
				Kind:       "ReplicaSet",
				Name:       rsName,
				ID:         "rs-uid",
				Group:      "apps",
				Controller: boolPtr(true),
			}},
		})
		store.Set(&workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesDeployment, ID: deployID},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      deploy,
				Namespace: ns,
			},
		})

		got, ok := authoritativeDeploymentOwnerForPodLanguages(store, ns, podName)
		require.True(t, ok)
		want := langUtil.NewNamespacedOwnerReference("apps/v1", langUtil.KindDeployment, deploy, ns)
		assert.Equal(t, want, got)
	})

	t.Run("reject deployment controller (forged owner ref pattern)", func(t *testing.T) {
		store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))

		store.Set(&workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: podUID},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      podName,
				Namespace: ns,
			},
			Owners: []workloadmeta.KubernetesPodOwner{{
				Kind:       "Deployment",
				Name:       "victim",
				ID:         "dep-uid",
				Group:      "apps",
				Controller: boolPtr(true),
			}},
		})
		store.Set(&workloadmeta.KubernetesDeployment{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesDeployment, ID: ns + "/victim"},
			EntityMeta: workloadmeta.EntityMeta{Name: "victim", Namespace: ns},
		})

		_, ok := authoritativeDeploymentOwnerForPodLanguages(store, ns, podName)
		assert.False(t, ok)
	})

	t.Run("reject when pod-template-hash mismatches ReplicaSet suffix", func(t *testing.T) {
		store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))

		store.Set(&workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: podUID},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      podName,
				Namespace: ns,
				Labels:    map[string]string{podTemplateHashLabelKey: "wronghash"},
			},
			Owners: []workloadmeta.KubernetesPodOwner{{
				Kind:       "ReplicaSet",
				Name:       rsName,
				ID:         "rs-uid",
				Group:      "apps",
				Controller: boolPtr(true),
			}},
		})
		store.Set(&workloadmeta.KubernetesDeployment{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesDeployment, ID: deployID},
			EntityMeta: workloadmeta.EntityMeta{Name: deploy, Namespace: ns},
		})

		_, ok := authoritativeDeploymentOwnerForPodLanguages(store, ns, podName)
		assert.False(t, ok)
	})

	t.Run("reject when deployment missing from workloadmeta", func(t *testing.T) {
		store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		))

		store.Set(&workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: podUID},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      podName,
				Namespace: ns,
				Labels:    map[string]string{podTemplateHashLabelKey: hash},
			},
			Owners: []workloadmeta.KubernetesPodOwner{{
				Kind:       "ReplicaSet",
				Name:       rsName,
				ID:         "rs-uid",
				Group:      "apps",
				Controller: boolPtr(true),
			}},
		})

		_, ok := authoritativeDeploymentOwnerForPodLanguages(store, ns, podName)
		assert.False(t, ok)
	})
}
