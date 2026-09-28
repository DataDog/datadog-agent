// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustermetadata implements pkg/clustermetadata contract for a single DCA replica.
package clustermetadata

import (
	"context"
	"fmt"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Kinds supported by Lookup. Kind strings are the lowercase Kubernetes kinds
// used by the contract.
const (
	KindPod        = "pod"
	KindDeployment = "deployment"
	KindNode       = "node"
)

// LocalStore answers clustermetadata queries on behalf of this replica. It
// implements cm.Store.
type LocalStore struct {
	wmeta  workloadmeta.Component
	tagger tagger.Component
	ring   *RingController
	// peerSource returns the current peer handles
	peerSource func() []cm.Store
}

var _ cm.Store = (*LocalStore)(nil)

// NewLocalStore returns a Store for this replica.
func NewLocalStore(wmeta workloadmeta.Component, tagger tagger.Component, ring *RingController, peerSource func() []cm.Store) *LocalStore {
	return &LocalStore{wmeta: wmeta, tagger: tagger, ring: ring, peerSource: peerSource}
}

// currentPeers returns the peer handles, or nil when no source is wired
// (single-replica deployments).
func (s *LocalStore) currentPeers() []cm.Store {
	if s.peerSource == nil {
		return nil
	}
	return s.peerSource()
}

// Lookup implements cm.Store.Lookup().
func (s *LocalStore) Lookup(ctx context.Context, req cm.LookupRequest) (cm.LookupAnswer, error) {
	local, err := s.localLookup(ctx, req)
	log.Debugf("ring store: Lookup(%s/%s/%s) local: kind=%d err=%v", req.Key.Kind, req.Key.Namespace, req.Key.Name, local.Kind, err)
	if err != nil || local.Kind == cm.AnswerFound || req.Key.Kind != KindPod {
		return local, err
	}

	answers := []cm.LookupAnswer{local}
	for _, peer := range s.currentPeers() {
		answer, err := peer.Lookup(ctx, req)
		if err != nil {
			log.Debugf("ring store: Lookup(%s/%s/%s) peer error: %v", req.Key.Kind, req.Key.Namespace, req.Key.Name, err)
			// A peer failure makes absence impossible to conclude, so it
			// fails the query.
			return cm.LookupAnswer{}, err
		}
		log.Debugf("ring store: Lookup(%s/%s/%s) peer answer: kind=%d %d tags", req.Key.Kind, req.Key.Namespace, req.Key.Name, answer.Kind, len(answer.Tags))
		answers = append(answers, answer)
	}
	result := cm.ReducePeerAnswers(answers)
	log.Debugf("ring store: Lookup(%s/%s/%s) reduced: kind=%d %d tags", req.Key.Kind, req.Key.Namespace, req.Key.Name, result.Kind, len(result.Tags))
	return result, nil
}

// LookupOrigin implements cm.Store.LookupOrigin().
func (s *LocalStore) LookupOrigin(ctx context.Context, req cm.OriginLookupRequest) (cm.LookupAnswer, error) {
	local, err := s.localLookupOrigin(ctx, req)
	log.Debugf("ring store: LookupOrigin(uid=%s) local: kind=%d err=%v", req.Key.PodUID, local.Kind, err)
	if err != nil || local.Kind == cm.AnswerFound {
		return local, err
	}

	answers := []cm.LookupAnswer{local}
	for _, peer := range s.currentPeers() {
		answer, err := peer.LookupOrigin(ctx, req)
		if err != nil {
			log.Debugf("ring store: LookupOrigin(uid=%s) peer error: %v", req.Key.PodUID, err)
			return cm.LookupAnswer{}, err
		}
		log.Debugf("ring store: LookupOrigin(uid=%s) peer answer: kind=%d %d tags", req.Key.PodUID, answer.Kind, len(answer.Tags))
		answers = append(answers, answer)
	}
	result := cm.ReducePeerAnswers(answers)
	log.Debugf("ring store: LookupOrigin(uid=%s) reduced: kind=%d %d tags", req.Key.PodUID, result.Kind, len(result.Tags))
	return result, nil
}

func (s *LocalStore) localLookup(ctx context.Context, req cm.LookupRequest) (cm.LookupAnswer, error) {
	// look in our own cache first
	entityID, found, err := s.resolveNamed(req.Key)
	if err != nil {
		return cm.LookupAnswer{}, err
	}
	if !found {
		if req.Key.Kind == KindPod {
			// if it's a pod, another replica might have it, but only claim
			// NotMine once our own watches are synced
			state := s.ring.State()
			if state.Ready() {
				log.Debugf("ring store: localLookup pod miss -> NOT_MINE (ready, %d my nodes)", len(state.MyNodes))
				return cm.LookupAnswer{Kind: cm.AnswerNotMine}, nil
			}
			log.Debugf("ring store: localLookup pod miss -> NOT_READY (my nodes: %v, synced: %v)", state.MyNodes, state.NodeSynced)
			return cm.LookupAnswer{Kind: cm.AnswerNotReady}, nil
		}
		// if it's not a pod it should be replicated across all DCAs, so that means it's absent
		return cm.LookupAnswer{Kind: cm.AnswerAbsent}, nil
	}

	return s.tagAnswer(entityID, req.Scope.Cardinality)
}

// localLookupOrigin is localLookup for origin keys. The container-to-pod
// mapping is only complete for this replica's pods, so a miss is NotMine.
func (s *LocalStore) localLookupOrigin(ctx context.Context, req cm.OriginLookupRequest) (cm.LookupAnswer, error) {
	var pod *workloadmeta.KubernetesPod
	var err error

	switch {
	case req.Key.PodUID != "":
		pod, err = s.wmeta.GetKubernetesPod(req.Key.PodUID)
	case req.Key.ContainerID != "":
		pod, err = s.wmeta.GetKubernetesPodForContainer(req.Key.ContainerID)
	default:
		return cm.LookupAnswer{}, fmt.Errorf("origin key must set PodUID or ContainerID")
	}
	if pod == nil {
		if err != nil && !pkgerrors.IsNotFound(err) {
			return cm.LookupAnswer{}, err
		}
		// origin keys are always pods: NotMine only once our watches are synced
		state := s.ring.State()
		if state.Ready() {
			log.Debugf("ring store: localLookupOrigin miss -> NOT_MINE (ready, %d my nodes)", len(state.MyNodes))
			return cm.LookupAnswer{Kind: cm.AnswerNotMine}, nil
		}
		log.Debugf("ring store: localLookupOrigin miss -> NOT_READY (my nodes: %v, synced: %v)", state.MyNodes, state.NodeSynced)
		return cm.LookupAnswer{Kind: cm.AnswerNotReady}, nil
	}

	return s.tagAnswer(taggertypes.NewEntityID(taggertypes.KubernetesPodUID, pod.EntityID.ID), req.Scope.Cardinality)
}

// resolveNamed maps a contract entity key to the tagger entity ID of the
// corresponding workloadmeta entity. found is false when the store does not
// know the entity.
func (s *LocalStore) resolveNamed(key cm.EntityKey) (taggertypes.EntityID, bool, error) {
	var (
		id   string
		err  error
		kind taggertypes.EntityIDPrefix
	)
	found := true

	switch key.Kind {
	case KindPod:
		var pod *workloadmeta.KubernetesPod
		pod, err = s.wmeta.GetKubernetesPodByName(key.Name, key.Namespace)
		if pod == nil {
			found = false
		} else {
			id, kind = pod.EntityID.ID, taggertypes.KubernetesPodUID
		}
	case KindDeployment:
		// Deployments are stored as namespace/name (see the kubeapiserver
		// collector's deployment parser).
		var deployment *workloadmeta.KubernetesDeployment
		deployment, err = s.wmeta.GetKubernetesDeployment(key.Namespace + "/" + key.Name)
		if deployment == nil {
			found = false
		} else {
			id, kind = deployment.EntityID.ID, taggertypes.KubernetesDeployment
		}
	case KindNode:
		var node *workloadmeta.KubernetesNode
		node, err = s.wmeta.GetKubernetesNode(key.Name)
		if node == nil {
			found = false
		} else {
			id, kind = node.EntityID.ID, taggertypes.KubernetesNode
		}
	default:
		return taggertypes.EntityID{}, false, fmt.Errorf("unsupported kind %q", key.Kind)
	}

	if err != nil && !pkgerrors.IsNotFound(err) {
		return taggertypes.EntityID{}, false, err
	}
	if !found {
		return taggertypes.EntityID{}, false, nil
	}
	return taggertypes.NewEntityID(kind, id), true, nil
}

// tagAnswer fetches tags for the entity at the requested cardinality.
func (s *LocalStore) tagAnswer(entityID taggertypes.EntityID, cardinality taggertypes.TagCardinality) (cm.LookupAnswer, error) {
	tags, err := s.tagger.Tag(entityID, cardinality)
	if err != nil {
		return cm.LookupAnswer{}, err
	}
	return cm.LookupAnswer{Kind: cm.AnswerFound, Tags: tags}, nil
}
