// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

const kubeServiceIDPrefix = "kube_service_uid://"

// ServicesForPod returns the services mapped to a given pod and namespace.
// If nothing is found, the boolean is false. This call is thread-safe.
func (metaBundle *metadataMapperBundle) ServicesForPod(ns, podName string) ([]string, bool) {
	return metaBundle.Services.Get(ns, podName)
}

// DeepCopy used to copy data between two metadataMapperBundle
func (metaBundle *metadataMapperBundle) DeepCopy(old *metadataMapperBundle) *metadataMapperBundle {
	if metaBundle == nil || old == nil {
		return metaBundle
	}
	metaBundle.Services = metaBundle.Services.DeepCopy(&old.Services)
	metaBundle.mapOnIP = old.mapOnIP
	return metaBundle
}

func EntityForService(svc *v1.Service) string {
	if svc == nil {
		return ""
	}
	return fmt.Sprintf("%s%s", kubeServiceIDPrefix, svc.ObjectMeta.UID)
}

// GetEndpoints() retrieves all the endpoints in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetServices() ([]v1.Service, error) {
	serviceList, err := c.Cl.CoreV1().Services(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return []v1.Service{}, err
	}

	return serviceList.Items, nil
}

// GetIngresses() retrieves all the ingress endpoints linked to services in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetIngresses() ([]v1beta1.Ingress, error) {
	ingressList, err := c.Cl.ExtensionsV1beta1().Ingresses(metav1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return []v1beta1.Ingress{}, err
	}

	return ingressList.Items, nil
}
