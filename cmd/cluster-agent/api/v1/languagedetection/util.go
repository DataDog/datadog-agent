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

func (ownersLanguages *OwnersLanguages) getOrInitialize(reference langUtil.NamespacedOwnerReference) *containersLanguageWithDirtyFlag {
	_, found := ownersLanguages.containersLanguages[reference]
	if !found {
		ownersLanguages.containersLanguages[reference] = newContainersLanguageWithDirtyFlag()
	}
	containersLanguages := ownersLanguages.containersLanguages[reference]
	return containersLanguages
}

func (ownersLanguages *OwnersLanguages) merge(other *OwnersLanguages) {
	ownersLanguages.mutex.Lock()
	defer ownersLanguages.mutex.Unlock()

	for owner, containersLanguages := range other.containersLanguages {
		if len(containersLanguages.languages) > 0 {
			ownersLanguages.getOrInitialize(owner).languages.Merge(containersLanguages.languages)
		}
	}
}

func (ownersLanguages *OwnersLanguages) clean(wlm workloadmeta.Component) error {
	ownersLanguages.mutex.Lock()
	defer ownersLanguages.mutex.Unlock()
	pushErrors := make([]error, 0, len(ownersLanguages.containersLanguages))

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
				DetectedLanguages: languages,
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

		_, found := langUtil.SupportedBaseOwners[namespacedOwnerRef.Kind]
		if found {

			containersLanguages := *getContainersLanguagesFromPodDetail(podDetail)

			if len(containersLanguages) > 0 {
				ownersContainersLanguages.getOrInitialize(namespacedOwnerRef).languages.Merge(containersLanguages)
			}
		}
	}

	return ownersContainersLanguages
}
