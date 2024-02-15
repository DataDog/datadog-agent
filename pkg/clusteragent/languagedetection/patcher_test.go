// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"reflect"
	"testing"
	"time"
)

func newMockLanguagePatcher(ctx context.Context, mockClient dynamic.Interface, mockStore workloadmeta.Mock, mockLogger log.Mock) languagePatcher {
	ctx, cancel := context.WithCancel(ctx)

	return languagePatcher{
		ctx:       ctx,
		cancel:    cancel,
		k8sClient: mockClient,
		store:     mockStore,
		logger:    mockLogger,
	}
}

func TestGenerateAnnotationsPatch(t *testing.T) {

	ctx := context.Background()
	lp := newMockLanguagePatcher(ctx, nil, nil, nil)

	tests := []struct {
		name                        string
		currentContainersLanguages  langUtil.ContainersLanguages
		detectedContainersLanguages langUtil.ContainersLanguages
		expectedAnnotationsPatch    map[string]interface{}
	}{
		{
			name: "stale containers, overrides, and new containers",
			currentContainersLanguages: langUtil.ContainersLanguages{
				*langUtil.NewContainer("cont-1"):     {"java": {}},
				*langUtil.NewContainer("cont-2"):     {"java": {}},
				*langUtil.NewInitContainer("cont-1"): {"java": {}},
				*langUtil.NewInitContainer("cont-2"): {"java": {}},
			},

			detectedContainersLanguages: langUtil.ContainersLanguages{
				*langUtil.NewContainer("cont-1"):     {"java": {}, "python": {}},
				*langUtil.NewInitContainer("cont-1"): {"go": {}},
				*langUtil.NewContainer("cont-3"):     {"java": {}},
			},

			expectedAnnotationsPatch: map[string]interface{}{
				"internal.dd.datadoghq.com/cont-1.detected_langs":      "java,python",
				"internal.dd.datadoghq.com/cont-2.detected_langs":      nil,
				"internal.dd.datadoghq.com/cont-3.detected_langs":      "java",
				"internal.dd.datadoghq.com/init.cont-1.detected_langs": "go",
				"internal.dd.datadoghq.com/init.cont-2.detected_langs": nil,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			annotationsPatch := lp.generateAnnotationsPatch(test.currentContainersLanguages, test.detectedContainersLanguages)
			assert.Truef(tt,
				reflect.DeepEqual(
					test.expectedAnnotationsPatch,
					annotationsPatch,
				),
				"annotations patch not correct, expected %v but found %v",
				test.expectedAnnotationsPatch,
				annotationsPatch,
			)
		})
	}

}

func TestHandleDeploymentEvent(t *testing.T) {

	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	lp := newMockLanguagePatcher(ctx, mockK8sClient, nil, nil)

	deploymentName := "test-deployment"
	ns := "test-namespace"

	// Create target deployment
	deploymentObject := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      deploymentName,
				"namespace": ns,
				"annotations": map[string]interface{}{
					"annotationkey1": "annotationvalue1",
					"annotationkey2": "annotationvalue2",
					"internal.dd.datadoghq.com/python-cont.detected_langs": "java,python",
					"internal.dd.datadoghq.com/stale-cont.detected_langs":  "java,python",
				},
			},
			"spec": map[string]interface{}{},
		},
	}
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	_, err := mockK8sClient.Resource(gvr).Namespace(ns).Create(context.TODO(), deploymentObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	mockDeploymentEvent := workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "test-namespace/test-deployment",
			},
			DetectedLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("python-cont"):          {"python": {}},
				*langUtil.NewInitContainer("python-ruby-init"): {"python": {}, "ruby": {}},
			},
			InjectableLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("stale-cont"): {"java": {}, "python": {}},
			},
		},
	}

	// Apply the patch
	lp.handleDeploymentEvent(mockDeploymentEvent)

	expectedAnnotations := map[string]string{
		"internal.dd.datadoghq.com/python-cont.detected_langs":           "python",
		"internal.dd.datadoghq.com/init.python-ruby-init.detected_langs": "python,ruby",
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
	}

	checkDeploymentAnnotations := func() bool {
		// Check the patch
		got, err := lp.k8sClient.Resource(gvr).Namespace(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("deployment not found")
			return false
		}

		annotations, found, err := unstructured.NestedStringMap(got.Object, "metadata", "annotations")
		if err != nil || !found {
			fmt.Println("couldn't get annotations")
			return false
		}

		return reflect.DeepEqual(expectedAnnotations, annotations)
	}

	assert.Eventuallyf(t,
		checkDeploymentAnnotations,
		1*time.Second,
		10*time.Millisecond,
		"deployment should be patched with the correct annotations",
	)
}

func TestPatchOwner(t *testing.T) {

	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	ctx := context.Background()
	lp := newMockLanguagePatcher(ctx, mockK8sClient, mockStore, nil)

	deploymentName := "test-deployment"
	ns := "test-namespace"
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	namespacedOwnerReference := langUtil.NewNamespacedOwnerReference("apps/v1", "Deployment", deploymentName, ns)

	// Create target deployment
	deploymentObject := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      deploymentName,
				"namespace": ns,
				"annotations": map[string]interface{}{
					"annotationkey1": "annotationvalue1",
					"annotationkey2": "annotationvalue2",
					"internal.dd.datadoghq.com/container-1.detected_langs": "java,python",
					"internal.dd.datadoghq.com/container-2.detected_langs": "cpp",
					"internal.dd.datadoghq.com/container-3.detected_langs": "ruby",
				},
			},
			"spec": map[string]interface{}{},
		},
	}
	_, err := mockK8sClient.Resource(gvr).Namespace(ns).Create(context.TODO(), deploymentObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	mockAnnotations := map[string]interface{}{
		"internal.dd.datadoghq.com/container-1.detected_langs": "cpp,java,python",
		"internal.dd.datadoghq.com/container-2.detected_langs": "python,ruby",
		"internal.dd.datadoghq.com/container-3.detected_langs": nil,
		"internal.dd.datadoghq.com/container-4.detected_langs": "cpp",
	}

	// Apply the patch
	lp.patchOwner(&namespacedOwnerReference, mockAnnotations)

	// Check the patch
	got, err := lp.k8sClient.Resource(gvr).Namespace(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err := unstructured.NestedStringMap(got.Object, "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)

	expectedAnnotations := map[string]string{
		"internal.dd.datadoghq.com/container-1.detected_langs": "cpp,java,python",
		"internal.dd.datadoghq.com/container-2.detected_langs": "python,ruby",
		"internal.dd.datadoghq.com/container-4.detected_langs": "cpp",
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
	}

	assert.True(t, reflect.DeepEqual(expectedAnnotations, annotations))
}

func TestRun(t *testing.T) {

	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))

	mockLogger := fxutil.Test[log.Component](t, logimpl.MockModule())

	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	ctx := context.Background()
	lp := newMockLanguagePatcher(ctx, mockK8sClient, mockStore, mockLogger)

	deploymentName := "test-deployment"
	ns := "test-namespace"

	// Create target deployment
	deploymentObject := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      deploymentName,
				"namespace": ns,
				"annotations": map[string]interface{}{
					"annotationkey1": "annotationvalue1",
					"annotationkey2": "annotationvalue2",
					"internal.dd.datadoghq.com/cont-1.detected_langs":      "java,python",
					"internal.dd.datadoghq.com/init.cont-3.detected_langs": "ruby",
				},
			},
			"spec": map[string]interface{}{},
		},
	}
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	_, err := mockK8sClient.Resource(gvr).Namespace(ns).Create(context.TODO(), deploymentObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	mockStore.Push(workloadmeta.SourceLanguageDetectionServer, workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "test-namespace/test-deployment",
			},
			DetectedLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("cont-1"):     {"java": {}, "python": {}},
				*langUtil.NewInitContainer("cont-2"): {"java": {}, "python": {}},
			},
			InjectableLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("cont-1"):     {"java": {}},
				*langUtil.NewInitContainer("cont-3"): {"ruby": {}},
			},
		},
	})

	go lp.run()

	expectedAnnotations := map[string]string{
		"internal.dd.datadoghq.com/cont-1.detected_langs":      "java,python",
		"internal.dd.datadoghq.com/init.cont-2.detected_langs": "java,python",
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
	}

	checkDeploymentAnnotations := func() bool {
		// Check the patch
		got, err := lp.k8sClient.Resource(gvr).Namespace(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			fmt.Println("deployment not found")
			return false
		}

		annotations, found, err := unstructured.NestedStringMap(got.Object, "metadata", "annotations")
		if err != nil || !found {
			fmt.Println("couldn't get annotations")
			return false
		}

		return reflect.DeepEqual(expectedAnnotations, annotations)
	}

	assert.Eventuallyf(
		t,
		checkDeploymentAnnotations,
		1*time.Second,
		10*time.Millisecond,
		"deployment should be patched with the correct annotations",
	)

	lp.cancel()
}
