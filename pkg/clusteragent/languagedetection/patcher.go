// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

// Package languagedetection contains the language detection patcher running in the cluster agent
package languagedetection

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
	"strings"
	"sync"
)

const (
	// subscriber is the workloadmeta subscriber name
	subscriber = "language_detection_patcher"
)

// LanguagePatcher defines an object that patches kubernetes resources with language annotations
type languagePatcher struct {
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient dynamic.Interface
	store     workloadmeta.Component
	logger    log.Component
}

// NewLanguagePatcher initializes and returns a new patcher with a dynamic k8s client
func newLanguagePatcher(ctx context.Context, store workloadmeta.Component, logger log.Component, apiCl *apiserver.APIClient) *languagePatcher {

	ctx, cancel := context.WithCancel(ctx)

	k8sClient := apiCl.DynamicCl
	return &languagePatcher{
		ctx:       ctx,
		cancel:    cancel,
		k8sClient: k8sClient,
		store:     store,
		logger:    logger,
	}
}

var (
	patcher             *languagePatcher
	languagePatcherOnce sync.Once
)

// Start initializes and starts the language detection patcher
func Start(ctx context.Context, store workloadmeta.Component, logger log.Component) error {

	if patcher != nil {
		return fmt.Errorf("can't start language detection patcher twice")
	}

	if store == nil {
		return fmt.Errorf("cannot initialize patcher with a nil workloadmeta store")
	}

	apiCl, err := apiserver.GetAPIClient()

	if err != nil {
		return err
	}

	languagePatcherOnce.Do(func() {
		logger.Info("Starting language detection patcher")
		patcher = newLanguagePatcher(ctx, store, logger, apiCl)
		go patcher.run()
	})

	return nil
}

// Stop stops the language detection patcher
func Stop() {
	if patcher != nil {
		patcher.cancel()
		patcher = nil
	}
}

func (lp *languagePatcher) run() {
	defer lp.logger.Info("Shutting down language detection patcher")

	// Capture all set events
	filterParams := workloadmeta.FilterParams{
		Kinds: []workloadmeta.Kind{
			// Currently only deployments are supported
			workloadmeta.KindKubernetesDeployment,
		},
		Source:    workloadmeta.SourceLanguageDetectionServer,
		EventType: workloadmeta.EventTypeAll,
	}

	eventCh := lp.store.Subscribe(
		subscriber,
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(&filterParams),
	)
	defer lp.store.Unsubscribe(eventCh)

	health := health.RegisterLiveness("process-language-detection-patcher")

	for {
		select {
		case <-lp.ctx.Done():
			err := health.Deregister()
			if err != nil {
				lp.logger.Warnf("error de-registering health check: %s", err)
			}
			return
		case <-health.C:
		case eventBundle, ok := <-eventCh:
			if !ok {
				return
			}
			lp.handleEvent(eventBundle)
		}
	}
}

func (lp *languagePatcher) generateAnnotationsPatch(currentLangs, newLangs langUtil.ContainersLanguages) map[string]interface{} {
	currentAnnotations := currentLangs.ToAnnotations()
	targetAnnotations := newLangs.ToAnnotations()

	annotationsPatch := make(map[string]interface{})

	// All newly detected languages should be included
	for key, val := range targetAnnotations {
		annotationsPatch[key] = val
	}

	// Languages that are no more detected should be removed
	for key := range currentAnnotations {
		_, found := annotationsPatch[key]

		if !found {
			annotationsPatch[key] = nil
		}
	}

	return annotationsPatch
}

// handleEvent handles events from workloadmeta
func (lp *languagePatcher) handleEvent(eventBundle workloadmeta.EventBundle) {
	eventBundle.Acknowledge()
	lp.logger.Tracef("Processing %d events", len(eventBundle.Events))

	for _, event := range eventBundle.Events {
		switch event.Entity.GetID().Kind {
		case workloadmeta.KindKubernetesDeployment:
			lp.handleDeploymentEvent(event)
		}
	}
}

func (lp *languagePatcher) handleDeploymentEvent(event workloadmeta.Event) {
	fmt.Println("Handling deployment event")

	deploymentID := event.Entity.(*workloadmeta.KubernetesDeployment).ID

	// extract deployment name and namespace from entity id
	deploymentIds := strings.Split(deploymentID, "/")
	namespace := deploymentIds[0]
	deploymentName := deploymentIds[1]

	// get the complete entity
	deployment, err := lp.store.GetKubernetesDeployment(deploymentID)

	if err != nil {
		lp.logger.Info("Didn't find deployment in store, skipping")
		// skip if not in store
		return
	}

	// construct namespaced owner reference
	owner := langUtil.NewNamespacedOwnerReference(
		"apps/v1",
		langUtil.KindDeployment,
		deploymentName,
		namespace,
	)

	if event.Type == workloadmeta.EventTypeUnset {
		// In case of unset event, we should clear language detection annotations if they are still present
		// If they aren't present, then the resource has been deleted, so we should skip

		if len(deployment.InjectableLanguages) > 0 {
			// If some annotations still exist, remove them
			annotationsPatch := lp.generateAnnotationsPatch(deployment.InjectableLanguages, langUtil.ContainersLanguages{})
			lp.patchOwner(&owner, annotationsPatch)
			return
		}

		SkippedPatches.Inc(owner.Kind, owner.Name, owner.Namespace)
	} else if event.Type == workloadmeta.EventTypeSet {
		detectedLanguages := deployment.DetectedLanguages
		injectableLanguages := deployment.InjectableLanguages

		// Calculate annotations patch
		annotationsPatch := lp.generateAnnotationsPatch(injectableLanguages, detectedLanguages)
		if len(annotationsPatch) > 0 {
			lp.patchOwner(&owner, annotationsPatch)
		} else {
			SkippedPatches.Inc(owner.Kind, owner.Name, owner.Namespace)
		}

	}
}

// patches the owner with the corresponding language annotations
func (lp *languagePatcher) patchOwner(namespacedOwnerRef *langUtil.NamespacedOwnerReference, annotationsPatch map[string]interface{}) {
	ownerGVR, err := langUtil.GetGVR(namespacedOwnerRef)
	if err != nil {
		lp.logger.Errorf("failed to update owner: %v", err)
		FailedPatches.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.Namespace)
		return
	}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Serialize the patch data
		patchData, err := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": annotationsPatch,
			},
		})
		if err != nil {
			return err
		}

		_, err = lp.k8sClient.Resource(ownerGVR).Namespace(namespacedOwnerRef.Namespace).Patch(context.TODO(), namespacedOwnerRef.Name, types.MergePatchType, patchData, metav1.PatchOptions{})
		if err != nil {
			PatchRetries.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.Namespace)
		}

		return err
	})

	if retryErr != nil {
		FailedPatches.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.Namespace)
		lp.logger.Errorf("failed to update owner: %v", retryErr)
		return
	}

	SuccessPatches.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.Namespace)
}
