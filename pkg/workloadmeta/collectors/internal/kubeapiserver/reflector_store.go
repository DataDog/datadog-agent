// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"fmt"
	"regexp"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	utilserror "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type reflectorStore struct {
	wlmetaStore workloadmeta.Store

	mu   sync.Mutex
	seen map[string]workloadmeta.EntityID
}

type podReflectorStore struct {
	reflectorStore

	options *parseOptions
}

type nodeReflectorStore struct {
	reflectorStore
}

func newPodReflectorStore(wlmetaStore workloadmeta.Store) cache.Store {
	annotationsExclude := config.Datadog.GetStringSlice("cluster_agent.kubernetes_resources_collection.pod_annotations_exclude")
	parseOptions, err := newParseOptions(annotationsExclude)
	if err != nil {
		_ = log.Errorf("unable to parse all pod_annotations_exclude: %v, err:", err)
	}
	return &podReflectorStore{
		reflectorStore: reflectorStore{
			wlmetaStore: wlmetaStore,
			seen:        make(map[string]workloadmeta.EntityID),
		},
		options: parseOptions,
	}
}

func newNodeReflectorStore(wlmetaStore workloadmeta.Store) cache.Store {
	return &nodeReflectorStore{
		reflectorStore: reflectorStore{
			wlmetaStore: wlmetaStore,
			seen:        make(map[string]workloadmeta.EntityID),
		},
	}
}

func (r *reflectorStore) add(uid types.UID, entity workloadmeta.Entity) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seen[string(uid)] = entity.GetID()

	r.wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: collectorID,
			Entity: entity,
		},
	})

	return nil
}

// Add notifies the workloadmeta store with  an EventTypeSet for the given
// object.
func (r *podReflectorStore) Add(obj interface{}) error {
	pod := obj.(*corev1.Pod)
	entity := parsePod(pod, r.options)

	return r.add(pod.UID, entity)
}

// Add notifies the workloadmeta store with  an EventTypeSet for the given
// object.
func (r *nodeReflectorStore) Add(obj interface{}) error {
	node := obj.(*corev1.Node)
	entity := parseNode(node)

	return r.add(node.UID, entity)
}

// Update notifies the workloadmeta store with  an EventTypeSet for the given
// object.
func (r *podReflectorStore) Update(obj interface{}) error {
	return r.Add(obj)
}

// Update notifies the workloadmeta store with  an EventTypeSet for the given
// object.
func (r *nodeReflectorStore) Update(obj interface{}) error {
	return r.Add(obj)
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

type entityUid struct {
	entity workloadmeta.Entity
	uid    types.UID
}

// Replace diffs the given list with the contents of the workloadmeta store
// (through r.seen), and updates and deletes the necessary objects.
func (r *reflectorStore) replace(entities []entityUid) error {
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

	return nil
}

// Replace diffs the given list with the contents of the workloadmeta store
// (through r.seen), and updates and deletes the necessary objects.
func (r *podReflectorStore) Replace(list []interface{}, _ string) error {
	entities := make([]entityUid, 0, len(list))

	for _, obj := range list {
		pod := obj.(*corev1.Pod)
		entities = append(entities, entityUid{parsePod(pod, r.options), pod.UID})
	}

	return r.replace(entities)
}

// Replace diffs the given list with the contents of the workloadmeta store
// (through r.seen), and updates and deletes the necessary objects.
func (r *nodeReflectorStore) Replace(list []interface{}, _ string) error {
	entities := make([]entityUid, 0, len(list))

	for _, obj := range list {
		node := obj.(*corev1.Node)
		entities = append(entities, entityUid{parseNode(node), node.UID})
	}

	return r.replace(entities)
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

type parseOptions struct {
	annotationsFilter []*regexp.Regexp
}

func newParseOptions(annotationsExclude []string) (*parseOptions, error) {
	options := parseOptions{}
	var errors []error
	for _, exclude := range annotationsExclude {
		filter, err := filterToRegex(exclude)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		options.annotationsFilter = append(options.annotationsFilter, filter)
	}

	return &options, utilserror.NewAggregate(errors)
}

func parsePod(pod *corev1.Pod, options *parseOptions) *workloadmeta.KubernetesPod {
	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, o := range pod.OwnerReferences {
		owners = append(owners, workloadmeta.KubernetesPodOwner{
			Kind: o.Kind,
			Name: o.Name,
			ID:   string(o.UID),
		})
	}

	var ready bool
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			if condition.Status == corev1.ConditionTrue {
				ready = true
			}
			break
		}
	}

	var pvcNames []string
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   string(pod.UID),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: filterMapStringKey(pod.Annotations, options.annotationsFilter),
			Labels:      pod.Labels,
		},
		Phase:                      string(pod.Status.Phase),
		Owners:                     owners,
		PersistentVolumeClaimNames: pvcNames,
		Ready:                      ready,
		IP:                         pod.Status.PodIP,
		PriorityClass:              pod.Spec.PriorityClassName,
		QOSClass:                   string(pod.Status.QOSClass),

		// Containers could be generated by this collector, but
		// currently it's not to save on memory, since this is supposed
		// to run in the Cluster Agent, and the total amount of
		// containers can be quite significant
		// Containers:                 []workloadmeta.OrchestratorContainer{},
	}
}

func parseNode(node *corev1.Node) *workloadmeta.KubernetesNode {
	return &workloadmeta.KubernetesNode{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesNode,
			ID:   node.Name,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        node.Name,
			Annotations: node.Annotations,
			Labels:      node.Labels,
		},
	}
}

func filterMapStringKey(mapInput map[string]string, keyFilters []*regexp.Regexp) map[string]string {
	for key := range mapInput {
		for _, filter := range keyFilters {
			if filter.MatchString(key) {
				delete(mapInput, key)
				// we can break now since the key is already excluded.
				break
			}
		}
	}

	return mapInput
}

// filterToRegex checks a filter's regex
func filterToRegex(filter string) (*regexp.Regexp, error) {
	r, err := regexp.Compile(filter)
	if err != nil {
		errormsg := fmt.Errorf("invalid regex '%s': %s", filter, err)
		return nil, errormsg
	}
	return r, nil
}
