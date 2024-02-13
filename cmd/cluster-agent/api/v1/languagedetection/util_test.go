// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"reflect"
	"testing"
	"time"
)

////////////////////////////////
//                            //
//   Owners Languages Tests   //
//                            //
////////////////////////////////

func TestOwnersLanguagesGetOrInitialise(t *testing.T) {
	mockNamespacedOwnerRef := langUtil.NewNamespacedOwnerReference("api-version", "deployment", "some-name", "some-ns")
	tests := []struct {
		name            string
		ownersLanguages *OwnersLanguages
		ownerRef        langUtil.NamespacedOwnerReference
		expected        *containersLanguageWithDirtyFlag
	}{
		{
			name:            "missing owner should get initialized",
			ownersLanguages: newOwnersLanguages(),
			ownerRef:        mockNamespacedOwnerRef,
			expected:        newContainersLanguageWithDirtyFlag(),
		},
		{
			name: "should return containers languages for existing owner",
			ownersLanguages: &OwnersLanguages{
				containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
					mockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
								"java": {},
							},
						},
						dirty: false,
					},
				},
			},

			ownerRef: mockNamespacedOwnerRef,
			expected: &containersLanguageWithDirtyFlag{
				languages: langUtil.TimedContainersLanguages{
					*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
						"java": {},
					},
				},
				dirty: false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			actual := test.ownersLanguages.getOrInitialize(test.ownerRef)
			assert.True(tt, reflect.DeepEqual(actual, test.expected), fmt.Sprintf("Expected %v, found %v", test.expected, actual))
		})
	}
}

func TestOwnersLanguagesMerge(t *testing.T) {
	mockNamespacedOwnerRef := langUtil.NewNamespacedOwnerReference("api-version", "deployment", "some-name", "some-ns")
	otherMockNamespacedOwnerRef := langUtil.NewNamespacedOwnerReference("api-version", "statefulset", "some-name", "some-ns")
	cleanMockNamespacedOwnerRef := langUtil.NewNamespacedOwnerReference("api-version", "daemonset", "some-name", "some-ns")

	mockExpiration := time.Now()

	tests := []struct {
		name               string
		ownersLanguages    *OwnersLanguages
		other              *OwnersLanguages
		expectedAfterMerge *OwnersLanguages
	}{
		{
			name:               "merge empty owners languages",
			ownersLanguages:    newOwnersLanguages(),
			other:              newOwnersLanguages(),
			expectedAfterMerge: newOwnersLanguages(),
		},
		{
			name:            "merge non-empty other to empty self",
			ownersLanguages: newOwnersLanguages(),
			other: &OwnersLanguages{
				containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
					mockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
								"java": mockExpiration,
							},
						},
						dirty: false,
					},
				},
			},
			expectedAfterMerge: &OwnersLanguages{
				containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
					mockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
								"java": mockExpiration,
							},
						},
						dirty: true,
					},
				},
			},
		},
		{
			name: "merge empty other to non-empty self",
			ownersLanguages: &OwnersLanguages{
				containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
					mockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
								"java": {},
							},
						},
						dirty: false,
					},
				},
			},
			other: newOwnersLanguages(),
			expectedAfterMerge: &OwnersLanguages{
				containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
					mockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
								"java": {},
							},
						},
						dirty: false,
					},
				},
			},
		},
		{
			name: "merge non-empty other to non-empty self",
			ownersLanguages: &OwnersLanguages{
				containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
					mockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
								"java": {},
								"ruby": {},
							},
						},
						dirty: false,
					},
					cleanMockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-other-container"): {
								"java": {},
								"ruby": {},
							},
						},
						dirty: false,
					},
				},
			},
			other: &OwnersLanguages{
				containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
					mockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
								"perl": {},
							},
							*langUtil.NewContainer("some-other-container"): langUtil.TimedLanguageSet{
								"cpp": {},
							},
						},
					},
					otherMockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-other-container"): {
								"java": {},
								"cpp":  {},
							},
						},
					},
					cleanMockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-other-container"): {
								"java": mockExpiration,
								"ruby": mockExpiration,
							},
						},
					},
				},
			},
			expectedAfterMerge: &OwnersLanguages{
				containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
					mockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-container"): langUtil.TimedLanguageSet{
								"java": {},
								"ruby": {},
								"perl": {},
							},
							*langUtil.NewContainer("some-other-container"): langUtil.TimedLanguageSet{
								"cpp": {},
							},
						},
						dirty: true,
					},
					cleanMockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-other-container"): {
								"java": mockExpiration,
								"ruby": mockExpiration,
							},
						},
						dirty: false,
					},
					otherMockNamespacedOwnerRef: {
						languages: langUtil.TimedContainersLanguages{
							*langUtil.NewContainer("some-other-container"): {
								"java": {},
								"cpp":  {},
							},
						},
						dirty: true,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			test.ownersLanguages.merge(test.other)
			assert.True(tt, reflect.DeepEqual(test.ownersLanguages.containersLanguages, test.expectedAfterMerge.containersLanguages), fmt.Sprintf("Expected %v, found %v", test.expectedAfterMerge.containersLanguages, test.ownersLanguages.containersLanguages))
		})
	}
}

func TestOwnersLanguagesFlush(t *testing.T) {
	mockSupportedOwnerA := langUtil.NewNamespacedOwnerReference("api-version", langUtil.KindDeployment, "deploymentA", "ns")
	mockSupportedOwnerB := langUtil.NewNamespacedOwnerReference("api-version", langUtil.KindDeployment, "deploymentB", "ns")
	mockUnsupportedOwner := langUtil.NewNamespacedOwnerReference("api-version", "Daemonset", "some-name", "ns")
	mockExpiration := time.Now()

	ownersLanguages := OwnersLanguages{
		containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
			mockSupportedOwnerA: {
				languages: langUtil.TimedContainersLanguages{
					*langUtil.NewContainer("some-container"): {
						"java": mockExpiration,
						"ruby": mockExpiration,
						"perl": mockExpiration,
					},
				},
				dirty: true,
			},

			mockSupportedOwnerB: {
				languages: langUtil.TimedContainersLanguages{
					*langUtil.NewContainer("some-container"): {
						"java": mockExpiration,
					},
					*langUtil.NewContainer("some-other-container"): {
						"cpp": mockExpiration,
					},
				},
				dirty: false,
			},
		},
	}

	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))

	err := ownersLanguages.flush(mockStore)
	assert.NoErrorf(t, err, "flush operation should not return an error")

	// Assertion: deploymentA is added to the store with the correct detected languages
	// Reason: deploymentA has detected languages with dirty flag set to true
	assert.Eventuallyf(t,
		func() bool {
			deploymentA, err := mockStore.GetKubernetesDeployment("ns/deploymentA")
			if err != nil {
				return false
			}

			return reflect.DeepEqual(deploymentA.DetectedLanguages, langUtil.ContainersLanguages{
				*langUtil.NewContainer("some-container"): {
					"perl": {},
					"java": {},
					"ruby": {},
				},
			})

		},
		2*time.Second,
		100*time.Millisecond,
		"Should find deploymentA in workloadmeta store with the correct languages")

	// Assertion: deploymentB is added to the store with the correct detected languages
	// Reason: deploymentB has detected languages with dirty flag set to false
	_, err = mockStore.GetKubernetesDeployment("ns/deploymentB")
	assert.Errorf(t, err, "deploymentB should not be in store since the dirty flag is set to false")

	// Assertion: dirty flags of flushed languages are reset to false
	assert.False(t, ownersLanguages.containersLanguages[mockSupportedOwnerA].dirty, "deploymentA dirty flag should be reset to false")
	assert.False(t, ownersLanguages.containersLanguages[mockSupportedOwnerB].dirty, "deploymentB dirty flag should be reset to false")
	assert.False(t, ownersLanguages.containersLanguages[mockSupportedOwnerB].dirty, "daemonset dirty flag should not be reset to false")

	// set deploymentB dirty flag
	ownersLanguages.containersLanguages[mockSupportedOwnerB].dirty = true

	// add unsupported owner to ownerslanguages
	ownersLanguages.containersLanguages[mockUnsupportedOwner] = &containersLanguageWithDirtyFlag{
		languages: langUtil.TimedContainersLanguages{
			*langUtil.NewContainer("some-container"): {
				"perl": mockExpiration,
				"java": mockExpiration,
				"ruby": mockExpiration,
			},
			*langUtil.NewContainer("some-other-container"): {
				"cpp": mockExpiration,
			},
		},
		dirty: true,
	}

	// clean owners languages
	err = ownersLanguages.flush(mockStore)
	assert.Errorf(t, err, "clean operation should return an error due to unsupported resource kind")

	// Assert that deploymentB is not added to the store with the correct languages
	assert.Eventuallyf(t, func() bool {
		deploymentB, err := mockStore.GetKubernetesDeployment("ns/deploymentB")
		if err != nil {
			return false
		}

		languagesInStore := deploymentB.DetectedLanguages

		return reflect.DeepEqual(languagesInStore, langUtil.ContainersLanguages{
			*langUtil.NewContainer("some-container"):       {"java": {}},
			*langUtil.NewContainer("some-other-container"): {"cpp": {}},
		})
	}, 2*time.Second, 100*time.Millisecond, "Should find deploymentB in workloadmeta store with the correct languages")

	// Assertion: dirty flags of flushed languages are reset to false
	assert.False(t, ownersLanguages.containersLanguages[mockSupportedOwnerA].dirty, "deploymentA dirty flag should be reset to false")
	assert.False(t, ownersLanguages.containersLanguages[mockSupportedOwnerB].dirty, "deploymentB dirty flag should be reset to false")
	assert.False(t, ownersLanguages.containersLanguages[mockSupportedOwnerB].dirty, "daemonset dirty flag should not be reset to false")
}

func TestOwnersLanguagesMergeAndFlush(t *testing.T) {
	mockSupportedOwnerA := langUtil.NewNamespacedOwnerReference("api-version", langUtil.KindDeployment, "deploymentA", "ns")
	mockExpiration := time.Now()

	ownersLanguages := OwnersLanguages{
		containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
			mockSupportedOwnerA: {
				languages: langUtil.TimedContainersLanguages{
					*langUtil.NewContainer("python-container"): {
						"python": mockExpiration.Add(10 * time.Minute),
					},
				},
				dirty: true,
			},
		},
	}

	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))

	err := ownersLanguages.flush(mockStore)
	assert.NoErrorf(t, err, "flush operation should not return an error")

	// Assertion: deploymentA is added to the store with the correct detected languages
	// Reason: deploymentA has detected languages with dirty flag set to true
	assert.Eventuallyf(t,
		func() bool {
			deploymentA, err := mockStore.GetKubernetesDeployment("ns/deploymentA")
			if err != nil {
				return false
			}

			return reflect.DeepEqual(deploymentA.DetectedLanguages, langUtil.ContainersLanguages{
				*langUtil.NewContainer("python-container"): {
					"python": {},
				},
			})

		},
		2*time.Second,
		100*time.Millisecond,
		"Should find deploymentA in workloadmeta store with the correct languages")

	mockOwnersLanguagesFromRequest := OwnersLanguages{
		containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
			mockSupportedOwnerA: {
				languages: langUtil.TimedContainersLanguages{
					*langUtil.NewContainer("python-container"): {
						"python": mockExpiration.Add(30 * time.Minute),
					},
					*langUtil.NewContainer("ruby-container"): {
						"ruby": mockExpiration.Add(50 * time.Minute),
					},
				},
				dirty: true,
			},
		},
	}

	// Assertion: dirty flags of flushed languages are reset to false
	assert.False(t, ownersLanguages.containersLanguages[mockSupportedOwnerA].dirty, "deploymentA dirty flag should be reset to false")

	err = ownersLanguages.mergeAndFlush(&mockOwnersLanguagesFromRequest, mockStore)
	assert.NoErrorf(t, err, "mergeAndFlush operation should not return an error")

	// Assertion: deploymentA is found in store with the correct detected languages
	// Reason: deploymentA has detected languages with dirty flag set to true
	assert.Eventuallyf(t,
		func() bool {
			deploymentA, err := mockStore.GetKubernetesDeployment("ns/deploymentA")
			if err != nil {
				return false
			}

			fmt.Println(deploymentA.DetectedLanguages)

			return reflect.DeepEqual(deploymentA.DetectedLanguages, langUtil.ContainersLanguages{
				*langUtil.NewContainer("python-container"): {
					"python": {},
				},
				*langUtil.NewContainer("ruby-container"): {
					"ruby": {},
				},
			})

		},
		2*time.Second,
		100*time.Millisecond,
		"Should find deploymentA in workloadmeta store with the correct languages")

	// Assertion: dirty flags of flushed languages are reset to false
	assert.False(t, ownersLanguages.containersLanguages[mockSupportedOwnerA].dirty, "deploymentA dirty flag should be reset to false")
}

func TestCleanExpiredLanguages(t *testing.T) {
	mockSupportedOwnerA := langUtil.NewNamespacedOwnerReference("api-version", langUtil.KindDeployment, "deploymentA", "ns")
	mockSupportedOwnerB := langUtil.NewNamespacedOwnerReference("api-version", langUtil.KindDeployment, "deploymentB", "ns")

	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))

	mockStore.Push(workloadmeta.SourceLanguageDetectionServer, workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "ns/deploymentA",
			},
			DetectedLanguages: langUtil.ContainersLanguages{
				*langUtil.NewContainer("some-container"): {
					"python": {},
					"java":   {},
				},
			},
		},
	},
		workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesDeployment{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesDeployment,
					ID:   "ns/deploymentB",
				},
				DetectedLanguages: langUtil.ContainersLanguages{
					*langUtil.NewContainer("some-container"): {
						"python": {},
						"java":   {},
					},
				},
			},
		},
	)

	ownersLanguages := OwnersLanguages{
		containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
			mockSupportedOwnerA: {
				languages: langUtil.TimedContainersLanguages{
					*langUtil.NewContainer("some-container"): {
						"python": time.Now().Add(-5 * time.Minute),
						"java":   time.Now().Add(5 * time.Minute),
					},
				},
				dirty: false,
			},
			mockSupportedOwnerB: {
				languages: langUtil.TimedContainersLanguages{
					*langUtil.NewContainer("some-container"): {
						"python": time.Now().Add(-5 * time.Minute),
						"java":   time.Now().Add(-5 * time.Minute),
					},
				},
				dirty: false,
			},
		},
	}

	ownersLanguages.cleanExpiredLanguages(mockStore)

	assert.Eventuallyf(t,
		func() bool {
			deploymentA, err := mockStore.GetKubernetesDeployment("ns/deploymentA")
			if err != nil {
				return false
			}

			fmt.Println(deploymentA)

			return reflect.DeepEqual(deploymentA.DetectedLanguages, langUtil.ContainersLanguages{
				*langUtil.NewContainer("some-container"): {
					"java": {},
				},
			})

		},
		2*time.Second,
		100*time.Millisecond,
		"Should find deploymentA in workloadmeta store with the correct languages")

	assert.Eventuallyf(t,
		func() bool {
			_, err := mockStore.GetKubernetesDeployment("ns/deploymentB")
			return err != nil
		},
		2*time.Second,
		100*time.Millisecond,
		"Should remove deploymentB from workloadmeta store since all languages are expired")

}

func TestCleanRemovedOwners(t *testing.T) {
	mockSupportedOwnerA := langUtil.NewNamespacedOwnerReference("api-version", langUtil.KindDeployment, "deploymentA", "ns")

	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))

	ownersLanguages := OwnersLanguages{
		containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
			mockSupportedOwnerA: {
				languages: langUtil.TimedContainersLanguages{
					*langUtil.NewContainer("some-container"): {
						"python": time.Now().Add(5 * time.Minute),
						"java":   time.Now().Add(5 * time.Minute),
					},
				},
				dirty: false,
			},
		},
	}

	// start cleanup process
	go ownersLanguages.cleanRemovedOwners(mockStore)

	// Simulate detecting languages
	mockStore.Push(workloadmeta.SourceLanguageDetectionServer, workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "ns/deploymentA",
			},
			DetectedLanguages: langUtil.ContainersLanguages{
				*langUtil.NewContainer("some-container"): {
					"python": {},
					"java":   {},
				},
			},
		},
	},
	)

	// simulate updating annotations
	mockStore.Push("kubeapiserver", workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "ns/deploymentA",
			},
			InjectableLanguages: langUtil.ContainersLanguages{
				*langUtil.NewContainer("some-container"): {
					"python": {},
					"java":   {},
				},
			},
		},
	},
	)

	time.Sleep(1 * time.Second)

	//simulate deleting deployment
	mockStore.Push("kubeapiserver", workloadmeta.Event{
		Type: workloadmeta.EventTypeUnset,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesDeployment,
				ID:   "ns/deploymentA",
			},
		},
	},
	)

	assert.Eventuallyf(t,
		func() bool {
			_, err := mockStore.GetKubernetesDeployment("ns/deploymentA")
			return err != nil
		},
		2*time.Second,
		100*time.Millisecond,
		"Should remove deploymentA from workloadmeta")
}

////////////////////////////////
//                            //
//    Util Functions Tests    //
//                            //
////////////////////////////////

func TestGetContainersLanguagesFromPodDetail(t *testing.T) {
	mockExpiration := time.Now()

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

	initContainerDetails := []*pbgo.ContainerLanguageDetails{
		{
			ContainerName: "init-mono-lang",
			Languages: []*pbgo.Language{
				{Name: "java"},
			},
		},
	}

	podLanguageDetails := &pbgo.PodLanguageDetails{
		Namespace:            "default",
		ContainerDetails:     containerDetails,
		InitContainerDetails: initContainerDetails,
		Ownerref: &pbgo.KubeOwnerInfo{
			Kind: "ReplicaSet",
			Name: "dummyrs-2342347",
		},
	}

	containerslanguages := getContainersLanguagesFromPodDetail(podLanguageDetails, mockExpiration)

	expectedContainersLanguages := langUtil.TimedContainersLanguages{
		*langUtil.NewContainer("mono-lang"): {
			"java": mockExpiration,
		},
		*langUtil.NewContainer("bi-lang"): {
			"java": mockExpiration,
			"cpp":  mockExpiration,
		},
		*langUtil.NewContainer("tri-lang"): {
			"java":   mockExpiration,
			"go":     mockExpiration,
			"python": mockExpiration,
		},
		*langUtil.NewInitContainer("init-mono-lang"): {
			"java": mockExpiration,
		},
	}

	assert.True(t, reflect.DeepEqual(containerslanguages, &expectedContainersLanguages), fmt.Sprintf("Expected %v, found %v", &expectedContainersLanguages, containerslanguages))
}

func TestGetOwnersLanguages(t *testing.T) {
	mockExpiration := time.Now()

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
				ContainerName: "init-container-3",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "cpp"},
				},
			},
			{
				ContainerName: "init-container-4",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "python"},
				},
			},
		},
		Ownerref: &pbgo.KubeOwnerInfo{
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
				ContainerName: "init-container-7",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "cpp"},
				},
			},
			{
				ContainerName: "init-container-8",
				Languages: []*pbgo.Language{
					{Name: "java"},
					{Name: "python"},
				},
			},
		},
		Ownerref: &pbgo.KubeOwnerInfo{
			Kind: "ReplicaSet",
			Name: "dummyrs-2-2342347",
			Id:   "some-uid",
		},
	}

	mockRequestData := &pbgo.ParentLanguageAnnotationRequest{
		PodDetails: []*pbgo.PodLanguageDetails{
			podALanguageDetails,
			podBLanguageDetails,
		},
	}

	expectedContainersLanguagesA := containersLanguageWithDirtyFlag{
		dirty: true,
		languages: langUtil.TimedContainersLanguages{
			*langUtil.NewContainer("container-1"): {
				"java": mockExpiration,
				"cpp":  mockExpiration,
				"go":   mockExpiration,
			},
			*langUtil.NewContainer("container-2"): {
				"java":   mockExpiration,
				"python": mockExpiration,
			},
			*langUtil.NewInitContainer("init-container-3"): {
				"java": mockExpiration,
				"cpp":  mockExpiration,
			},
			*langUtil.NewInitContainer("init-container-4"): {
				"java":   mockExpiration,
				"python": mockExpiration,
			},
		},
	}

	expectedContainersLanguagesB := containersLanguageWithDirtyFlag{
		dirty: true,
		languages: langUtil.TimedContainersLanguages{
			*langUtil.NewContainer("container-5"): {
				"python": mockExpiration,
				"cpp":    mockExpiration,
				"go":     mockExpiration,
			},
			*langUtil.NewContainer("container-6"): {
				"java": mockExpiration,
				"ruby": mockExpiration,
			},
			*langUtil.NewInitContainer("init-container-7"): {
				"java": mockExpiration,
				"cpp":  mockExpiration,
			},
			*langUtil.NewInitContainer("init-container-8"): {
				"java":   mockExpiration,
				"python": mockExpiration,
			},
		},
	}

	expectedOwnersLanguages := &OwnersLanguages{
		containersLanguages: map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag{
			langUtil.NewNamespacedOwnerReference("apps/v1", "Deployment", "dummyrs-1", "default"): &expectedContainersLanguagesA,
			langUtil.NewNamespacedOwnerReference("apps/v1", "Deployment", "dummyrs-2", "custom"):  &expectedContainersLanguagesB,
		},
	}

	actualOwnersLanguages := getOwnersLanguages(mockRequestData, mockExpiration)

	assert.True(t, reflect.DeepEqual(expectedOwnersLanguages, actualOwnersLanguages), fmt.Sprintf("Expected %v, found %v", expectedOwnersLanguages, actualOwnersLanguages))
}

func TestGeneratePushEvent(t *testing.T) {
	mockSupportedOwner := langUtil.NewNamespacedOwnerReference("api-version", "Deployment", "some-name", "some-ns")
	mockUnsupportedOwner := langUtil.NewNamespacedOwnerReference("api-version", "UnsupportedResourceKind", "some-name", "some-ns")
	mockExpiration := time.Now()

	tests := []struct {
		name          string
		languages     langUtil.TimedContainersLanguages
		owner         langUtil.NamespacedOwnerReference
		expectedEvent *workloadmeta.Event
	}{
		{
			name:          "unsupported owner",
			languages:     make(langUtil.TimedContainersLanguages),
			owner:         mockUnsupportedOwner,
			expectedEvent: nil,
		},
		{
			name:      "empty containers languages object with supported owner",
			languages: make(langUtil.TimedContainersLanguages),
			owner:     mockSupportedOwner,
			expectedEvent: &workloadmeta.Event{
				Type: workloadmeta.EventTypeUnset,
				Entity: &workloadmeta.KubernetesDeployment{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesDeployment,
						ID:   "some-ns/some-name",
					},
					DetectedLanguages: make(langUtil.ContainersLanguages),
				},
			},
		},
		{
			name: "non-empty containers languages with supported owner",
			languages: langUtil.TimedContainersLanguages{
				langUtil.Container{Name: "container-1", Init: false}: {
					"java": mockExpiration,
					"cpp":  mockExpiration,
				},
				langUtil.Container{Name: "container-2", Init: false}: {
					"java": mockExpiration,
					"cpp":  mockExpiration,
				},
				langUtil.Container{Name: "container-3", Init: true}: {
					"python": mockExpiration,
					"ruby":   mockExpiration,
				},
				langUtil.Container{Name: "container-4", Init: true}: {
					"go":   mockExpiration,
					"java": mockExpiration,
				},
			},
			owner: mockSupportedOwner,
			expectedEvent: &workloadmeta.Event{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesDeployment{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesDeployment,
						ID:   "some-ns/some-name",
					},
					DetectedLanguages: langUtil.ContainersLanguages{
						langUtil.Container{Name: "container-1", Init: false}: {
							"java": {},
							"cpp":  {},
						},
						langUtil.Container{Name: "container-2", Init: false}: {
							"java": {},
							"cpp":  {},
						},
						langUtil.Container{Name: "container-3", Init: true}: {
							"python": {},
							"ruby":   {},
						},
						langUtil.Container{Name: "container-4", Init: true}: {
							"go":   {},
							"java": {},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			actualEvent := generatePushEvent(test.owner, test.languages)

			if actualEvent == nil && test.expectedEvent == nil {
				return

			}

			// Event has correct type
			assert.Equal(tt, test.expectedEvent.Type, actualEvent.Type)

			// Event entity has correct Entity Id
			assert.True(
				tt,
				reflect.DeepEqual(test.expectedEvent.Entity.GetID(), actualEvent.Entity.GetID()),
				fmt.Sprintf(
					"entity id is not correct: expected %v, but found %v",
					test.expectedEvent.Entity.GetID(),
					actualEvent.Entity.GetID(),
				),
			)

			// Event has correct detected languages
			actualDeploymentEntity := actualEvent.Entity.(*workloadmeta.KubernetesDeployment)
			expectedDeploymentEntity := test.expectedEvent.Entity.(*workloadmeta.KubernetesDeployment)

			actualDetectedLanguages := actualDeploymentEntity.DetectedLanguages
			expectedDetectedLanguages := expectedDeploymentEntity.DetectedLanguages

			assert.True(
				tt,
				reflect.DeepEqual(actualDetectedLanguages, expectedDetectedLanguages),
				fmt.Sprintf("container languages are not correct: expected %v, found %v", expectedDetectedLanguages, actualDetectedLanguages),
			)
		})
	}
}
