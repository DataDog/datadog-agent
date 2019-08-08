// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
