// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"fmt"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// entityUID glue together a WLM Entity and a Kube UID
type entityUID struct {
	entity workloadmeta.Entity
	uid    types.UID
}

// seenEntity records enough about a previously-notified entity to later
// retract its ownership relationships, even once the originating object is
// no longer available (e.g. when Replace() diffs it away).
type seenEntity struct {
	entityID        workloadmeta.EntityID
	ownerReferences []workloadmeta.EntityID
}

type reflectorStore struct {
	wlmetaStore workloadmeta.Component

	mu     sync.Mutex
	seen   map[string]seenEntity // needs to be updated only if the object is added
	parser kubernetesresourceparsers.ObjectParser
	// hasSynced logic is based on the logic see in FIFO queue (client-go/tools/cache/fifo.go)
	// Normally `Replace` is called first and then `Add/Update/Delete`.
	// If `Add/Update/Delete` is called first, triggers hasSynced
	hasSynced bool

	// filter to keep only resources that the Cluster-Agent needs
	filter reflectorStoreFilter

	// entityRelationships is the collector-wide owner->children index, shared
	// across every reflectorStore instance since a child's owner may be
	// discovered by a different GVR's reflector than the child itself.
	entityRelationships *entityRelationships
}

// The filter is called in Replace/Add/Delete functions before the obj is parsed
type reflectorStoreFilter interface {
	filteredOut(workloadmeta.Entity) bool
}

// pushChildReferences notifies wlmetaStore of owner's current set of
// children, under a dedicated source so that the per-source merge combines
// it with owner's canonical fields (from its own collector source) instead
// of overwriting them. If owner's kind isn't one workloadmeta tracks, or the
// owner doesn't otherwise exist, this is a no-op.
func (r *reflectorStore) pushChildReferences(owner workloadmeta.EntityID) {
	entity, err := entityFromEntityID(owner)
	if err != nil {
		return
	}

	entity.SetChildReferences(r.entityRelationships.children(owner))

	r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceKubernetesChildReferences,
			Entity: entity,
		},
	})
}

// retractChildReferences retracts any SourceKubernetesChildReferences
// contribution previously pushed for id, since id is being removed and can no
// longer be tracked as an owner. It's a no-op if no such contribution exists.
func (r *reflectorStore) retractChildReferences(id workloadmeta.EntityID) {
	entity, err := entityFromEntityID(id)
	if err != nil {
		return
	}

	r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceKubernetesChildReferences,
			Entity: entity,
		},
	})
}

// Add notifies the workloadmeta store with  an EventTypeSet for the given
// object.
func (r *reflectorStore) Add(obj interface{}) error {
	metaObj := obj.(metav1.Object)
	entity := r.parser.Parse(obj)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Update ownership map
	for _, owner := range entity.GetOwnerReferences() {
		r.entityRelationships.addChild(owner, entity.GetID())
		r.pushChildReferences(owner)
	}

	r.hasSynced = true
	if r.filter != nil && r.filter.filteredOut(entity) {
		// Don't store the object in memory if it is filtered out
		return nil
	}

	r.seen[string(metaObj.GetUID())] = seenEntity{entityID: entity.GetID(), ownerReferences: entity.GetOwnerReferences()}
	r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: entity,
		},
	})

	return nil
}

// Update notifies the workloadmeta store with  an EventTypeSet for the given
// object.
func (r *reflectorStore) Update(obj interface{}) error {
	return r.Add(obj)
}

// Replace diffs the given list with the contents of the workloadmeta store
// (through r.seen), and updates and deletes the necessary objects.
func (r *reflectorStore) Replace(list []interface{}, _ string) error {
	entities := make([]entityUID, 0, len(list))

	for _, obj := range list {
		entity := r.parser.Parse(obj)
		if r.filter != nil && r.filter.filteredOut(entity) {
			continue
		}
		entities = append(entities, entityUID{entity, obj.(metav1.Object).GetUID()})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var events []workloadmeta.CollectorEvent

	seenNow := make(map[string]seenEntity)
	seenBefore := r.seen

	for _, entityuid := range entities {
		entity := entityuid.entity

		// Update ownership map
		for _, owner := range entity.GetOwnerReferences() {
			r.entityRelationships.addChild(owner, entity.GetID())
			r.pushChildReferences(owner)
		}

		uid := string(entityuid.uid)

		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: entity,
		})

		delete(seenBefore, uid)

		seenNow[uid] = seenEntity{entityID: entity.GetID(), ownerReferences: entity.GetOwnerReferences()}
	}

	for _, stale := range seenBefore {
		entity, err := entityFromEntityID(stale.entityID)
		if err != nil {
			return err
		}

		// Update ownership map: this entity is no longer a child of its
		// owners, and can no longer be an owner of anything itself.
		for _, owner := range stale.ownerReferences {
			r.entityRelationships.removeChild(owner, stale.entityID)
			r.pushChildReferences(owner)
		}
		r.entityRelationships.removeOwner(stale.entityID)
		r.retractChildReferences(stale.entityID)

		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: entity,
		})
	}

	r.wlmetaStore.Notify(events)
	r.seen = seenNow
	r.hasSynced = true

	return nil
}

// Delete notifies the workloadmeta store with  an EventTypeUnset for the given
// object.
func (r *reflectorStore) Delete(obj interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var uid types.UID
	var entity workloadmeta.Entity
	switch v := obj.(type) {
	// All the supported objects need to be in this switch statement to be able
	// to be deleted.
	case *corev1.Pod:
		uid = v.UID
	case *MinimalPod:
		uid = v.UID
	case *appsv1.Deployment:
		uid = v.UID
	case *corev1.Node:
		uid = v.UID
	case *metav1.PartialObjectMetadata:
		uid = v.UID
	case *unstructured.Unstructured:
		uid = v.GetUID()
	default:
		return fmt.Errorf("failed to identify Kind of object: %#v", obj)
	}

	r.hasSynced = true
	delete(r.seen, string(uid))

	entity = r.parser.Parse(obj)

	// Update ownership map: this entity is no longer a child of its owners,
	// and can no longer be an owner of anything itself.
	for _, owner := range entity.GetOwnerReferences() {
		r.entityRelationships.removeChild(owner, entity.GetID())
		r.pushChildReferences(owner)
	}
	r.entityRelationships.removeOwner(entity.GetID())
	r.retractChildReferences(entity.GetID())

	if r.filter != nil && r.filter.filteredOut(entity) {
		return nil
	}

	r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: entity,
		},
	})

	return nil
}

func (r *reflectorStore) HasSynced() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.hasSynced
}

// List is not implemented
func (r *reflectorStore) List() []interface{} {
	panic("not implemented")
}

// ListKeys is not implemented
func (r *reflectorStore) ListKeys() []string {
	panic("not implemented")
}

// Get is not implemented
func (r *reflectorStore) Get(_ interface{}) (item interface{}, exists bool, err error) {
	panic("not implemented")
}

// GetByKey is not implemented
func (r *reflectorStore) GetByKey(_ string) (item interface{}, exists bool, err error) {
	panic("not implemented")
}

// Resync is not implemented
func (r *reflectorStore) Resync() error {
	panic("not implemented")
}

func entityFromEntityID(entityID workloadmeta.EntityID) (workloadmeta.Entity, error) {
	// All the supported objects need to be in this switch statement
	switch entityID.Kind {
	case workloadmeta.KindKubernetesDeployment:
		return &workloadmeta.KubernetesDeployment{
			EntityID: entityID,
		}, nil

	case workloadmeta.KindKubernetesNode:
		return &workloadmeta.KubernetesNode{
			EntityID: entityID,
		}, nil

	case workloadmeta.KindKubernetesPod:
		return &workloadmeta.KubernetesPod{
			EntityID: entityID,
		}, nil

	case workloadmeta.KindKubernetesMetadata:
		return &workloadmeta.KubernetesMetadata{
			EntityID: entityID,
		}, nil

	case workloadmeta.KindKubernetesKueueQueue:
		return &workloadmeta.KubernetesKueueQueue{
			EntityID: entityID,
		}, nil

	case workloadmeta.KindKubernetesKueueResourceFlavor:
		return &workloadmeta.KubernetesKueueResourceFlavor{
			EntityID: entityID,
		}, nil

	case workloadmeta.KindKubernetesKueueWorkload:
		return &workloadmeta.KubernetesKueueWorkload{
			EntityID: entityID,
		}, nil
	}

	return nil, fmt.Errorf("unsupported entity kind: %s", entityID.Kind)
}
