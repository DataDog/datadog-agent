// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubemetadata"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func Test_MetadataFakeClient(t *testing.T) {
	ns := "default"
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	objectMeta := metav1.ObjectMeta{
		Name:        "test-app",
		Namespace:   ns,
		Labels:      map[string]string{"test-label": "test-value"},
		Annotations: map[string]string{"k": "v"},
	}

	createObjects := func() []runtime.Object {
		return []runtime.Object{
			&metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       kubernetes.DeploymentKind,
				},
				ObjectMeta: objectMeta,
			},
		}
	}

	expected := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesMetadata{
					EntityID: workloadmeta.EntityID{
						ID:   string(kubemetadata.GenerateEntityID("apps", "deployments", "default", "test-app")),
						Kind: workloadmeta.KindKubernetesMetadata,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:        objectMeta.Name,
						Namespace:   "default",
						Labels:      objectMeta.Labels,
						Annotations: objectMeta.Annotations,
					},
					GVR: &gvr,
				},
			},
		},
	}

	testCollectMetadataEvent(t, createObjects, gvr, expected)
}
