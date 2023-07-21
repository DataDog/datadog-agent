// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// objectParser is an interface allowing to plug any object
type objectParser interface {
	Parse(obj interface{}) workloadmeta.Entity
}

// entityUid glue together a WLM Entity and a Kube UID
type entityUid struct {
	entity workloadmeta.Entity
	uid    types.UID
}

type reflectorStore struct {
	wlmetaStore workloadmeta.Store

	mu     sync.Mutex
	seen   map[string]workloadmeta.EntityID
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
	entity := r.parser.Parse(obj)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.hasSynced = true
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
	entities := make([]entityUid, 0, len(list))

	for _, obj := range list {
		metaObj := obj.(metav1.Object)
		entities = append(entities, entityUid{r.parser.Parse(obj), metaObj.GetUID()})
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

		if _, ok := seenBefore[uid]; ok {
			delete(seenBefore, uid)
		}

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

	var kind workloadmeta.Kind
	var uid types.UID
	if pod, ok := obj.(*corev1.Pod); ok {
		kind = workloadmeta.KindKubernetesPod
		uid = pod.UID
	} else if node, ok := obj.(*corev1.Node); ok {
		kind = workloadmeta.KindKubernetesNode
		uid = node.UID
	}

	r.hasSynced = true
	delete(r.seen, string(uid))

	r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeUnset,
			Source: collectorID,
			Entity: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: kind,
					ID:   string(uid),
				},
			},
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
func (r *reflectorStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	panic("not implemented")
}

// GetByKey is not implemented
func (r *reflectorStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	panic("not implemented")
}

// Resync is not implemented
func (r *reflectorStore) Resync() error {
	panic("not implemented")
}
