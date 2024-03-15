// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"context"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"

	"go.uber.org/fx"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newMockLanguagePatcher(mockClient dynamic.Interface, mockStore workloadmeta.Mock) LanguagePatcher {
	return LanguagePatcher{
		k8sClient: mockClient,
		store:     mockStore,
	}
}

func TestGetContainersLanguagesFromPodDetail(t *testing.T) {

	lp := newMockLanguagePatcher(nil, nil)

	containerDetails := []*pbgo.ContainerLanguageDetails{
		{
			ContainerName: "mono-lang",
			Languages: []*pbgo.Language{
				{Name: "java"},
			},
		},
		{
			ContainerName: "bi-lang",
			Languages: []*pbgo.Language{
				{Name: "java"},
				{Name: "cpp"},
			},
		},
		{
			ContainerName: "tri-lang",
			Languages: []*pbgo.Language{
				{Name: "java"},
				{Name: "go"},
				{Name: "python"},
			},
		},
	}

	podLanguageDetails := &pbgo.PodLanguageDetails{
		Namespace:        "default",
		ContainerDetails: containerDetails,
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   "dummyId",
			Kind: "ReplicaSet",
			Name: "dummyrs-2342347",
		},
	}

	containerslanguages := lp.getContainersLanguagesFromPodDetail(podLanguageDetails)

	expectedContainersLanguages := langUtil.NewContainersLanguages()

	expectedContainersLanguages.GetOrInitializeLanguageset("mono-lang").Parse("java")
	expectedContainersLanguages.GetOrInitializeLanguageset("bi-lang").Parse("java,cpp")
	expectedContainersLanguages.GetOrInitializeLanguageset("tri-lang").Parse("java,go,python")

	assert.True(t, reflect.DeepEqual(containerslanguages, expectedContainersLanguages))
}

func TestGetOwnersLanguages(t *testing.T) {
	lp := newMockLanguagePatcher(nil, nil)

	defaultNs := "default"
	customNs := "custom"

	podALanguageDetails := &pbgo.PodLanguageDetails{
		Namespace: defaultNs,
		Name:      "pod-a",
		ContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-1",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "cpp"},
					{Name: "go"},
				},
			},
			{
				ContainerName: "container-2",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "python"},
				},
			},
		},
		InitContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-3",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "cpp"},
				},
			},
			{
				ContainerName: "container-4",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "python"},
				},
			},
		},
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   "dummyId-1",
			Kind: "ReplicaSet",
			Name: "dummyrs-1-2342347",
		},
	}

	podBLanguageDetails := &pbgo.PodLanguageDetails{
		Namespace: customNs,
		Name:      "pod-b",
		ContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-5",
				Languages: []*pbgo.Language{
					{Name: "python"},
					{Name: "cpp"},
					{Name: "go"},
				},
			},
			{
				ContainerName: "container-6",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "ruby"},
				},
			},
		},
		InitContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-7",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "cpp"},
				},
			},
			{
				ContainerName: "container-8",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "python"},
				},
			},
		},
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   "dummyId-2",
			Kind: "ReplicaSet",
			Name: "dummyrs-2-2342347",
		},
	}

	mockRequestData := &pbgo.ParentLanguageAnnotationRequest{
		PodDetails: []*pbgo.PodLanguageDetails{
			podALanguageDetails,
			podBLanguageDetails,
		},
	}

	expectedContainersLanguagesA := langUtil.NewContainersLanguages()

	expectedContainersLanguagesA.GetOrInitializeLanguageset("container-1").Parse("java,cpp,go")
	expectedContainersLanguagesA.GetOrInitializeLanguageset("container-2").Parse("java,python")
	expectedContainersLanguagesA.GetOrInitializeLanguageset("init.container-3").Parse("java,cpp")
	expectedContainersLanguagesA.GetOrInitializeLanguageset("init.container-4").Parse("java,python")

	expectedContainersLanguagesB := langUtil.NewContainersLanguages()

	expectedContainersLanguagesB.GetOrInitializeLanguageset("container-5").Parse("python,cpp,go")
	expectedContainersLanguagesB.GetOrInitializeLanguageset("container-6").Parse("java,ruby")
	expectedContainersLanguagesB.GetOrInitializeLanguageset("init.container-7").Parse("java,cpp")
	expectedContainersLanguagesB.GetOrInitializeLanguageset("init.container-8").Parse("java,python")

	expectedOwnersLanguages := &OwnersLanguages{
		NewNamespacedOwnerReference("apps/v1", "Deployment", "dummyrs-1", "dummyId-1", "default"): expectedContainersLanguagesA,
		NewNamespacedOwnerReference("apps/v1", "Deployment", "dummyrs-2", "dummyId-2", "custom"):  expectedContainersLanguagesB,
	}

	actualOwnersLanguages := lp.getOwnersLanguages(mockRequestData)

	assert.True(t, reflect.DeepEqual(expectedOwnersLanguages, actualOwnersLanguages))

}

func TestDetectedNewLanguages(t *testing.T) {
	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))

	mockStore.Set(&workloadmeta.KubernetesDeployment{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesDeployment,
			ID:   "default/dummy",
		},
		ContainerLanguages: map[string][]languagemodels.Language{
			"container-1": {
				{
					Name:    "go",
					Version: "1.2",
				},
			},
		},
		InitContainerLanguages: map[string][]languagemodels.Language{
			"container-2": {
				{
					Name:    "java",
					Version: "18",
				},
			},
		},
	})

	namespacedOwnerReference := NewNamespacedOwnerReference("apps/v1", "deployment", "dummy", "uid-122", "default")

	lp := newMockLanguagePatcher(nil, mockStore)

	detectedContainersLanguages := langUtil.NewContainersLanguages()

	detectedContainersLanguages.GetOrInitializeLanguageset("container-1").Add("go")
	detectedContainersLanguages.GetOrInitializeLanguageset("init.container-2").Add("java")

	assert.False(t, lp.detectedNewLanguages(&namespacedOwnerReference, detectedContainersLanguages))

	detectedContainersLanguages.GetOrInitializeLanguageset("container-1").Add("ruby")
	detectedContainersLanguages.GetOrInitializeLanguageset("container-3").Add("cpp")

	assert.True(t, lp.detectedNewLanguages(&namespacedOwnerReference, detectedContainersLanguages))
}

func TestPatchOwner(t *testing.T) {

	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	lp := newMockLanguagePatcher(mockK8sClient, mockStore)

	deploymentName := "test-deployment"
	ns := "test-namespace"
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	namespacedOwnerReference := NewNamespacedOwnerReference("apps/v1", "Deployment", deploymentName, "uid-dummy", ns)

	mockContainersLanguages := langUtil.NewContainersLanguages()
	mockContainersLanguages.GetOrInitializeLanguageset("container-1").Parse("cpp,java,python")
	mockContainersLanguages.GetOrInitializeLanguageset("container-2").Parse("python,ruby")
	mockContainersLanguages.GetOrInitializeLanguageset("container-3").Parse("cpp")
	mockContainersLanguages.GetOrInitializeLanguageset("container-4").Parse("")

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
				},
			},
			"spec": map[string]interface{}{},
		},
	}
	_, err := mockK8sClient.Resource(gvr).Namespace(ns).Create(context.TODO(), deploymentObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Apply the patch
	assert.NoError(t, lp.patchOwner(&namespacedOwnerReference, mockContainersLanguages))

	// Check the patch
	got, err := lp.k8sClient.Resource(gvr).Namespace(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err := unstructured.NestedStringMap(got.Object, "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)

	expectedAnnotations := map[string]string{
		"internal.dd.datadoghq.com/container-1.detected_langs": "cpp,java,python",
		"internal.dd.datadoghq.com/container-2.detected_langs": "python,ruby",
		"internal.dd.datadoghq.com/container-3.detected_langs": "cpp",
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
	}

	assert.True(t, reflect.DeepEqual(expectedAnnotations, annotations))
}

func TestPatchAllOwners(t *testing.T) {
	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	lp := newMockLanguagePatcher(mockK8sClient, mockStore)

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	// Mock definition for deployment A
	deploymentAName := "test-deployment-A"
	nsA := "test-namespace-A"

	podALanguageDetails := &pbgo.PodLanguageDetails{
		Namespace: nsA,
		Name:      "pod-a",
		ContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-1",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "cpp"},
					{Name: "python"},
				},
			},
			{
				ContainerName: "container-2",
				Languages: []*pbgo.Language{
					{Name: "ruby"},
					{Name: "python"},
				},
			},
		},
		InitContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-3",
				Languages: []*pbgo.Language{
					{Name: "cpp"},
				},
			},
			{
				ContainerName: "container-4",
				Languages:     []*pbgo.Language{},
			},
		},
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   "dummyId-1",
			Kind: "ReplicaSet",
			Name: "test-deployment-A-2342347",
		},
	}

	// Mock definition for deployment B
	deploymentBName := "test-deployment-B"
	nsB := "test-namespace-B"

	podBLanguageDetails := &pbgo.PodLanguageDetails{
		Namespace: nsB,
		Name:      "pod-b",
		ContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-1",
				Languages: []*pbgo.Language{
					{Name: "python"},
				},
			},
			{
				ContainerName: "container-2",
				Languages: []*pbgo.Language{
					{Name: "golang"},
				},
			},
		},
		InitContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-3",
				Languages: []*pbgo.Language{
					{Name: "cpp"},
					{Name: "java"},
				},
			},
			{
				ContainerName: "container-4",
				Languages:     []*pbgo.Language{},
			},
		},
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   "dummyId-2",
			Kind: "ReplicaSet",
			Name: "test-deployment-B-2342347",
		},
	}

	// Create target deployment A
	deploymentAObject := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      deploymentAName,
				"namespace": nsA,
				"annotations": map[string]interface{}{
					"annotationkey1": "annotationvalue1",
					"annotationkey2": "annotationvalue2",
					"internal.dd.datadoghq.com/container-1.detected_langs": "java,python",
					"internal.dd.datadoghq.com/container-2.detected_langs": "python",
				},
			},
			"spec": map[string]interface{}{},
		},
	}
	_, err := mockK8sClient.Resource(gvr).Namespace(nsA).Create(context.TODO(), deploymentAObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create target deployment B
	deploymentBObject := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      deploymentBName,
				"namespace": nsB,
			},
			"spec": map[string]interface{}{},
		},
	}
	_, err = mockK8sClient.Resource(gvr).Namespace(nsB).Create(context.TODO(), deploymentBObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	mockRequestData := &pbgo.ParentLanguageAnnotationRequest{
		PodDetails: []*pbgo.PodLanguageDetails{
			podALanguageDetails,
			podBLanguageDetails,
		},
	}

	// Apply the patches to all owners
	lp.PatchAllOwners(mockRequestData)

	// Check the patch of owner A
	got, err := lp.k8sClient.Resource(gvr).Namespace(nsA).Get(context.TODO(), deploymentAName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err := unstructured.NestedStringMap(got.Object, "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)

	expectedAnnotationsA := map[string]string{
		"internal.dd.datadoghq.com/container-1.detected_langs":      "cpp,java,python",
		"internal.dd.datadoghq.com/container-2.detected_langs":      "python,ruby",
		"internal.dd.datadoghq.com/init.container-3.detected_langs": "cpp",
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
	}

	assert.True(t, reflect.DeepEqual(expectedAnnotationsA, annotations))

	// Check the patch of owner B
	got, err = lp.k8sClient.Resource(gvr).Namespace(nsB).Get(context.TODO(), deploymentBName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err = unstructured.NestedStringMap(got.Object, "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)

	expectedAnnotationsB := map[string]string{
		"internal.dd.datadoghq.com/container-1.detected_langs":      "python",
		"internal.dd.datadoghq.com/container-2.detected_langs":      "golang",
		"internal.dd.datadoghq.com/init.container-3.detected_langs": "cpp,java",
	}

	assert.True(t, reflect.DeepEqual(expectedAnnotationsB, annotations))

}
