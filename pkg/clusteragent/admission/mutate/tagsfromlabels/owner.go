// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagsfromlabels

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type owner struct {
	name            string
	namespace       string
	kind            string
	labels          map[string]string
	ownerReferences []metav1.OwnerReference
}

// ownerInfo wraps the information needed to get pod's owner object
type ownerInfo struct {
	gvr  schema.GroupVersionResource
	name string
}

// buildID returns a unique identifier for the ownerInfo object
func (o *ownerInfo) buildID(ns string) string {
	return fmt.Sprintf("%s/%s/%s", ns, o.name, o.gvr.String())
}

// getOwnerInfo returns the required information to get the owner object
func getOwnerInfo(owner metav1.OwnerReference) (*ownerInfo, error) {
	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return nil, err
	}
	return &ownerInfo{
		gvr:  gv.WithResource(strings.ToLower(owner.Kind) + "s"),
		name: owner.Name,
	}, nil
}

// getOwner returns the object of the pod's owner
// If the owner is a replicaset it returns the corresponding deployment
func getOwner(owner metav1.OwnerReference, ns string, dc dynamic.Interface, ownerCacheTTL time.Duration) (*owner, error) {
	ownerInfo, err := getOwnerInfo(owner)
	if err != nil {
		return nil, err
	}

	obj, err := getAndCacheOwner(ownerInfo, ns, dc, ownerCacheTTL)
	if err != nil {
		return nil, err
	}

	// Try to discover standard labels from the deployment object if the owner is a replicaset
	if obj.kind == "ReplicaSet" && len(obj.ownerReferences) > 0 {
		rsOwnerInfo, err := getOwnerInfo(obj.ownerReferences[0])
		if err != nil {
			return nil, err
		}

		return getAndCacheOwner(rsOwnerInfo, ns, dc, ownerCacheTTL)
	}

	return obj, nil
}

// getAndCacheOwner tries to fetch the owner object from cache before querying the api server
func getAndCacheOwner(info *ownerInfo, ns string, dc dynamic.Interface, ownerCacheTTL time.Duration) (*owner, error) {
	infoID := info.buildID(ns)
	if cachedObj, hit := cache.Cache.Get(infoID); hit {
		metrics.GetOwnerCacheHit.Inc(info.gvr.Resource)
		owner, valid := cachedObj.(*owner)
		if !valid {
			log.Debugf("Invalid owner object for '%s', forcing a cache miss", infoID)
		} else {
			return owner, nil
		}
	}

	log.Tracef("Cache miss while getting owner '%s'", infoID)
	metrics.GetOwnerCacheMiss.Inc(info.gvr.Resource)
	ownerObj, err := dc.Resource(info.gvr).Namespace(ns).Get(context.TODO(), info.name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	owner := &owner{
		name:            ownerObj.GetName(),
		kind:            ownerObj.GetKind(),
		namespace:       ownerObj.GetNamespace(),
		labels:          ownerObj.GetLabels(),
		ownerReferences: ownerObj.GetOwnerReferences(),
	}

	cache.Cache.Set(infoID, owner, ownerCacheTTL)
	return owner, nil
}
