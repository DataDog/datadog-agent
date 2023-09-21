// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1

import (
	"context"
	"reflect"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/languagedetection"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestGetContainersLanguagesFromPodDetail(t *testing.T) {

	lp := &languagePatcher{
		k8sClient: nil,
	}

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
			Kind: "replicaset",
			Name: "dummyrs-2342347",
		},
	}

	containerslanguages := lp.getContainersLanguagesFromPodDetail(podLanguageDetails)

	expectedContainersLanguages := languagedetection.NewContainersLanguages()

	expectedContainersLanguages.Parse("mono-lang", "java")
	expectedContainersLanguages.Parse("bi-lang", "java,cpp")
	expectedContainersLanguages.Parse("tri-lang", "java,go,python")

	assert.True(t, reflect.DeepEqual(containerslanguages, expectedContainersLanguages))
}

func TestGetOwnersLanguages(t *testing.T) {
	lp := &languagePatcher{
		k8sClient: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
	}

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
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   "dummyId-1",
			Kind: "replicaset",
			Name: "dummyrs-1-2342347",
		},
	}

	podBLanguageDetails := &pbgo.PodLanguageDetails{
		Namespace: customNs,
		Name:      "pod-b",
		ContainerDetails: []*pbgo.ContainerLanguageDetails{
			{
				ContainerName: "container-3",
				Languages: []*pbgo.Language{
					{Name: "python"},
					{Name: "cpp"},
					{Name: "go"},
				},
			},
			{
				ContainerName: "container-4",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "ruby"},
				},
			},
		},
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   "dummyId-2",
			Kind: "replicaset",
			Name: "dummyrs-2-2342347",
		},
	}

	mockRequestData := &pbgo.ParentLanguageAnnotationRequest{
		PodDetails: []*pbgo.PodLanguageDetails{
			podALanguageDetails,
			podBLanguageDetails,
		},
	}

	expectedContainersLanguagesA := languagedetection.NewContainersLanguages()

	expectedContainersLanguagesA.Parse("container-1", "java,cpp,go")
	expectedContainersLanguagesA.Parse("container-2", "java,python")

	expectedContainersLanguagesB := languagedetection.NewContainersLanguages()

	expectedContainersLanguagesB.Parse("container-3", "python,cpp,go")
	expectedContainersLanguagesB.Parse("container-4", "java,ruby")

	expectedOwnersLanguages := &OwnersLanguages{
		*NewOwnerInfo("dummyrs-1", "default", "deployment"): expectedContainersLanguagesA,
		*NewOwnerInfo("dummyrs-2", "custom", "deployment"):  expectedContainersLanguagesB,
	}

	actualOwnersLanguages := lp.getOwnersLanguages(mockRequestData)

	assert.True(t, reflect.DeepEqual(expectedOwnersLanguages, actualOwnersLanguages))

}

func TestUpdatedOwnerAnnotations(t *testing.T) {
	lp := &languagePatcher{
		k8sClient: nil,
	}

	mockContainersLanguages := languagedetection.NewContainersLanguages()
	mockContainersLanguages.Parse("container-1", "cpp,java,python")
	mockContainersLanguages.Parse("container-2", "python,ruby")
	mockContainersLanguages.Parse("container-3", "cpp")
	mockContainersLanguages.Parse("container-4", "")

	// Case of existing annotations
	mockCurrentAnnotations := map[string]string{
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
		"apm.datadoghq.com/container-1.languages": "java,python",
		"apm.datadoghq.com/container-2.languages": "cpp",
	}

	expectedUpdatedAnnotations := map[string]string{
		"annotationkey1": "annotationvalue1",
		"annotationkey2": "annotationvalue2",
		"apm.datadoghq.com/container-1.languages": "cpp,java,python",
		"apm.datadoghq.com/container-2.languages": "cpp,python,ruby",
		"apm.datadoghq.com/container-3.languages": "cpp",
	}

	expectedAddedLanguages := 4

	actualUpdatedAnnotations, actualAddedLanguages := lp.getUpdatedOwnerAnnotations(mockCurrentAnnotations, mockContainersLanguages)

	assert.Equal(t, expectedAddedLanguages, actualAddedLanguages)
	assert.Equal(t, expectedUpdatedAnnotations, actualUpdatedAnnotations)

	// Case of non-existing annotations
	mockCurrentAnnotations = nil

	expectedUpdatedAnnotations = map[string]string{
		"apm.datadoghq.com/container-1.languages": "cpp,java,python",
		"apm.datadoghq.com/container-2.languages": "python,ruby",
		"apm.datadoghq.com/container-3.languages": "cpp",
	}

	expectedAddedLanguages = 6

	actualUpdatedAnnotations, actualAddedLanguages = lp.getUpdatedOwnerAnnotations(mockCurrentAnnotations, mockContainersLanguages)

	assert.Equal(t, expectedAddedLanguages, actualAddedLanguages)
	assert.Equal(t, expectedUpdatedAnnotations, actualUpdatedAnnotations)

}

func TestPatchOwner(t *testing.T) {

	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	lp := &languagePatcher{
		k8sClient: mockK8sClient,
	}

	deploymentName := "test-deployment"
	ns := "test-namespace"
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	ownerinfo := NewOwnerInfo(deploymentName, ns, "deployment")

	mockContainersLanguages := languagedetection.NewContainersLanguages()
	mockContainersLanguages.Parse("container-1", "cpp,java,python")
	mockContainersLanguages.Parse("container-2", "python,ruby")
	mockContainersLanguages.Parse("container-3", "cpp")
	mockContainersLanguages.Parse("container-4", "")

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
					"apm.datadoghq.com/container-1.languages": "java,python",
					"apm.datadoghq.com/container-2.languages": "cpp",
				},
			},
			"spec": map[string]interface{}{},
		},
	}
	_, err := mockK8sClient.Resource(gvr).Namespace(ns).Create(context.TODO(), deploymentObject, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Apply the patch
	assert.NoError(t, lp.patchOwner(ownerinfo, mockContainersLanguages))

	// Check the patch
	got, err := lp.k8sClient.Resource(gvr).Namespace(ns).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err := unstructured.NestedStringMap(got.Object, "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)

	// Assert correct number of annotations
	assert.Equal(t, 5, len(annotations))

	//Assert that language annotation are correctly set
	assert.Equal(t, annotations["apm.datadoghq.com/container-1.languages"], "cpp,java,python")
	assert.Equal(t, annotations["apm.datadoghq.com/container-2.languages"], "cpp,python,ruby")
	assert.Equal(t, annotations["apm.datadoghq.com/container-3.languages"], "cpp")

	//Assert that other annotations are not modified
	assert.Equal(t, annotations["annotationkey1"], "annotationvalue1")
	assert.Equal(t, annotations["annotationkey2"], "annotationvalue2")
}

func TestPatchAllOwners(t *testing.T) {
	mockK8sClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

	lp := &languagePatcher{
		k8sClient: mockK8sClient,
	}

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	// Mock definition for deployment A
	deploymentAName := "test-deployment-A"
	nsA := "test-namespace-A"
	ownerAinfo := NewOwnerInfo(deploymentAName, nsA, "deployment")

	// Define mock containers languages for deployment A
	mockContainersLanguagesA := languagedetection.NewContainersLanguages()
	mockContainersLanguagesA.Parse("container-1", "cpp,java,python")
	mockContainersLanguagesA.Parse("container-2", "python,ruby")
	mockContainersLanguagesA.Parse("container-3", "cpp")
	mockContainersLanguagesA.Parse("container-4", "")

	// Mock definition for deployment B
	deploymentBName := "test-deployment-B"
	nsB := "test-namespace-B"
	ownerBinfo := NewOwnerInfo(deploymentBName, nsB, "deployment")

	// Define mock containers languages for deployment B
	mockContainersLanguagesB := languagedetection.NewContainersLanguages()
	mockContainersLanguagesB.Parse("container-1", "python")
	mockContainersLanguagesB.Parse("container-2", "golang")
	mockContainersLanguagesB.Parse("container-3", "cpp,java")
	mockContainersLanguagesB.Parse("container-4", "")

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
					"apm.datadoghq.com/container-1.languages": "java,python",
					"apm.datadoghq.com/container-2.languages": "cpp",
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

	ownerslanguages := &OwnersLanguages{
		*ownerAinfo: mockContainersLanguagesA,
		*ownerBinfo: mockContainersLanguagesB,
	}

	// Apply the patches to all owners
	lp.patchAllOwners(ownerslanguages)

	// Check the patch of owner A
	got, err := lp.k8sClient.Resource(gvr).Namespace(nsA).Get(context.TODO(), deploymentAName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err := unstructured.NestedStringMap(got.Object, "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)

	// Assert correct number of annotations
	assert.Equal(t, 5, len(annotations))

	//Assert that language annotation are correctly set
	assert.Equal(t, annotations["apm.datadoghq.com/container-1.languages"], "cpp,java,python")
	assert.Equal(t, annotations["apm.datadoghq.com/container-2.languages"], "cpp,python,ruby")
	assert.Equal(t, annotations["apm.datadoghq.com/container-3.languages"], "cpp")

	//Assert that other annotations are not modified
	assert.Equal(t, annotations["annotationkey1"], "annotationvalue1")
	assert.Equal(t, annotations["annotationkey2"], "annotationvalue2")

	// Check the patch of owner B
	got, err = lp.k8sClient.Resource(gvr).Namespace(nsB).Get(context.TODO(), deploymentBName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err = unstructured.NestedStringMap(got.Object, "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)

	// Assert correct number of annotations
	assert.Equal(t, 3, len(annotations))

	// Assert that language annotation are correctly set
	assert.Equal(t, annotations["apm.datadoghq.com/container-1.languages"], "python")
	assert.Equal(t, annotations["apm.datadoghq.com/container-2.languages"], "golang")
	assert.Equal(t, annotations["apm.datadoghq.com/container-3.languages"], "cpp,java")
}
