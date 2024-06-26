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
	Parse(obj interface{}) workloadmeta.Entity
}

// entityUID glue together a WLM Entity and a Kube UID
type entityUID struct {
	entity workloadmeta.Entity
	uid    types.UID
}

type reflectorStore struct {
	wlmetaStore workloadmeta.Component

	mu     sync.Mutex
	seen   map[string]workloadmeta.EntityID // needs to be updated only if the object is added
	parser objectParser
	// hasSynced logic is based on the logic see in FIFO queue (client-go/tools/cache/fifo.go)
	// Normally `Replace` is called first and then `Add/Update/Delete`.
	// If `Add/Update/Delete` is called first, triggers hasSynced
	hasSynced bool

	// filter to keep only resources that the Cluster-Agent needs
	filter reflectorStoreFilter
}

// The filter is called in Replace/Add/Delete functions before the obj is parsed
type reflectorStoreFilter interface {
	filteredOut(workloadmeta.Entity) bool
}

// Add notifies the workloadmeta store with  an EventTypeSet for the given
// object.
func (r *reflectorStore) Add(obj interface{}) error {
	metaObj := obj.(metav1.Object)
	entity := r.parser.Parse(obj)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.hasSynced = true
	if r.filter != nil && r.filter.filteredOut(entity) {
		// Don't store the object in memory if it is filtered out
		return nil
	}

	r.seen[string(metaObj.GetUID())] = entity.GetID()
	r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: collectorID,
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

	seenNow := make(map[string]workloadmeta.EntityID)
	seenBefore := r.seen

	for _, entityuid := range entities {
		entity := entityuid.entity
		uid := string(entityuid.uid)

		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeSet,
			Source: collectorID,
			Entity: entity,
		})

		delete(seenBefore, uid)

		seenNow[uid] = entity.GetID()
	}

	for _, entityID := range seenBefore {
		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: collectorID,
			Entity: &workloadmeta.KubernetesPod{
				EntityID: entityID,
			},
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
	case *appsv1.Deployment:
		uid = v.UID
	case *metav1.PartialObjectMetadata:
		uid = v.UID
	default:
		return fmt.Errorf("failed to identify Kind of object: %#v", obj)
	}

	r.hasSynced = true
	delete(r.seen, string(uid))

	entity = r.parser.Parse(obj)

	if r.filter != nil && r.filter.filteredOut(entity) {
		return nil
	}

	r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeUnset,
			Source: collectorID,
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
