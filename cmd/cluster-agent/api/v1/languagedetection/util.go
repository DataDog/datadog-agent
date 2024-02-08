// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"sync"
)

// containersLanguageWithDirtyFlag encapsulates containers languages along with a dirty flag
// The dirty flag is used to know if the containers languages are flushed to workload metadata store or not.
// The dirty flag is reset when languages are flushed to workload metadata store.
type containersLanguageWithDirtyFlag struct {
	languages langUtil.ContainersLanguages
	dirty     bool
}

func newContainersLanguageWithDirtyFlag() *containersLanguageWithDirtyFlag {
	return &containersLanguageWithDirtyFlag{
		languages: make(langUtil.ContainersLanguages),
		dirty:     true,
	}
}

////////////////////////////////
//                            //
//      Owners Languages      //
//                            //
////////////////////////////////

// OwnersLanguages maps a namespaced owner (kubernetes resource) to containers languages
// This is mainly used as a preliminary storage for detected languages of kubernetes resources prior to storing
// languages in workload meta store.
//
// It is needed in order to:
//   - control what to store in workload metadata store based on detected languages TTL and last detection time
//   - avoid flakiness in the set of detected languages during the rollout of a kubernetes resource;
//     during rollout the handler may, depending on the deployment size for example, receive different languages
//     based on whether the source pod has been rolled out yet or not, which can cause flakiness in the set of detected languages.
//
// Components using OwnersLanguages should only invoke the mergeAndFlush method, which is thread-safe.
// Other methods are not thread-safe; they are supposed to be invoked only within mergeAndFlush.
type OwnersLanguages struct {
	containersLanguages map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag
	mutex               sync.Mutex
}

func newOwnersLanguages() *OwnersLanguages {
	return &OwnersLanguages{
		containersLanguages: make(map[langUtil.NamespacedOwnerReference]*containersLanguageWithDirtyFlag),
		mutex:               sync.Mutex{},
	}
}

// getOrInitialize returns the containers languages for a specific namespaced owner, initialising it if it doesn't already
// exist.
// This method is not thread-safe.
func (ownersLanguages *OwnersLanguages) getOrInitialize(reference langUtil.NamespacedOwnerReference) *containersLanguageWithDirtyFlag {
	_, found := ownersLanguages.containersLanguages[reference]
	if !found {
		ownersLanguages.containersLanguages[reference] = newContainersLanguageWithDirtyFlag()
	}
	containersLanguages := ownersLanguages.containersLanguages[reference]
	return containersLanguages
}

// merge merges another owners languages instance data with the current containers languages.
// This method is not thread-safe.
func (ownersLanguages *OwnersLanguages) merge(other *OwnersLanguages) {
	for owner, containersLanguages := range other.containersLanguages {
		langsWithDirtyFlag := ownersLanguages.getOrInitialize(owner)
		if modified := langsWithDirtyFlag.languages.Merge(containersLanguages.languages); modified {
			langsWithDirtyFlag.dirty = true
		}
	}
}

// flush flushes to workloadmeta store containers languages that have dirty flag set to true, and then resets
// dirty flag to false.
// This method is not thread-safe.
func (ownersLanguages *OwnersLanguages) flush(wlm workloadmeta.Component) error {
	pushErrors := make([]error, 0)

	for owner, containersLanguages := range ownersLanguages.containersLanguages {

		// Skip if not dirty
		if !containersLanguages.dirty {
			continue
		}

		// Generate push event
		if event := generatePushEvent(owner, containersLanguages.languages); event != nil {
			pushError := wlm.Push(workloadmeta.SourceLanguageDetectionServer, *event)
			if pushError != nil {
				pushErrors = append(pushErrors, pushError)
			} else {
				containersLanguages.dirty = false
			}
		} else {
			pushErrors = append(
				pushErrors,
				fmt.Errorf(
					"failed to generate push event for %v %v/%v. reason: unsupported resource kind",
					owner.Kind,
					owner.Namespace,
					owner.Name),
			)
		}
	}
	return errors.Join(pushErrors...)
}

// mergeAndFlush merges the current containers languages for all owners with owners containers languages
// passed as an argument. It then flushes the containers languages having a set dirty flag to workloadmeta store
// and resets dirty flags to false.
// This method is thread-safe, and it serves as the unique entrypoint to instances of this type.
func (ownersLanguages *OwnersLanguages) mergeAndFlush(other *OwnersLanguages, wlm workloadmeta.Component) error {
	ownersLanguages.mutex.Lock()
	defer ownersLanguages.mutex.Unlock()

	ownersLanguages.merge(other)

	return ownersLanguages.flush(wlm)
}

////////////////////////////////
//                            //
//           Utils            //
//                            //
////////////////////////////////

func generatePushEvent(owner langUtil.NamespacedOwnerReference, languages langUtil.ContainersLanguages) *workloadmeta.Event {
	_, found := langUtil.SupportedBaseOwners[owner.Kind]

	if !found {
		return nil
	}

	switch owner.Kind {
	case langUtil.KindDeployment:
		return &workloadmeta.Event{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesDeployment{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesDeployment,
					ID:   fmt.Sprintf("%s/%s", owner.Namespace, owner.Name),
				},
				DetectedLanguages: languages.DeepCopy(),
			},
		}
	default:
		return nil
	}
}

// getContainersLanguagesFromPodDetail returns containers languages objects for both standard containers
// and for init container
func getContainersLanguagesFromPodDetail(podDetail *pbgo.PodLanguageDetails) *langUtil.ContainersLanguages {
	containersLanguages := make(langUtil.ContainersLanguages)

	// handle standard containers
	for _, containerLanguageDetails := range podDetail.ContainerDetails {
		containerName := containerLanguageDetails.ContainerName
		languages := containerLanguageDetails.Languages
		for _, language := range languages {
			containersLanguages.GetOrInitialize(*langUtil.NewContainer(containerName)).Add(langUtil.Language(language.Name))
		}
	}

	// handle init containers
	for _, containerLanguageDetails := range podDetail.InitContainerDetails {
		containerName := containerLanguageDetails.ContainerName
		languages := containerLanguageDetails.Languages
		for _, language := range languages {
			containersLanguages.GetOrInitialize(*langUtil.NewInitContainer(containerName)).Add(langUtil.Language(language.Name))
		}
	}

	return &containersLanguages
}

// getOwnersLanguages constructs OwnersLanguages from owners (i.e. k8s parent resource)
func getOwnersLanguages(requestData *pbgo.ParentLanguageAnnotationRequest) *OwnersLanguages {
	ownersContainersLanguages := newOwnersLanguages()

	podDetails := requestData.PodDetails

	for _, podDetail := range podDetails {
		namespacedOwnerRef := langUtil.GetNamespacedBaseOwnerReference(podDetail)

		if _, found := langUtil.SupportedBaseOwners[namespacedOwnerRef.Kind]; found {
			containersLanguages := *getContainersLanguagesFromPodDetail(podDetail)
			langsWithDirtyFlag := ownersContainersLanguages.getOrInitialize(namespacedOwnerRef)
			if modified := langsWithDirtyFlag.languages.Merge(containersLanguages); modified {
				langsWithDirtyFlag.dirty = true
			}
		}
	}

	return ownersContainersLanguages
}
