// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver && test

package listeners

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// CreateDummyKubeService creates a dummy KubeServiceService for testing purposes.
func CreateDummyKubeService(name, namespace string, annotations map[string]string) *KubeServiceService {
	ksvc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "10.0.0.1",
			Ports: []v1.ServicePort{
				{Name: "test1", Port: 123},
			},
		},
	}
	return &KubeServiceService{
		entity:   apiserver.EntityForService(ksvc),
		metadata: workloadfilter.CreateKubeService(name, namespace, annotations),
	}
}

// CreateDummyKubeEndpoint creates a dummy KubeEndpointService for testing purposes.
func CreateDummyKubeEndpoint(name, namespace string, annotations map[string]string) *KubeEndpointService {
	return &KubeEndpointService{
		entity:   apiserver.EntityForEndpoints(namespace, name, "10.0.0.1"),
		metadata: workloadfilter.CreateKubeEndpoint(name, namespace, annotations),
	}
}

// CreateDummyContainerService creates a dummy ContainerService for testing purposes.
func CreateDummyContainerService(ctn *workloadmeta.Container, tagger tagger.Component, wmeta workloadmeta.Component) *WorkloadService {
	return &WorkloadService{
		entity:        ctn,
		adIdentifiers: []string{ctn.Image.RawName},
		tagger:        tagger,
		wmeta:         wmeta,
	}
}

// CreateDummyPod creates a dummy KubernetesPod for testing purposes.
func CreateDummyPod(name, namespace string, annotations map[string]string) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "pod-id",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

// CreateDummyContainer creates a dummy Container for testing purposes.
func CreateDummyContainer(name, image string) *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "container-id",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: name,
		},
		Image: workloadmeta.ContainerImage{
			RawName: image,
		},
	}
}
