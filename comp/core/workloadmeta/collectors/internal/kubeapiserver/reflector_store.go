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
	"k8s.io/apimachinery/pkg/types"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// objectParser is an interface allowing to plug any object
type objectParser interface {
	Parse(obj interface{}) []workloadmeta.Entity
}

// entityUID glue together a WLM Entity slice and a Kube UID
type entitiesUID struct {
	entities []workloadmeta.Entity
	uid      types.UID
}

type reflectorStore struct {
	wlmetaStore workloadmeta.Component

	mu     sync.Mutex
	seen   map[string][]workloadmeta.EntityID // needs to be updated only if the object is added
	parser objectParser
	// hasSynced logic is based on the logic see in FIFO queue (client-go/tools/cache/fifo.go)
	// Normally `Replace` is called first and then `Add/Update/Delete`.
	// If `Add/Update/Delete` is called first, triggers hasSynced
	hasSynced bool
}

// Add notifies the workloadmeta store with  an EventTypeSet for the given
// object.
func (r *reflectorStore) Add(obj interface{}) error {
	metaObj := obj.(metav1.Object)
	producedEntities := r.parser.Parse(obj)
	r.seen[string(metaObj.GetUID())] = make([]workloadmeta.EntityID, 0, len(producedEntities))

	r.mu.Lock()
	defer r.mu.Unlock()
	r.hasSynced = true

	for _, entity := range producedEntities {
		r.seen[string(metaObj.GetUID())] = append(r.seen[string(metaObj.GetUID())], entity.GetID())
		r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
			{
				Type:   workloadmeta.EventTypeSet,
				Source: collectorID,
				Entity: entity,
			},
		})
	}

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
	entitiesuids := make([]entitiesUID, 0, len(list))

	for _, obj := range list {
		parsedEntites := r.parser.Parse(obj)
		entitiesuids = append(entitiesuids, entitiesUID{parsedEntites, obj.(metav1.Object).GetUID()})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var events []workloadmeta.CollectorEvent

	seenNow := make(map[string][]workloadmeta.EntityID)
	seenBefore := r.seen

	for _, entitiesuid := range entitiesuids {
		producedEntities := entitiesuid.entities
		uid := string(entitiesuid.uid)

		seenNow[uid] = make([]workloadmeta.EntityID, 0, len(producedEntities))

		for _, producedEntity := range producedEntities {
			events = append(events, workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: collectorID,
				Entity: producedEntity,
			})

			seenNow[uid] = append(seenNow[uid], producedEntity.GetID())
		}

		delete(seenBefore, uid)
	}

	for _, entityIDs := range seenBefore {

		for _, entityID := range entityIDs {
			entity, err := entityFromEntityID(entityID)
			if err != nil {
				return err
			}

			events = append(events, workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeUnset,
				Source: collectorID,
				Entity: entity,
			})
		}
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
	switch v := obj.(type) {
	// All the supported objects need to be in this switch statement to be able
	// to be deleted.
	case *corev1.Pod:
		uid = v.UID
	case *appsv1.Deployment:
		uid = v.UID
	case *metav1.PartialObjectMetadata:
		uid = v.UID
	default:
		return fmt.Errorf("failed to identify Kind of object: %#v", obj)
	}

	r.hasSynced = true
	delete(r.seen, string(uid))

	producedEntities := r.parser.Parse(obj)

	for _, entity := range producedEntities {

		r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
			{
				Type:   workloadmeta.EventTypeUnset,
				Source: collectorID,
				Entity: entity,
			},
		})
	}

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

	case workloadmeta.KindKubernetesPod:
		return &workloadmeta.KubernetesPod{
			EntityID: entityID,
		}, nil

	case workloadmeta.KindKubernetesMetadata:
		return &workloadmeta.KubernetesMetadata{
			EntityID: entityID,
		}, nil
	}

	return nil, fmt.Errorf("unsupported entity kind: %s", entityID.Kind)
}
