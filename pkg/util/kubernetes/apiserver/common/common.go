// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package common provides utility functions for interacting with Kubernetes API server.
package common

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultClusterIDMap = "datadog-cluster-id"
)

// getKubeSystemUID returns the UID of the kube-system namespace from the cluster
// We use it as the cluster ID so that even if the configmap is removed
// the new one should get the same ID.
func getKubeSystemUID(ctx context.Context, coreClient corev1.CoreV1Interface) (string, error) {
	svc, err := coreClient.Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(svc.UID), nil
}

// ResolveClusterID reads the cluster ID from a ConfigMap, and persists it when it
// is missing. It requires get, create, and update permissions on ConfigMaps in the
// Cluster Agent namespace. Callers own retries and caching.
func ResolveClusterID(ctx context.Context, coreClient corev1.CoreV1Interface) (string, error) {
	myNS := namespace.GetMyNamespace()

	cm, err := coreClient.ConfigMaps(myNS).Get(ctx, defaultClusterIDMap, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Errorf("Cannot retrieve ConfigMap %s/%s: %s", myNS, defaultClusterIDMap, err)
			return "", err
		}
		// The ConfigMap is absent; persist the stable kube-system namespace UID.
		clusterID, err := getKubeSystemUID(ctx, coreClient)
		if err != nil {
			log.Errorf("Failed getting the kube-system namespace: %v", err)
			return "", err
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
		_, err = coreClient.ConfigMaps(myNS).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			log.Errorf("Cannot create ConfigMap %s/%s: %s", myNS, defaultClusterIDMap, err)
			return "", err
		}
		return clusterID, nil
	}

	// config map exists, use its content or update it if the content doesn't look right
	clusterID, found := cm.Data["id"]
	if found && len([]byte(clusterID)) == 36 {
		return clusterID, nil
	}

	log.Warnf("Content of ConfigMap %s/%s doesn't look like a cluster ID, updating it", myNS, defaultClusterIDMap)
	clusterID, err = getKubeSystemUID(ctx, coreClient)
	if err != nil {
		log.Errorf("Failed getting the kube-system namespace: %v", err)
		return "", err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data["id"] = clusterID
	_, err = coreClient.ConfigMaps(myNS).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		log.Errorf("Failed to update ConfigMap %s/%s with correct cluster ID: %s", myNS, defaultClusterIDMap, err)
		return "", err
	}
	return clusterID, nil
}
