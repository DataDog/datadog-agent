// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
)

// OwnersLanguages maps an owner to the detected languages of each container
type OwnersLanguages map[NamespacedOwnerReference]util.ContainersLanguages

// LanguagePatcher defines an object that patches kubernetes resources with language annotations
type LanguagePatcher struct {
	k8sClient dynamic.Interface
}

// NewLanguagePatcher initializes and returns a new patcher with a dynamic k8s client
func NewLanguagePatcher() (*LanguagePatcher, error) {
	apiCl, err := apiserver.GetAPIClient()

	if err != nil {
		return nil, err
	}

	k8sClient := apiCl.DynamicCl
	return &LanguagePatcher{
		k8sClient: k8sClient,
	}, nil
}

func (lp *LanguagePatcher) getContainersLanguagesFromPodDetail(podDetail *pbgo.PodLanguageDetails) util.ContainersLanguages {
	containerslanguages := util.NewContainersLanguages()

	for _, containerLanguageDetails := range podDetail.ContainerDetails {
		container := containerLanguageDetails.ContainerName
		languages := containerLanguageDetails.Languages
		for _, language := range languages {
			containerslanguages.GetOrInitializeLanguageset(container).Add(language.Name)
		}
	}

	// Handle Init Containers separately
	for _, containerLanguageDetails := range podDetail.InitContainerDetails {
		container := fmt.Sprintf("init.%s", containerLanguageDetails.ContainerName)
		languages := containerLanguageDetails.Languages
		for _, language := range languages {
			containerslanguages.GetOrInitializeLanguageset(container).Add(language.Name)
		}
	}

	return containerslanguages
}

// Gets the containers languages for every owner
func (lp *LanguagePatcher) getOwnersLanguages(requestData *pbgo.ParentLanguageAnnotationRequest) *OwnersLanguages {
	ownerslanguages := make(OwnersLanguages)
	podDetails := requestData.PodDetails

	// Generate annotations for each supported owner
	for _, podDetail := range podDetails {
		namespacedOwnerRef := getNamespacedBaseOwnerReference(podDetail)

		_, found := supportedBaseOwners[namespacedOwnerRef.Kind]
		if found {
			ownerslanguages[namespacedOwnerRef] = lp.getContainersLanguagesFromPodDetail(podDetail)
		}
	}

	return &ownerslanguages
}

// Updates the existing annotations based on the detected languages.
// Currently we only add languages to the annotations.
func (lp *LanguagePatcher) getUpdatedOwnerAnnotations(currentAnnotations map[string]string, containerslanguages util.ContainersLanguages) (map[string]string, int) {
	if currentAnnotations == nil {
		currentAnnotations = make(map[string]string)
	}

	// Add the existing language annotations into containers languages object
	existingContainersLanguages := util.NewContainersLanguages()
	existingContainersLanguages.ParseAnnotations(currentAnnotations)

	// Append the potentially new languages to the containers languages object
	languagesBeforeUpdate := existingContainersLanguages.TotalLanguages()
	for container, languageset := range containerslanguages {
		existingContainersLanguages.GetOrInitializeLanguageset(container).Merge(languageset)
	}
	languagesAfterUpdate := existingContainersLanguages.TotalLanguages()

	// Convert containers languages into annotations map
	updatedLanguageAnnotations := existingContainersLanguages.ToAnnotations()

	for annotationKey, annotationValue := range updatedLanguageAnnotations {
		currentAnnotations[annotationKey] = annotationValue
	}

	addedLanguages := languagesAfterUpdate - languagesBeforeUpdate
	return currentAnnotations, addedLanguages
}

// patches the owner with the corresponding language annotations
func (lp *LanguagePatcher) patchOwner(namespacedOwnerRef *NamespacedOwnerReference, containerslanguages util.ContainersLanguages) error {
	ownerGVR, err := getGVR(namespacedOwnerRef)
	if err != nil {
		return err
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		owner, err := lp.k8sClient.Resource(ownerGVR).Namespace(namespacedOwnerRef.namespace).Get(context.TODO(), namespacedOwnerRef.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		currentAnnotations := owner.GetAnnotations()
		updatedAnnotations, addedLanguages := lp.getUpdatedOwnerAnnotations(currentAnnotations, containerslanguages)
		if addedLanguages == 0 {
			// No need to patch owner because no new languages were added
			SkippedPatches.Inc()
			return nil
		}
		owner.SetAnnotations(updatedAnnotations)

		_, err = lp.k8sClient.Resource(ownerGVR).Namespace(namespacedOwnerRef.namespace).Update(context.TODO(), owner, metav1.UpdateOptions{})
		if err != nil {
			PatchRetries.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.namespace)
		}

		return err
	})

	if retryErr != nil {
		FailedPatches.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.namespace)
		return fmt.Errorf("Failed to update owner: %v", retryErr)
	}

	SuccessPatches.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.namespace)

	return nil
}

// PatchAllOwners patches all owners with the corresponding language annotations
func (lp *LanguagePatcher) PatchAllOwners(requestData *pbgo.ParentLanguageAnnotationRequest) {
	ownerslanguages := lp.getOwnersLanguages(requestData)

	// Patch annotations to deployments
	for namespacedOwnerRef, ownerlanguages := range *ownerslanguages {
		err := lp.patchOwner(&namespacedOwnerRef, ownerlanguages)
		if err != nil {
			log.Errorf("Error patching language annotations to deployment %s in %s namespace: %v", namespacedOwnerRef.Name, namespacedOwnerRef.namespace, err)
		}
	}
}
