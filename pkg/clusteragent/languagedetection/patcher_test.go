// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	eventuallyTestTimeout = 2 * time.Second
	eventuallyTestTick    = 100 * time.Millisecond
)

func newMockLanguagePatcher(ctx context.Context, mockClient dynamic.Interface, mockStore workloadmetaimpl.Mock, mockLogger log.Mock) languagePatcher {
	ctx, cancel := context.WithCancel(ctx)

	return languagePatcher{
		ctx:       ctx,
		cancel:    cancel,
		k8sClient: mockClient,
		store:     mockStore,
		logger:    mockLogger,
		queue: workqueue.NewRateLimitingQueue(
			workqueue.NewItemExponentialFailureRateLimiter(
				1*time.Second,
				4*time.Second,
			),
		),
	}
}

// TestRun tests that the patcher object runs as expected
func TestRun(t *testing.T) {

	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mockStore := fxutil.Test[workloadmetaimpl.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetaimpl.MockModuleV2(),
	))
	mocklogger := fxutil.Test[log.Component](t, logimpl.MockModule())

	ctx := context.Background()
	lp := newMockLanguagePatcher(ctx, mockK8sClient, mockStore, mocklogger)

	go lp.run(ctx)
	defer lp.cancel()

	deploymentName := "test-deployment"
	longContNameDeploymentName := "test-deployment-long-cont-name"
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
					"internal.dd.datadoghq.com/some-cont.detected_langs":  "java",
					"internal.dd.datadoghq.com/stale-cont.detected_langs": "java,python",
				},
			},
			"spec": map[string]interface{}{},
		},
	}

	// Create  long container name deployment
	longContNameDeployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":        longContNameDeploymentName,
				"namespace":   ns,
				"annotations": map[string]interface{}{},
			},
			"spec": map[string]interface{}{},
		},
	}
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	_, err := mockK8sClient.Resource(gvr).Namespace(ns).Create(context.TODO(), deploymentObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	_, err = mockK8sClient.Resource(gvr).Namespace(ns).Create(context.TODO(), longContNameDeployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	////////////////////////////////
	//                            //
	//     Handling Set Event     //
	//                            //
	////////////////////////////////

	mockStore.Push("kubeapiserver", workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "test-namespace/test-deployment",
			},
			InjectableLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("some-cont"):  {"java": {}},
				*langUtil.NewContainer("stale-cont"): {"java": {}, "python": {}},
			},
		}})

	mockDeploymentEventToFail := workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "test-namespace/" + longContNameDeploymentName,
			},
			DetectedLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("some-cont"):            {"java": {}, "python": {}},
				*langUtil.NewInitContainer("python-ruby-init"): {"ruby": {}, "python": {}},
				// The max allowed annotation key name length in kubernetes is 63
				// To test that validation works, we are using a container name of length 69
				*langUtil.NewInitContainer(strings.Repeat("x", 69)): {"ruby": {}, "python": {}},
			},
		},
	}

	mockStore.Push(workloadmeta.SourceLanguageDetectionServer, mockDeploymentEventToFail)

	mockDeploymentEventToSucceed := workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "test-namespace/test-deployment",
			},
			DetectedLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("some-cont"):            {"java": {}, "python": {}},
				*langUtil.NewInitContainer("python-ruby-init"): {"ruby": {}, "python": {}},
			},
		},
	}

	mockStore.Push(workloadmeta.SourceLanguageDetectionServer, mockDeploymentEventToSucceed)

	expectedAnnotations := map[string]string{
		"internal.dd.datadoghq.com/some-cont.detected_langs":             "java,python",
		"internal.dd.datadoghq.com/init.python-ruby-init.detected_langs": "python,ruby",
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
	}

	checkDeploymentAnnotations := func() bool {
		// Check the patch
		got, err := lp.k8sClient.Resource(gvr).Namespace(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false
		}

		annotations := got.GetAnnotations()

		return reflect.DeepEqual(expectedAnnotations, annotations)
	}

	assert.Eventuallyf(t,
		checkDeploymentAnnotations,
		1*time.Second,
		10*time.Millisecond,
		"deployment should be patched with the correct annotations",
	)

	// Check that the deployment with long container name was not patched
	// This is correct since workloadmeta events are processed sequentially, which means that since the second event has been asserted first
	// the first event has already been processed and its side-effect can be asserted instantly
	assert.Truef(t, func() bool {
		// Check the patch
		got, err := lp.k8sClient.Resource(gvr).Namespace(ns).Get(context.TODO(), longContNameDeploymentName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		annotations := got.GetAnnotations()

		return len(annotations) == 0
	}(), "Deployment should not be patched with language annotations since one of the containers has a very long name")

	// Simulate kubeapiserver collector (i.e. update injectable languages in wlm)
	mockDeploymentEvent := workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "test-namespace/test-deployment",
			},
			InjectableLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("some-cont"):            {"java": {}, "python": {}},
				*langUtil.NewInitContainer("python-ruby-init"): {"ruby": {}, "python": {}},
			},
		},
	}

	mockStore.Push("kubeapiserver", mockDeploymentEvent)

	assert.Eventuallyf(t,
		func() bool {
			deployment, err := mockStore.GetKubernetesDeployment(fmt.Sprintf("%s/%s", ns, deploymentName))
			if err != nil {
				return false
			}

			return reflect.DeepEqual(deployment.InjectableLanguages, langUtil.ContainersLanguages{
				*langUtil.NewContainer("some-cont"):            {"java": {}, "python": {}},
				*langUtil.NewInitContainer("python-ruby-init"): {"ruby": {}, "python": {}},
			}) && reflect.DeepEqual(deployment.DetectedLanguages, langUtil.ContainersLanguages{
				*langUtil.NewContainer("some-cont"):            {"java": {}, "python": {}},
				*langUtil.NewInitContainer("python-ruby-init"): {"ruby": {}, "python": {}},
			})
		},
		eventuallyTestTimeout,
		eventuallyTestTick,
		"Should find deploymentA in workloadmeta store with the correct languages")

	////////////////////////////////
	//                            //
	//    Handling Unset Event    //
	//                            //
	////////////////////////////////

	mockDeploymentUnsetEvent := workloadmeta.Event{
		Type: workloadmeta.EventTypeUnset,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "test-namespace/test-deployment",
			},
		},
	}

	mockStore.Push(workloadmeta.SourceLanguageDetectionServer, mockDeploymentUnsetEvent)

	expectedAnnotations = map[string]string{
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
	}

	checkDeploymentAnnotations = func() bool {
		// Check the patch
		got, err := lp.k8sClient.Resource(gvr).Namespace(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if err != nil {
			return false
		}

		annotations, found, err := unstructured.NestedStringMap(got.Object, "metadata", "annotations")
		if err != nil || !found {
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

func TestPatcherRetriesFailedPatches(t *testing.T) {
	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	mockStore := fxutil.Test[workloadmetaimpl.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetaimpl.MockModuleV2(),
	))
	mocklogger := fxutil.Test[log.Component](t, logimpl.MockModule())

	ctx := context.Background()
	lp := newMockLanguagePatcher(ctx, mockK8sClient, mockStore, mocklogger)

	go lp.run(ctx)
	defer lp.cancel()

	deploymentName := "test-deployment"
	ns := "test-namespace"

	// Create  long container name deployment
	longContNameDeployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":        deploymentName,
				"namespace":   ns,
				"annotations": map[string]interface{}{},
			},
			"spec": map[string]interface{}{},
		},
	}

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	_, err := mockK8sClient.Resource(gvr).Namespace(ns).Create(context.TODO(), longContNameDeployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	mockDeploymentEventToFail := workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   ns + "/" + deploymentName,
			},
			DetectedLanguages: map[langUtil.Container]langUtil.LanguageSet{
				*langUtil.NewContainer("some-cont"):            {"java": {}, "python": {}},
				*langUtil.NewInitContainer("python-ruby-init"): {"ruby": {}, "python": {}},
				// The max allowed annotation key name length in kubernetes is 63
				// To test that failed patches are retried, we are using a container name of length 69
				*langUtil.NewInitContainer(strings.Repeat("x", 69)): {"ruby": {}, "python": {}},
			},
		},
	}

	mockStore.Push(workloadmeta.SourceLanguageDetectionServer, mockDeploymentEventToFail)

	owner := langUtil.NamespacedOwnerReference{
		Name:       deploymentName,
		APIVersion: "apps/v1",
		Kind:       langUtil.KindDeployment,
		Namespace:  ns,
	}

	assert.Eventuallyf(t, func() bool { return lp.queue.NumRequeues(owner) >= 1 }, 2*time.Second, 20*time.Millisecond, "Patching should have been retried")
}
