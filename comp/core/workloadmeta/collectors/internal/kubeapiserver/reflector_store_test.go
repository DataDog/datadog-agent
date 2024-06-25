// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var timeout = 10 * time.Second
var interval = 50 * time.Millisecond

func Test_AddDelete_Deployment(t *testing.T) {
	workloadmetaComponent := mockedWorkloadmeta(t)

	deploymentStore := newDeploymentReflectorStore(workloadmetaComponent)

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
	}

	err := deploymentStore.Add(&deployment)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesDeployment("test-namespace/test-deployment")
		return err == nil
	}, timeout, interval)

	err = deploymentStore.Delete(&deployment)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesDeployment("test-namespace/test-deployment")
		return err != nil
	}, timeout, interval)
}

func Test_AddDelete_Pod(t *testing.T) {
	workloadmetaComponent := mockedWorkloadmeta(t)

	podStore := newPodReflectorStore(workloadmetaComponent)

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       "pod-uid",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
	}

	err := podStore.Add(&pod)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesPod(string(pod.UID))
		return err == nil
	}, timeout, interval)

	err = podStore.Delete(&pod)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesDeployment(string(pod.UID))
		return err != nil
	}, timeout, interval)
}

func Test_AddDelete_PartialObjectMetadata(t *testing.T) {
	workloadmetaComponent := mockedWorkloadmeta(t)

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}
	parser, err := newMetadataParser(gvr, nil)
	require.NoError(t, err)

	metadataStore := &reflectorStore{
		wlmetaStore: workloadmetaComponent,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
	}

	partialObjMetadata := metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
	}

	kubeMetadataEntityID := util.GenerateKubeMetadataEntityID("namespaces", "", "test-object")

	err = metadataStore.Add(&partialObjMetadata)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesMetadata(kubeMetadataEntityID)
		return err == nil
	}, timeout, interval)

	err = metadataStore.Delete(&partialObjMetadata)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err = workloadmetaComponent.GetKubernetesMetadata(kubeMetadataEntityID)
		return err != nil
	}, timeout, interval)
}

func mockedWorkloadmeta(t *testing.T) workloadmeta.Component {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))
}
