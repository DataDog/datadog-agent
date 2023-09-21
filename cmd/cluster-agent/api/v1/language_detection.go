// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gorilla/mux"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/util/retry"
)

const (
	maximumWaitForAPIServer = 10 * time.Second
)

// Install language detection endpoints
func InstallLanguageDetectionEndpoints(r *mux.Router) {
	r.HandleFunc("/languagedetection", api.WithTelemetryWrapper("setDetectedLanguages", setDetectedLanguages)).Methods("POST")
}

// ownerInfo maintains the information about a Kubernetes resource owner.
type ownerInfo struct {
	name      string
	namespace string
	kind      string
}

// NewOwnerInfo initializes a new ownerInfo structure with the provided arguments.
// If the "kind" is given as "replicaset", it returns the info of the parent deployment
// and the name will be parsed for the Deployment of the ReplicaSet.
func NewOwnerInfo(name string, namespace string, kind string) *ownerInfo {

	if kind == "replicaset" {
		kind = "deployment"
		name = kubernetes.ParseDeploymentForReplicaSet(name)
	}

	return &ownerInfo{
		name:      name,
		namespace: namespace,
		kind:      kind,
	}
}

// Returns the GroupVersionResource of the owner object
// Currently supports only "Deployment" type
// Returns an error for unsupported types
func (o *ownerInfo) getGVR() (*schema.GroupVersionResource, error) {
	if o.kind == "deployment" {
		return &schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: fmt.Sprintf("%ss", o.kind),
		}, nil
	}
	return &schema.GroupVersionResource{}, errors.New(fmt.Sprintf("unsupported owner kind %s", o.kind))
}

type OwnersLanguages map[ownerInfo]*languagedetection.ContainersLanguages

type LanguagePatcherInterface interface {
	getContainersLanguagesFromPodDetail(podDetail *pbgo.PodLanguageDetails) *languagedetection.ContainersLanguages
	getOwnersLanguages(requestData *pbgo.ParentLanguageAnnotationRequest) *OwnersLanguages
	getUpdatedOwnerAnnotations(currentAnnotations map[string]string, containerslanguages *languagedetection.ContainersLanguages) (map[string]string, int)
	patchOwner(deploymentName string, namespace string, containerslanguages *languagedetection.ContainersLanguages) error
	patchAllOwners(ownerlanguages *OwnersLanguages)
}

type languagePatcher struct {
	k8sClient dynamic.Interface
}

// Initializes a new patcher with a dynamic k8s client
func NewLanguagePatcher() (*languagePatcher, error) {
	apiCtx, apiCancel := context.WithTimeout(context.Background(), maximumWaitForAPIServer)
	defer apiCancel()
	apiCl, err := apiserver.WaitForAPIClient(apiCtx)

	if err != nil {
		return nil, err
	}

	k8sClient := apiCl.DynamicCl
	return &languagePatcher{
		k8sClient: k8sClient,
	}, nil
}

func (lp *languagePatcher) getContainersLanguagesFromPodDetail(podDetail *pbgo.PodLanguageDetails) *languagedetection.ContainersLanguages {
	containerslanguages := languagedetection.NewContainersLanguages()

	for _, containerLanguageDetails := range podDetail.ContainerDetails {
		container := containerLanguageDetails.ContainerName
		languages := containerLanguageDetails.Languages
		for _, language := range languages {
			containerslanguages.Add(container, language.Name)
		}
	}
	return containerslanguages
}

// Gets the containers languages for every owner
func (lp *languagePatcher) getOwnersLanguages(requestData *pbgo.ParentLanguageAnnotationRequest) *OwnersLanguages {
	ownerslanguages := make(OwnersLanguages)
	podDetails := requestData.PodDetails

	// Generate annotations for each owner
	for _, podDetail := range podDetails {
		ns := podDetail.Namespace
		ownerRef := podDetail.Ownerref
		ownerinfo := *NewOwnerInfo(ownerRef.Name, ns, ownerRef.Kind)
		ownerslanguages[ownerinfo] = lp.getContainersLanguagesFromPodDetail(podDetail)
	}

	return &ownerslanguages
}

func (lp *languagePatcher) getUpdatedOwnerAnnotations(currentAnnotations map[string]string, containerslanguages *languagedetection.ContainersLanguages) (map[string]string, int) {

	if currentAnnotations == nil {
		currentAnnotations = make(map[string]string)
	}

	// Add the existing language annotations into containers languages object
	existingContainersLanguages := languagedetection.NewContainersLanguages()
	existingContainersLanguages.ParseAnnotations(currentAnnotations)

	// Append the potentially new languages to the containers languages object
	languagesBeforeUpdate := existingContainersLanguages.TotalLanguages()
	for container, languageset := range containerslanguages.Languages {
		existingContainersLanguages.Parse(container, fmt.Sprint(languageset))
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
func (lp *languagePatcher) patchOwner(ownerinfo *ownerInfo, containerslanguages *languagedetection.ContainersLanguages) error {

	ownerGVR, err := ownerinfo.getGVR()

	if err != nil {
		return err
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		owner, err := lp.k8sClient.Resource(*ownerGVR).Namespace(ownerinfo.namespace).Get(context.TODO(), ownerinfo.name, metav1.GetOptions{})
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

		_, err = lp.k8sClient.Resource(*ownerGVR).Namespace(ownerinfo.namespace).Update(context.TODO(), owner, metav1.UpdateOptions{})

		if err != nil {
			PatchRetries.Inc(ownerinfo.kind, ownerinfo.name, ownerinfo.namespace)
		}

		return err
	})

	if retryErr != nil {
		FailedPatches.Inc(ownerinfo.kind, ownerinfo.name, ownerinfo.namespace)
		return fmt.Errorf("Failed to update owner: %v", retryErr)
	} else {
		SuccessPatches.Inc(ownerinfo.kind, ownerinfo.name, ownerinfo.namespace)
	}

	return nil
}

// patches all owners with the corresponding language annotations
func (lp *languagePatcher) patchAllOwners(ownerslanguages *OwnersLanguages) {

	// Patch annotations to deployments
	for ownerinfo, ownerlanguages := range *ownerslanguages {
		err := lp.patchOwner(&ownerinfo, ownerlanguages)

		if err != nil {
			log.Errorf("Error patching language annotations to deployment %s in %s namespace: %v", ownerinfo.name, ownerinfo.namespace, err)
		}
	}
}

func setDetectedLanguages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		ErrorResponses.Inc()
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
	}

	// Create a new instance of the protobuf message type
	requestData := &pbgo.ParentLanguageAnnotationRequest{}

	// Unmarshal the request body into the protobuf message
	err = proto.Unmarshal(body, requestData)
	if err != nil {
		ErrorResponses.Inc()
		http.Error(w, "Failed to unmarshal request body", http.StatusBadRequest)
	}

	lp, err := NewLanguagePatcher()

	if err != nil {
		ErrorResponses.Inc()
		http.Error(w, "Failed to get k8s apiserver client", http.StatusInternalServerError)
	}

	// Generate annotations for each deployment
	ownersAnnotations := lp.getOwnersLanguages(requestData)

	// Patch annotations to deployments
	lp.patchAllOwners(ownersAnnotations)

	// Respond to the request
	OkResponses.Inc()
	w.WriteHeader(http.StatusOK)
}
