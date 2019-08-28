// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const kubeServiceIDPrefix = "kube_service://"

// ServicesMapper maps pod names to the names of the services targeting the pod
// keyed by the namespace a pod belongs to. This data structure allows for O(1)
// lookups of services given a namespace and pod name.
//
// The data is stored in the following schema:
// {
// 	"namespace": {
// 		"pod": [ "svc1", "svc2", "svc3" ]
// 	}
// }
type ServicesMapper map[string]map[string]sets.String

// Get returns the list of services for a given namespace and pod name.
func (m ServicesMapper) Get(namespace, podName string) ([]string, bool) {
	if _, ok := m[namespace]; !ok {
		return nil, false
	}
	if _, ok := m[namespace][podName]; !ok {
		return nil, false
	}
	return m[namespace][podName].UnsortedList(), true
}

// Set updates services for a given namespace and pod name.
func (m ServicesMapper) Set(namespace, podName string, svcs ...string) {
	if _, ok := m[namespace]; !ok {
		m[namespace] = make(map[string]sets.String)
	}
	if _, ok := m[namespace][podName]; !ok {
		m[namespace][podName] = sets.NewString()
	}
	m[namespace][podName].Insert(svcs...)
}

// Delete deletes services for a given namespace.
func (m ServicesMapper) Delete(namespace string, svcs ...string) {
	if _, ok := m[namespace]; !ok {
		// Nothing to delete.
		return
	}
	for podName, svcSet := range m[namespace] {
		svcSet.Delete(svcs...)

		if svcSet.Len() == 0 {
			delete(m[namespace], podName)
		}
	}
	if len(m[namespace]) == 0 {
		delete(m, namespace)
	}
}

// ServicesForPod returns the services mapped to a given pod and namespace.
// If nothing is found, the boolean is false. This call is thread-safe.
func (metaBundle *MetadataMapperBundle) ServicesForPod(ns, podName string) ([]string, bool) {
	metaBundle.m.RLock()
	defer metaBundle.m.RUnlock()

	return metaBundle.Services.Get(ns, podName)
}

func EntityForService(svc *v1.Service) string {
	if svc == nil {
		return ""
	}
	return fmt.Sprintf("%s%s", kubeServiceIDPrefix, svc.ObjectMeta.UID)
}

// GetEndpoints() retrieves all the endpoints in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetServices() ([]v1.Service, error) {
	serviceList, err := c.Cl.CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return []v1.Service{}, err
	}

	return serviceList.Items, nil
}
