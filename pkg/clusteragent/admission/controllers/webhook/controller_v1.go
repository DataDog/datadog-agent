// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"context"
	"strings"
	"time"

	admiv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	admissioninformers "k8s.io/client-go/informers/admissionregistration/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	admissionlisters "k8s.io/client-go/listers/admissionregistration/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ControllerV1 is responsible for watching the TLS certificate stored
// in a Secret and reconciling the webhook configuration based on it.
// It uses the admissionregistration/v1 API.
type ControllerV1 struct {
	controllerBase
	validatingWebhooksInformer cache.SharedIndexInformer
	validatingWebhooksLister   admissionlisters.ValidatingWebhookConfigurationLister
	validatingWebhookTemplates []admiv1.ValidatingWebhook
	mutatingWebhooksLister     admissionlisters.MutatingWebhookConfigurationLister
	mutatingWebhookTemplates   []admiv1.MutatingWebhook
}

// NewControllerV1 returns a new Webhook Controller using admissionregistration/v1.
func NewControllerV1(
	client kubernetes.Interface,
	secretInformer coreinformers.SecretInformer,
	validatingWebhookInformer admissioninformers.ValidatingWebhookConfigurationInformer,
	mutatingWebhookInformer admissioninformers.MutatingWebhookConfigurationInformer,
	isLeaderFunc func() bool,
	isLeaderNotif <-chan struct{},
	config Config,
	wmeta workloadmeta.Component,
	pa workload.PodPatcher,
	datadogConfig config.Component,
	demultiplexer demultiplexer.Component,
) *ControllerV1 {
	controller := &ControllerV1{}
	controller.clientSet = client
	controller.config = config
	controller.secretsLister = secretInformer.Lister()
	controller.secretsSynced = secretInformer.Informer().HasSynced
	controller.validatingWebhooksInformer = validatingWebhookInformer.Informer()
	controller.validatingWebhooksLister = validatingWebhookInformer.Lister()
	controller.validatingWebhooksSynced = validatingWebhookInformer.Informer().HasSynced
	controller.mutatingWebhooksLister = mutatingWebhookInformer.Lister()
	controller.mutatingWebhooksSynced = mutatingWebhookInformer.Informer().HasSynced
	controller.queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: "webhooks"},
	)
	controller.isLeaderFunc = isLeaderFunc
	controller.isLeaderNotif = isLeaderNotif
	controller.webhooks = controller.generateWebhooks(wmeta, pa, datadogConfig, demultiplexer)
	controller.generateTemplates()

	if _, err := secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.handleSecret,
		UpdateFunc: controller.handleSecretUpdate,
		DeleteFunc: controller.handleSecret,
	}); err != nil {
		log.Errorf("cannot add event handler to secret informer: %v", err)
	}

	if _, err := validatingWebhookInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.handleWebhook,
		UpdateFunc: controller.handleWebhookUpdate,
		DeleteFunc: controller.handleWebhook,
	}); err != nil {
		log.Errorf("cannot add event handler to validating webhook informer: %v", err)
	}

	if _, err := mutatingWebhookInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.handleWebhook,
		UpdateFunc: controller.handleWebhookUpdate,
		DeleteFunc: controller.handleWebhook,
	}); err != nil {
		log.Errorf("cannot add event handler to mutating webhook informer: %v", err)
	}

	return controller
}

// Run starts the controller to process Secret and Webhook
// events after sync'ing the informer's cache.
func (c *ControllerV1) Run(stopCh <-chan struct{}) {
	defer c.queue.ShutDown()

	log.Infof("Starting webhook controller for secret %s/%s and webhook %s - Using admissionregistration/v1", c.config.getSecretNs(), c.config.getSecretName(), c.config.getWebhookName())
	defer log.Infof("Stopping webhook controller for secret %s/%s and webhook %s", c.config.getSecretNs(), c.config.getSecretName(), c.config.getWebhookName())

	// Check if ValidatingWebhookConfiguration RBACs are enabled.
	err := apiserver.SyncInformers(map[apiserver.InformerName]cache.SharedInformer{apiserver.ValidatingWebhooksInformer: c.validatingWebhooksInformer}, 0)
	if err != nil {
		log.Warnf("Validating Webhook Informer not synced: disabling validation webhook controller")
		c.config.validationEnabled = false
	}

	syncedInformer := []cache.InformerSynced{
		c.secretsSynced,
	}
	if c.config.validationEnabled {
		syncedInformer = append(syncedInformer, c.validatingWebhooksSynced)
	}
	if c.config.mutationEnabled {
		syncedInformer = append(syncedInformer, c.mutatingWebhooksSynced)
	}

	if ok := cache.WaitForCacheSync(stopCh, syncedInformer...); !ok {
		return
	}

	go c.enqueueOnLeaderNotif(stopCh)
	go wait.Until(c.run, time.Second, stopCh)

	// Trigger a reconciliation to create the Webhook if it doesn't exist
	c.triggerReconciliation()

	<-stopCh
}

// run waits for items to process in the work queue.
func (c *ControllerV1) run() {
	for c.processNextWorkItem(c.reconcile) {
	}
}

// handleWebhookUpdate handles the new Webhook reported in update events.
// It can be a callback function for update events.
func (c *ControllerV1) handleWebhookUpdate(oldObj, newObj interface{}) {
	if !c.isLeaderFunc() {
		return
	}

	switch newObj.(type) {
	case *admiv1.ValidatingWebhookConfiguration:
		newWebhook, _ := newObj.(*admiv1.ValidatingWebhookConfiguration)
		oldWebhook, ok := oldObj.(*admiv1.ValidatingWebhookConfiguration)
		if !ok {
			log.Debugf("Expected ValidatingWebhookConfiguration object, got: %v", oldObj)
			return
		}

		if newWebhook.ResourceVersion == oldWebhook.ResourceVersion {
			return
		}
		c.handleWebhook(newObj)
	case *admiv1.MutatingWebhookConfiguration:
		newWebhook, _ := newObj.(*admiv1.MutatingWebhookConfiguration)
		oldWebhook, ok := oldObj.(*admiv1.MutatingWebhookConfiguration)
		if !ok {
			log.Debugf("Expected MutatingWebhookConfiguration object, got: %v", oldObj)
			return
		}

		if newWebhook.ResourceVersion == oldWebhook.ResourceVersion {
			return
		}
		c.handleWebhook(newObj)
	default:
		log.Debugf("Expected ValidatingWebhookConfiguration or MutatingWebhookConfiguration object, got: %v", newObj)
		return
	}
}

// reconcile creates/updates the webhook object on new events.
func (c *ControllerV1) reconcile() error {
	secret, err := c.getSecret()
	if err != nil {
		return err
	}

	if c.config.mutationEnabled {
		mutatingWebhook, err := c.mutatingWebhooksLister.Get(c.config.getWebhookName())
		if err != nil {
			if errors.IsNotFound(err) {
				log.Infof("Mutating Webhook %s was not found, creating it", c.config.getWebhookName())
				err := c.createMutatingWebhook(secret)
				if err != nil {
					log.Errorf("Failed to create Mutating Webhook %s: %v", c.config.getWebhookName(), err)
				}
			}
		} else {
			log.Debugf("Mutating Webhook %s was found, updating it", c.config.getWebhookName())
			err := c.updateMutatingWebhook(secret, mutatingWebhook)
			if err != nil {
				log.Errorf("Failed to update Mutating Webhook %s: %v", c.config.getWebhookName(), err)
			}
		}
	}

	if c.config.validationEnabled {
		validatingWebhook, err := c.validatingWebhooksLister.Get(c.config.getWebhookName())
		if err != nil {
			if errors.IsNotFound(err) {
				log.Infof("Validating Webhook %s was not found, creating it", c.config.getWebhookName())
				err := c.createValidatingWebhook(secret)
				if err != nil {
					log.Errorf("Failed to create Validating Webhook %s: %v", c.config.getWebhookName(), err)
				}
			}
		} else {
			log.Debugf("Validating Webhook %s was found, updating it", c.config.getWebhookName())
			err := c.updateValidatingWebhook(secret, validatingWebhook)
			if err != nil {
				log.Errorf("Failed to update Validating Webhook %s: %v", c.config.getWebhookName(), err)
			}
		}
	}

	return err
}

// createValidatingWebhook creates a new ValidatingWebhookConfiguration object.
func (c *ControllerV1) createValidatingWebhook(secret *corev1.Secret) error {
	webhook := &admiv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.getWebhookName(),
		},
		Webhooks: c.newValidatingWebhooks(secret),
	}

	_, err := c.clientSet.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.TODO(), webhook, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		log.Infof("Validating Webhook %s already exists", webhook.GetName())
		return nil
	}

	return err
}

// updateValidatingWebhook stores a new configuration in the ValidatingWebhookConfiguration object.
func (c *ControllerV1) updateValidatingWebhook(secret *corev1.Secret, webhook *admiv1.ValidatingWebhookConfiguration) error {
	webhook = webhook.DeepCopy()
	webhook.Webhooks = c.newValidatingWebhooks(secret)
	_, err := c.clientSet.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(context.TODO(), webhook, metav1.UpdateOptions{})
	return err
}

// newValidatingWebhooks generates Webhook objects from config templates with updated CABundle from Secret.
func (c *ControllerV1) newValidatingWebhooks(secret *corev1.Secret) []admiv1.ValidatingWebhook {
	webhooks := []admiv1.ValidatingWebhook{}
	for _, tpl := range c.validatingWebhookTemplates {
		tpl.ClientConfig.CABundle = certificate.GetCABundle(secret.Data)
		webhooks = append(webhooks, tpl)
	}

	return webhooks
}

// createMutatingWebhook creates a new MutatingWebhookConfiguration object.
func (c *ControllerV1) createMutatingWebhook(secret *corev1.Secret) error {
	webhook := &admiv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.getWebhookName(),
		},
		Webhooks: c.newMutatingWebhooks(secret),
	}

	_, err := c.clientSet.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.TODO(), webhook, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		log.Infof("Mutating Webhook %s already exists", webhook.GetName())
		return nil
	}

	return err
}

// updateMutatingWebhook stores a new configuration in the MutatingWebhookConfiguration object.
func (c *ControllerV1) updateMutatingWebhook(secret *corev1.Secret, webhook *admiv1.MutatingWebhookConfiguration) error {
	webhook = webhook.DeepCopy()
	webhook.Webhooks = c.newMutatingWebhooks(secret)
	_, err := c.clientSet.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(context.TODO(), webhook, metav1.UpdateOptions{})
	return err
}

// newMutatingWebhooks generates Webhook objects from config templates with updated CABundle from Secret.
func (c *ControllerV1) newMutatingWebhooks(secret *corev1.Secret) []admiv1.MutatingWebhook {
	webhooks := []admiv1.MutatingWebhook{}
	for _, tpl := range c.mutatingWebhookTemplates {
		tpl.ClientConfig.CABundle = certificate.GetCABundle(secret.Data)
		webhooks = append(webhooks, tpl)
	}

	return webhooks
}

// generateTemplates generates the webhook templates from the configuration.
func (c *ControllerV1) generateTemplates() {
	// Generate validating webhook templates
	validatingWebhooks := []admiv1.ValidatingWebhook{}
	for _, webhook := range c.webhooks {
		if !webhook.IsEnabled() || webhook.WebhookType() != common.ValidatingWebhook {
			continue
		}
		nsSelector, objSelector := webhook.LabelSelectors(c.config.useNamespaceSelector())
		validatingWebhooks = append(
			validatingWebhooks,
			c.getValidatingWebhookSkeleton(
				webhook.Name(),
				webhook.Endpoint(),
				webhook.Operations(),
				webhook.Resources(),
				nsSelector,
				objSelector,
				webhook.MatchConditions(),
			),
		)
	}
	c.validatingWebhookTemplates = validatingWebhooks

	// Generate mutating webhook templates
	mutatingWebhooks := []admiv1.MutatingWebhook{}
	for _, webhook := range c.webhooks {
		if !webhook.IsEnabled() || webhook.WebhookType() != common.MutatingWebhook {
			continue
		}
		nsSelector, objSelector := webhook.LabelSelectors(c.config.useNamespaceSelector())
		mutatingWebhooks = append(
			mutatingWebhooks,
			c.getMutatingWebhookSkeleton(
				webhook.Name(),
				webhook.Endpoint(),
				webhook.Operations(),
				webhook.Resources(),
				nsSelector,
				objSelector,
				webhook.MatchConditions(),
			),
		)
	}
	c.mutatingWebhookTemplates = mutatingWebhooks
}

func (c *ControllerV1) getValidatingWebhookSkeleton(nameSuffix, path string, operations []admiv1.OperationType, resourcesMap map[string][]string, namespaceSelector, objectSelector *metav1.LabelSelector, matchConditions []admiv1.MatchCondition) admiv1.ValidatingWebhook {
	matchPolicy := admiv1.Exact
	sideEffects := admiv1.SideEffectClassNone
	port := c.config.getServicePort()
	timeout := c.config.getTimeout()
	failurePolicy := c.getFailurePolicy()

	webhook := admiv1.ValidatingWebhook{
		Name: c.config.configName(nameSuffix),
		ClientConfig: admiv1.WebhookClientConfig{
			Service: &admiv1.ServiceReference{
				Namespace: c.config.getServiceNs(),
				Name:      c.config.getServiceName(),
				Port:      &port,
				Path:      &path,
			},
		},
		FailurePolicy:           &failurePolicy,
		MatchPolicy:             &matchPolicy,
		SideEffects:             &sideEffects,
		TimeoutSeconds:          &timeout,
		AdmissionReviewVersions: []string{"v1", "v1beta1"},
		NamespaceSelector:       namespaceSelector,
		ObjectSelector:          objectSelector,
		MatchConditions:         matchConditions,
	}

	for group, resources := range resourcesMap {
		for _, resource := range resources {
			webhook.Rules = append(webhook.Rules, admiv1.RuleWithOperations{
				Operations: operations,
				Rule: admiv1.Rule{
					APIGroups:   []string{group},
					APIVersions: []string{"v1"},
					Resources:   []string{resource},
				},
			})
		}
	}

	return webhook
}

func (c *ControllerV1) getMutatingWebhookSkeleton(nameSuffix, path string, operations []admiv1.OperationType, resourcesMap map[string][]string, namespaceSelector, objectSelector *metav1.LabelSelector, matchConditions []admiv1.MatchCondition) admiv1.MutatingWebhook {
	matchPolicy := admiv1.Exact
	sideEffects := admiv1.SideEffectClassNone
	port := c.config.getServicePort()
	timeout := c.config.getTimeout()
	failurePolicy := c.getFailurePolicy()
	reinvocationPolicy := c.getReinvocationPolicy()

	webhook := admiv1.MutatingWebhook{
		Name: c.config.configName(nameSuffix),
		ClientConfig: admiv1.WebhookClientConfig{
			Service: &admiv1.ServiceReference{
				Namespace: c.config.getServiceNs(),
				Name:      c.config.getServiceName(),
				Port:      &port,
				Path:      &path,
			},
		},
		ReinvocationPolicy:      &reinvocationPolicy,
		FailurePolicy:           &failurePolicy,
		MatchPolicy:             &matchPolicy,
		SideEffects:             &sideEffects,
		TimeoutSeconds:          &timeout,
		AdmissionReviewVersions: []string{"v1", "v1beta1"},
		NamespaceSelector:       namespaceSelector,
		ObjectSelector:          objectSelector,
		MatchConditions:         matchConditions,
	}

	for group, resources := range resourcesMap {
		for _, resource := range resources {
			webhook.Rules = append(webhook.Rules, admiv1.RuleWithOperations{
				Operations: operations,
				Rule: admiv1.Rule{
					APIGroups:   []string{group},
					APIVersions: []string{"v1"},
					Resources:   []string{resource},
				},
			})
		}
	}

	return webhook
}

func (c *ControllerV1) getFailurePolicy() admiv1.FailurePolicyType {
	policy := strings.ToLower(c.config.getFailurePolicy())
	switch policy {
	case "ignore":
		return admiv1.Ignore
	case "fail":
		return admiv1.Fail
	default:
		log.Warnf("Unknown failure policy %s - defaulting to 'Ignore'", policy)
		return admiv1.Ignore
	}
}

func (c *ControllerV1) getReinvocationPolicy() admiv1.ReinvocationPolicyType {
	policy := strings.ToLower(c.config.getReinvocationPolicy())
	switch policy {
	case "ifneeded":
		return admiv1.IfNeededReinvocationPolicy
	case "never":
		return admiv1.NeverReinvocationPolicy
	default:
		log.Warnf("Unknown reinvocation policy %q - defaulting to %q", c.config.getReinvocationPolicy(), admiv1.IfNeededReinvocationPolicy)
		return admiv1.IfNeededReinvocationPolicy
	}
}
