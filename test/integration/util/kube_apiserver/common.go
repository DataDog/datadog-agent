// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package kubernetes

import (
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/integration/utils"
)

// initAPIServerCompose returns a ComposeConf ready to launch
// with etcd and the apiserver running in the same network
// namespace as the current process.
func initAPIServerCompose() (*utils.ComposeConf, error) {
	compose := &utils.ComposeConf{
		ProjectName: "kube_events",
		FilePath:    "testdata/apiserver-compose.yaml",
		Variables:   map[string]string{},
	}
	return compose, nil
}

func createObjectReference(namespace, kind, name string) *apiv1.ObjectReference {
	return &apiv1.ObjectReference{
		Namespace: namespace,
		Kind:      kind,
		Name:      name,
	}
}

func createEvent(namespace, name, reason string, involvedObject apiv1.ObjectReference) *apiv1.Event {
	return &apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		InvolvedObject: involvedObject,
		Reason:         reason,
	}
}

func createPodOnNode(namespace, name, nodeName string) *apiv1.Pod {
	return &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: apiv1.PodSpec{
			NodeName: nodeName,
			Containers: []apiv1.Container{
				{
					Name:  "dummy",
					Image: "dummy",
				},
			},
		},
	}
}
