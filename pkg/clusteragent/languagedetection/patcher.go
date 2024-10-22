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
	"errors"
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

const (
	// subscriber is the workloadmeta subscriber name
	subscriber    = "language_detection_patcher"
	statusSuccess = "success"
	statusRetry   = "retry"
	statusError   = "error"
	statusSkip    = "skip"
)

// LanguagePatcher defines an object that patches kubernetes resources with language annotations
type languagePatcher struct {
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient dynamic.Interface
	store     workloadmeta.Component
	logger    log.Component
	queue     workqueue.RateLimitingInterface
}

// NewLanguagePatcher initializes and returns a new patcher with a dynamic k8s client
func newLanguagePatcher(ctx context.Context, store workloadmeta.Component, logger log.Component, datadogConfig config.Component, apiCl *apiserver.APIClient) *languagePatcher {

	ctx, cancel := context.WithCancel(ctx)

	k8sClient := apiCl.DynamicCl

	return &languagePatcher{
		ctx:       ctx,
		cancel:    cancel,
		k8sClient: k8sClient,
		store:     store,
		logger:    logger,
		queue: workqueue.NewRateLimitingQueueWithConfig(
			workqueue.NewItemExponentialFailureRateLimiter(
				datadogConfig.GetDuration("cluster_agent.language_detection.patcher.base_backoff"),
				datadogConfig.GetDuration("cluster_agent.language_detection.patcher.max_backoff"),
			),
			workqueue.RateLimitingQueueConfig{
				Name:            subsystem,
				MetricsProvider: queueMetricsProvider,
			},
		),
	}
}

var (
	patcher             *languagePatcher
	languagePatcherOnce sync.Once
)

// Start initializes and starts the language detection patcher
func Start(ctx context.Context, store workloadmeta.Component, logger log.Component, datadogConfig config.Component) error {

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
		patcher = newLanguagePatcher(ctx, store, logger, datadogConfig, apiCl)
		go patcher.run(ctx)
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

func (lp *languagePatcher) run(ctx context.Context) {
	defer lp.logger.Info("Shutting down language detection patcher")

	lp.startProcessingPatchingRequests(ctx)

	// Capture all set events
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceLanguageDetectionServer).
		AddKind(workloadmeta.KindKubernetesDeployment).
		Build()

	eventCh := lp.store.Subscribe(
		subscriber,
		workloadmeta.NormalPriority,
		filter,
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
			eventBundle.Acknowledge()
			lp.handleEvents(eventBundle.Events)
		}
	}
}

// handleEvents handles events from workloadmeta
func (lp *languagePatcher) handleEvents(events []workloadmeta.Event) {
	lp.logger.Tracef("Processing %d events", len(events))

	for _, event := range events {
		owner, err := lp.extractOwnerFromEvent(event)

		if err != nil {
			lp.logger.Errorf("failed to handle event: %v", err)
		}

		lp.queue.Add(*owner)
	}
}

func (lp *languagePatcher) extractOwnerFromEvent(event workloadmeta.Event) (*langUtil.NamespacedOwnerReference, error) {
	entityKind := event.Entity.GetID().Kind

	switch entityKind {
	case workloadmeta.KindKubernetesDeployment:
		// Case of deployment
		deploymentID := event.Entity.(*workloadmeta.KubernetesDeployment).ID

		// extract deployment name and namespace from entity id
		deploymentIDs := strings.Split(deploymentID, "/")
		namespace := deploymentIDs[0]
		deploymentName := deploymentIDs[1]

		// construct namespaced owner reference
		owner := langUtil.NewNamespacedOwnerReference(
			"apps/v1",
			langUtil.KindDeployment,
			deploymentName,
			namespace,
		)

		return &owner, nil
	default:
		return nil, fmt.Errorf("Unsupported entity kind: %v", entityKind)
	}
}

func (lp *languagePatcher) startProcessingPatchingRequests(ctx context.Context) {
	go func() {
		<-ctx.Done()
		lp.queue.ShutDown()
	}()
	go func() {
		for {
			obj, shutdown := lp.queue.Get()
			if shutdown {
				break
			}

			owner, ok := obj.(langUtil.NamespacedOwnerReference)
			if !ok {
				// The item in the queue was not of the expected type. This should not happen.
				lp.logger.Errorf("The item in the queue is not of the expected type (i.e. NamespacedOwnerReference). This should not have happened.")
				lp.queue.Forget(obj)
				continue
			}

			err := lp.processOwner(ctx, owner)
			if err != nil {
				lp.logger.Errorf("Failed processing %s: %s/%s. It will be retried later: %v", owner.Kind, owner.Namespace, owner.Name, err)
				Patches.Inc(owner.Kind, owner.Name, owner.Namespace, statusError)
				lp.queue.AddRateLimited(owner)
			} else {
				lp.queue.Forget(obj)
			}

			lp.queue.Done(obj)
		}
	}()
}

func (lp *languagePatcher) processOwner(ctx context.Context, owner langUtil.NamespacedOwnerReference) error {
	lp.logger.Tracef("Processing %s: %s/%s", owner.Kind, owner.Namespace, owner.Name)

	var err error

	switch owner.Kind {
	case langUtil.KindDeployment:
		err = lp.handleDeployment(ctx, owner)
	}

	return err
}

func (lp *languagePatcher) handleDeployment(ctx context.Context, owner langUtil.NamespacedOwnerReference) error {
	deploymentID := fmt.Sprintf("%s/%s", owner.Namespace, owner.Name)

	// get the complete entity
	deployment, err := lp.store.GetKubernetesDeployment(deploymentID)

	if err != nil {
		lp.logger.Info("Didn't find deployment in store, skipping")
		// skip if not in store
		return nil
	}

	detectedLanguages := deployment.DetectedLanguages
	injectableLanguages := deployment.InjectableLanguages

	// Calculate annotations patch
	annotationsPatch := lp.generateAnnotationsPatch(injectableLanguages, detectedLanguages)
	if len(annotationsPatch) > 0 {
		err = lp.patchOwner(ctx, &owner, annotationsPatch)
	} else {
		Patches.Inc(owner.Kind, owner.Name, owner.Namespace, statusSkip)
	}

	return err
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

// patches the owner with the corresponding language annotations
func (lp *languagePatcher) patchOwner(ctx context.Context, namespacedOwnerRef *langUtil.NamespacedOwnerReference, annotationsPatch map[string]interface{}) error {

	setAnnotations := map[string]string{}
	for k, v := range annotationsPatch {
		if v != nil {
			setAnnotations[k] = fmt.Sprintf("%v", v)
		}
	}

	errs := validation.ValidateAnnotations(setAnnotations, field.NewPath("annotations"))

	if len(errs) > 0 {
		return errors.New(errs.ToAggregate().Error())
	}

	ownerGVR, err := langUtil.GetGVR(namespacedOwnerRef)
	if err != nil {
		return err
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

		_, err = lp.k8sClient.Resource(ownerGVR).Namespace(namespacedOwnerRef.Namespace).Patch(ctx, namespacedOwnerRef.Name, types.MergePatchType, patchData, metav1.PatchOptions{})
		if err != nil {
			Patches.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.Namespace, statusRetry)
		}

		return err
	})

	if retryErr != nil {
		return retryErr
	}

	Patches.Inc(namespacedOwnerRef.Kind, namespacedOwnerRef.Name, namespacedOwnerRef.Namespace, statusSuccess)
	return nil
}
