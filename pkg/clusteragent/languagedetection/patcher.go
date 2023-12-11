// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
)

// OwnersLanguages maps an owner to the detected languages of each container
type OwnersLanguages map[NamespacedOwnerReference]langUtil.ContainersLanguages

// LanguagePatcher defines an object that patches kubernetes resources with language annotations
type LanguagePatcher struct {
	k8sClient dynamic.Interface
	store     workloadmeta.Component
}

// NewLanguagePatcher initializes and returns a new patcher with a dynamic k8s client
func NewLanguagePatcher(store workloadmeta.Component) (*LanguagePatcher, error) {
	if store == nil {
		return nil, fmt.Errorf("cannot initialize patcher with a nil workloadmeta store")
	}

	apiCl, err := apiserver.GetAPIClient()

	if err != nil {
		return nil, err
	}

	k8sClient := apiCl.DynamicCl
	return &LanguagePatcher{
		k8sClient: k8sClient,
		store:     store,
	}, nil
}

func (lp *LanguagePatcher) getContainersLanguagesFromPodDetail(podDetail *pbgo.PodLanguageDetails) langUtil.ContainersLanguages {
	containerslanguages := langUtil.NewContainersLanguages()

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

func (lp *LanguagePatcher) detectedNewLanguages(namespacedOwnerRef *NamespacedOwnerReference, detectedLanguages langUtil.ContainersLanguages) bool {
	// Currently we only support deployment owners
	id := fmt.Sprintf("%s/%s", namespacedOwnerRef.namespace, namespacedOwnerRef.Name)
	owner, err := lp.store.GetKubernetesDeployment(id)

	if err != nil {
		return true
	}

	existingContainersLanguages := langUtil.NewContainersLanguages()

	for container, languages := range owner.ContainerLanguages {
		for _, language := range languages {
			existingContainersLanguages.GetOrInitializeLanguageset(container).Add(string(language.Name))
		}
	}

	for container, languages := range owner.InitContainerLanguages {
		for _, language := range languages {
			existingContainersLanguages.GetOrInitializeLanguageset(fmt.Sprintf("init.%s", container)).Add(string(language.Name))
		}
	}

	for container, languages := range detectedLanguages {
		for language := range languages {
			if _, found := existingContainersLanguages.GetOrInitializeLanguageset(container)[language]; !found {
				return true
			}
		}
	}

	return false
}

// patches the owner with the corresponding language annotations
func (lp *LanguagePatcher) patchOwner(namespacedOwnerRef *NamespacedOwnerReference, containerslanguages langUtil.ContainersLanguages) error {
	ownerGVR, err := getGVR(namespacedOwnerRef)
	if err != nil {
		return err
	}

	if !lp.detectedNewLanguages(namespacedOwnerRef, containerslanguages) {
		// No need to patch
		SkippedPatches.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.namespace)
		return nil
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {

		langAnnotations := containerslanguages.ToAnnotations()

		// Serialize the patch data
		patchData, err := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": langAnnotations,
			},
		})
		if err != nil {
			return err
		}

		_, err = lp.k8sClient.Resource(ownerGVR).Namespace(namespacedOwnerRef.namespace).Patch(context.TODO(), namespacedOwnerRef.Name, types.MergePatchType, patchData, metav1.PatchOptions{})
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
