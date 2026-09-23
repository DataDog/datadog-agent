// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This is an external test package because it drives the collector into the
// real tag store, and tagstore imports collectors.
package collectors_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// TestTagsClearedWhenAnEntityLosesThem goes from workloadmeta events to the
// real tag store, because clearing is decided in two places: the handler
// always reports its current tags (an empty TagInfo when there are none), and
// the store interprets an empty TagInfo depending on what it holds. Neither
// half can be checked in isolation.
func TestTagsClearedWhenAnEntityLosesThem(t *testing.T) {
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	newCollector := func(t *testing.T, labelsAsTags map[string]map[string]string) (*collectors.WorkloadMetaCollector, *tagstore.TagStore) {
		tagStore := tagstore.NewTagStore(nil)
		collector := collectors.NewWorkloadMetaCollector(context.Background(), configmock.New(t), store, tagStore)
		collector.InitK8sResourcesMetaAsTagsForTest(labelsAsTags, nil)
		return collector, tagStore
	}
	set := func(collector *collectors.WorkloadMetaCollector, entity workloadmeta.Entity) {
		collector.ProcessEventsForTest(workloadmeta.EventBundle{
			Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: entity}},
		})
	}
	lowTags := func(t *testing.T, tagStore *tagstore.TagStore, id types.EntityID) []string {
		tags, err := tagStore.LookupHashed(id, types.LowCardinality)
		require.NoError(t, err)
		return tags.Get()
	}
	assertGone := func(t *testing.T, tagStore *tagstore.TagStore, id types.EntityID) {
		_, err := tagStore.LookupHashed(id, types.LowCardinality)
		assert.ErrorIs(t, err, tagstore.ErrNotFound, "the entity must be removed, not kept with no tags")
	}

	deploymentID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesDeployment, ID: "default/fooapp"}
	deploymentTaggerID := types.NewEntityID(types.KubernetesDeployment, deploymentID.ID)
	deployment := func(kinds sets.Set[string], labels map[string]string) *workloadmeta.KubernetesDeployment {
		return &workloadmeta.KubernetesDeployment{
			EntityID:        deploymentID,
			EntityMeta:      workloadmeta.EntityMeta{Name: "fooapp", Namespace: "default", Labels: labels},
			AutoscalerKinds: kinds,
		}
	}

	t.Run("last autoscaler removed from a deployment", func(t *testing.T) {
		collector, tagStore := newCollector(t, nil)

		set(collector, deployment(sets.New(kubernetes.AutoscalerKindHPA), nil))
		assert.Equal(t, []string{"kube_autoscaler_kind:hpa"}, lowTags(t, tagStore, deploymentTaggerID))

		set(collector, deployment(nil, nil))
		assertGone(t, tagStore, deploymentTaggerID)
	})

	t.Run("label mapped as a tag removed from a deployment", func(t *testing.T) {
		collector, tagStore := newCollector(t, map[string]map[string]string{"deployments.apps": {"team": "team"}})

		set(collector, deployment(nil, map[string]string{"team": "platform"}))
		assert.Equal(t, []string{"team:platform"}, lowTags(t, tagStore, deploymentTaggerID))

		set(collector, deployment(nil, map[string]string{}))
		assertGone(t, tagStore, deploymentTaggerID)
	})

	// StatefulSets and other workloads reach the tagger as KubernetesMetadata,
	// so they get the same behaviour without any per-kind code.
	t.Run("label mapped as a tag removed from a generic resource", func(t *testing.T) {
		collector, tagStore := newCollector(t, map[string]map[string]string{"statefulsets.apps": {"team": "team"}})

		metadataID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesMetadata, ID: "apps/statefulsets/default/db"}
		metadataTaggerID := types.NewEntityID(types.KubernetesMetadata, metadataID.ID)
		statefulSet := func(labels map[string]string) *workloadmeta.KubernetesMetadata {
			return &workloadmeta.KubernetesMetadata{
				EntityID:   metadataID,
				EntityMeta: workloadmeta.EntityMeta{Name: "db", Namespace: "default", Labels: labels},
				GVR:        &schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"},
			}
		}

		set(collector, statefulSet(map[string]string{"team": "data"}))
		assert.Equal(t, []string{"team:data"}, lowTags(t, tagStore, metadataTaggerID))

		set(collector, statefulSet(map[string]string{}))
		assertGone(t, tagStore, metadataTaggerID)
	})

	// StatefulSets and Argo Rollouts carry their autoscaler kinds on their
	// generic metadata entity.
	t.Run("autoscaler kinds on a statefulset, then removed", func(t *testing.T) {
		collector, tagStore := newCollector(t, nil)

		metadataID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesMetadata, ID: "apps/statefulsets/default/db"}
		metadataTaggerID := types.NewEntityID(types.KubernetesMetadata, metadataID.ID)
		statefulSet := func(kinds sets.Set[string]) *workloadmeta.KubernetesMetadata {
			return &workloadmeta.KubernetesMetadata{
				EntityID:        metadataID,
				EntityMeta:      workloadmeta.EntityMeta{Name: "db", Namespace: "default"},
				GVR:             &schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"},
				AutoscalerKinds: kinds,
			}
		}

		set(collector, statefulSet(sets.New(kubernetes.AutoscalerKindHPA, kubernetes.AutoscalerKindVPA)))
		assert.ElementsMatch(t, []string{"kube_autoscaler_kind:hpa", "kube_autoscaler_kind:vpa"}, lowTags(t, tagStore, metadataTaggerID))

		set(collector, statefulSet(nil))
		assertGone(t, tagStore, metadataTaggerID)
	})

	// The reason handlers used to return nil: an entity that never had tags
	// must not get an empty tagger entity. The store now guarantees it.
	t.Run("entity that never had tags gets no tagger entity", func(t *testing.T) {
		collector, tagStore := newCollector(t, nil)

		set(collector, deployment(nil, map[string]string{"team": "platform"}))
		assertGone(t, tagStore, deploymentTaggerID)
	})
}
