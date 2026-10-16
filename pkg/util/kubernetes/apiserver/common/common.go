// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package common provides utility functions for interacting with Kubernetes API server.
package common

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultClusterIDMap = "datadog-cluster-id"
)

var (
	// clusterIDMutex serializes GetOrCreateClusterID calls in the process.
	clusterIDMutex sync.Mutex
	// clusterIDBackoff is the retry policy of GetOrCreateClusterID on concurrent
	// writes and transient API server errors.
	clusterIDBackoff = wait.Backoff{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Factor:   2,
		Jitter:   0.1,
	}
)

// GetKubeSystemUID returns the UID of the kube-system namespace from the cluster
// We use it as the cluster ID so that even if the configmap is removed
// the new one should get the same ID.
func GetKubeSystemUID(coreClient corev1.CoreV1Interface) (string, error) {
	svc, err := coreClient.Namespaces().Get(context.TODO(), "kube-system", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(svc.UID), nil
}

// GetOrCreateClusterID generates a cluster ID and persists it to a ConfigMap.
// It first checks if the CM exists, in which case it uses the ID it contains.
// It is safe to call concurrently, including from several cluster agent replicas.
// It thus requires get, create, and update perms on configmaps in the cluster-agent's namespace
func GetOrCreateClusterID(coreClient corev1.CoreV1Interface) (string, error) {
	cacheClusterIDKey := cache.BuildAgentKey(constants.ClusterIDCacheKey)
	if x, found := cache.Cache.Get(cacheClusterIDKey); found {
		return x.(string), nil
	}

	clusterIDMutex.Lock()
	defer clusterIDMutex.Unlock()
	// Another caller may have set it while waiting for the lock.
	if x, found := cache.Cache.Get(cacheClusterIDKey); found {
		return x.(string), nil
	}

	var clusterID string
	// The result is only cached on success: failed calls are tried again by the next caller.
	err := retry.OnError(clusterIDBackoff, isRetriableClusterIDError, func() error {
		var err error
		clusterID, err = tryGetOrCreateClusterID(coreClient)
		return err
	})
	if err != nil {
		log.Errorf("Failed to get or create the cluster ID: %v", err)
		return "", err
	}
	cache.Cache.Set(cacheClusterIDKey, clusterID, cache.NoExpiration)
	return clusterID, nil
}

// isRetriableClusterIDError returns whether getting or creating the cluster ID can succeed if tried again.
func isRetriableClusterIDError(err error) bool {
	// AlreadyExists and Conflict: another writer (e.g. another replica) created or
	// updated the ConfigMap concurrently, it then holds the ID.
	return errors.IsAlreadyExists(err) || errors.IsConflict(err) ||
		errors.IsServerTimeout(err) || errors.IsTimeout(err) || errors.IsTooManyRequests(err) ||
		errors.IsInternalError(err) || errors.IsServiceUnavailable(err)
}

// tryGetOrCreateClusterID gets the cluster ID from the ConfigMap, creating the
// ConfigMap or fixing its content if needed.
func tryGetOrCreateClusterID(coreClient corev1.CoreV1Interface) (string, error) {
	myNS := namespace.GetMyNamespace()

	cm, err := coreClient.ConfigMaps(myNS).Get(context.TODO(), defaultClusterIDMap, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", fmt.Errorf("cannot retrieve ConfigMap %s/%s: %w", myNS, defaultClusterIDMap, err)
		}
		// the config map doesn't exist yet, generate a UUID and persist it
		clusterID, err := GetKubeSystemUID(coreClient)
		if err != nil {
			return "", fmt.Errorf("failed getting the kube-system namespace: %w", err)
		}
		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultClusterIDMap,
				Namespace: myNS,
			},
			Data: map[string]string{
				"id": clusterID,
			},
		}
		_, err = coreClient.ConfigMaps(myNS).Create(context.TODO(), cm, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("cannot create ConfigMap %s/%s: %w", myNS, defaultClusterIDMap, err)
		}
		return clusterID, nil
	}

	// config map exists, use its content or update it if the content doesn't look right
	clusterID, found := cm.Data["id"]
	if found && len([]byte(clusterID)) == 36 {
		return clusterID, nil
	}

	log.Warnf("Content of ConfigMap %s/%s doesn't look like a cluster ID, updating it", myNS, defaultClusterIDMap)
	clusterID, err = GetKubeSystemUID(coreClient)
	if err != nil {
		return "", fmt.Errorf("failed getting the kube-system namespace: %w", err)
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["id"] = clusterID
	_, err = coreClient.ConfigMaps(myNS).Update(context.TODO(), cm, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to update ConfigMap %s/%s with correct cluster ID: %w", myNS, defaultClusterIDMap, err)
	}
	return clusterID, nil
}
