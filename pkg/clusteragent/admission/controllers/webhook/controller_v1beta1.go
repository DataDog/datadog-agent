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
	admiv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	admissioninformers "k8s.io/client-go/informers/admissionregistration/v1beta1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	admissionlisters "k8s.io/client-go/listers/admissionregistration/v1beta1"
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

// ControllerV1beta1 is responsible for watching the TLS certificate stored
// in a Secret and reconciling the webhook configuration based on it.
// It uses the admissionregistration/v1beta1 API.
type ControllerV1beta1 struct {
	controllerBase
	validatingWebhooksInformer cache.SharedIndexInformer
	validatingWebhooksLister   admissionlisters.ValidatingWebhookConfigurationLister
	validatingWebhookTemplates []admiv1beta1.ValidatingWebhook
	mutatingWebhooksLister     admissionlisters.MutatingWebhookConfigurationLister
	mutatingWebhookTemplates   []admiv1beta1.MutatingWebhook
}

// NewControllerV1beta1 returns a new Webhook Controller using admissionregistration/v1beta1.
func NewControllerV1beta1(
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
) *ControllerV1beta1 {
	controller := &ControllerV1beta1{}
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
		log.Errorf("cannot add event handler to webhook informer: %v", err)
	}

	if _, err := mutatingWebhookInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.handleWebhook,
		UpdateFunc: controller.handleWebhookUpdate,
		DeleteFunc: controller.handleWebhook,
	}); err != nil {
		log.Errorf("cannot add event handler to webhook informer: %v", err)
	}

	return controller
}

// Run starts the controller to process Secret and Webhook
// events after sync'ing the informer's cache.
func (c *ControllerV1beta1) Run(stopCh <-chan struct{}) {
	defer c.queue.ShutDown()

	log.Infof("Starting webhook controller for secret %s/%s and webhook %s - Using admissionregistration/v1beta1", c.config.getSecretNs(), c.config.getSecretName(), c.config.getWebhookName())
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
func (c *ControllerV1beta1) run() {
	for c.processNextWorkItem(c.reconcile) {
	}
}

// handleWebhookUpdate handles the new Webhook reported in update events.
// It can be a callback function for update events.
func (c *ControllerV1beta1) handleWebhookUpdate(oldObj, newObj interface{}) {
	if !c.isLeaderFunc() {
		return
	}

	switch newObj.(type) {
	case *admiv1beta1.ValidatingWebhookConfiguration:
		newWebhook, _ := newObj.(*admiv1beta1.ValidatingWebhookConfiguration)
		oldWebhook, ok := oldObj.(*admiv1beta1.ValidatingWebhookConfiguration)
		if !ok {
			log.Debugf("Expected ValidatingWebhookConfiguration object, got: %v", oldObj)
			return
		}

		if newWebhook.ResourceVersion == oldWebhook.ResourceVersion {
			return
		}
		c.handleWebhook(newObj)
	case *admiv1beta1.MutatingWebhookConfiguration:
		newWebhook, _ := newObj.(*admiv1beta1.MutatingWebhookConfiguration)
		oldWebhook, ok := oldObj.(*admiv1beta1.MutatingWebhookConfiguration)
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
func (c *ControllerV1beta1) reconcile() error {
	secret, err := c.getSecret()
	if err != nil {
		return err
	}

	if c.config.mutationEnabled {
		mutatingWebhook, err := c.mutatingWebhooksLister.Get(c.config.getWebhookName())
		if err != nil {
			if errors.IsNotFound(err) {
				log.Infof("Webhook %s was not found, creating it", c.config.getWebhookName())
				err = c.createMutatingWebhook(secret)
				if err != nil {
					log.Errorf("Failed to create Mutating Webhook %s: %v", c.config.getWebhookName(), err)
				}
			}
		} else {
			log.Debugf("The Webhook %s was found, updating it", c.config.getWebhookName())
			err = c.updateMutatingWebhook(secret, mutatingWebhook)
			if err != nil {
				log.Errorf("Failed to update Mutating Webhook %s: %v", c.config.getWebhookName(), err)
			}
		}
	}

	if c.config.validationEnabled {
		validatingWebhook, err := c.validatingWebhooksLister.Get(c.config.getWebhookName())
		if err != nil {
			if errors.IsNotFound(err) {
				log.Infof("Webhook %s was not found, creating it", c.config.getWebhookName())
				err = c.createValidatingWebhook(secret)
				if err != nil {
					log.Errorf("Failed to create Validating Webhook %s: %v", c.config.getWebhookName(), err)
				}
			}
		} else {
			log.Debugf("The Webhook %s was found, updating it", c.config.getWebhookName())
			err = c.updateValidatingWebhook(secret, validatingWebhook)
			if err != nil {
				log.Errorf("Failed to update Validating Webhook %s: %v", c.config.getWebhookName(), err)
			}
		}
	}

	return err
}

// createValidatingWebhook creates a new ValidatingWebhookConfiguration object.
func (c *ControllerV1beta1) createValidatingWebhook(secret *corev1.Secret) error {
	webhook := &admiv1beta1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.getWebhookName(),
		},
		Webhooks: c.newValidatingWebhooks(secret),
	}

	_, err := c.clientSet.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Create(context.TODO(), webhook, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		log.Infof("Webhook %s already exists", webhook.GetName())
		return nil
	}

	return err
}

// updateValidatingWebhook stores a new configuration in the ValidatingWebhookConfiguration object.
func (c *ControllerV1beta1) updateValidatingWebhook(secret *corev1.Secret, webhook *admiv1beta1.ValidatingWebhookConfiguration) error {
	webhook = webhook.DeepCopy()
	webhook.Webhooks = c.newValidatingWebhooks(secret)
	_, err := c.clientSet.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Update(context.TODO(), webhook, metav1.UpdateOptions{})
	return err
}

// newValidatingWebhooks generates Webhook objects from config templates with updated CABundle from Secret.
func (c *ControllerV1beta1) newValidatingWebhooks(secret *corev1.Secret) []admiv1beta1.ValidatingWebhook {
	webhooks := []admiv1beta1.ValidatingWebhook{}
	for _, tpl := range c.validatingWebhookTemplates {
		tpl.ClientConfig.CABundle = certificate.GetCABundle(secret.Data)
		webhooks = append(webhooks, tpl)
	}

	return webhooks
}

// createMutatingWebhook creates a new MutatingWebhookConfiguration object.
func (c *ControllerV1beta1) createMutatingWebhook(secret *corev1.Secret) error {
	webhook := &admiv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.getWebhookName(),
		},
		Webhooks: c.newMutatingWebhooks(secret),
	}

	_, err := c.clientSet.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(context.TODO(), webhook, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		log.Infof("Webhook %s already exists", webhook.GetName())
		return nil
	}

	return err
}

// updateMutatingWebhook stores a new config in the MutatingWebhookConfiguration object.
func (c *ControllerV1beta1) updateMutatingWebhook(secret *corev1.Secret, webhook *admiv1beta1.MutatingWebhookConfiguration) error {
	webhook = webhook.DeepCopy()
	webhook.Webhooks = c.newMutatingWebhooks(secret)
	_, err := c.clientSet.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Update(context.TODO(), webhook, metav1.UpdateOptions{})

	return err
}

// newWebhooks generates Webhook objects from config templates with updated CABundle from Secret.
func (c *ControllerV1beta1) newMutatingWebhooks(secret *corev1.Secret) []admiv1beta1.MutatingWebhook {
	webhooks := []admiv1beta1.MutatingWebhook{}
	for _, tpl := range c.mutatingWebhookTemplates {
		tpl.ClientConfig.CABundle = certificate.GetCABundle(secret.Data)
		webhooks = append(webhooks, tpl)
	}

	return webhooks
}

func (c *ControllerV1beta1) generateTemplates() {
	validatingWebhooks := []admiv1beta1.ValidatingWebhook{}
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
				convertMatchConditions(webhook.MatchConditions()),
			),
		)
	}
	c.validatingWebhookTemplates = validatingWebhooks

	mutatingWebhooks := []admiv1beta1.MutatingWebhook{}
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
				convertMatchConditions(webhook.MatchConditions()),
			),
		)
	}
	c.mutatingWebhookTemplates = mutatingWebhooks
}

func (c *ControllerV1beta1) getValidatingWebhookSkeleton(nameSuffix, path string, operations []admiv1beta1.OperationType, resourcesMap map[string][]string, namespaceSelector, objectSelector *metav1.LabelSelector, matchConditions []admiv1beta1.MatchCondition) admiv1beta1.ValidatingWebhook {
	matchPolicy := admiv1beta1.Exact
	sideEffects := admiv1beta1.SideEffectClassNone
	port := c.config.getServicePort()
	timeout := c.config.getTimeout()
	failurePolicy := c.getFailurePolicy()

	webhook := admiv1beta1.ValidatingWebhook{
		Name: c.config.configName(nameSuffix),
		ClientConfig: admiv1beta1.WebhookClientConfig{
			Service: &admiv1beta1.ServiceReference{
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
		AdmissionReviewVersions: []string{"v1beta1"},
		NamespaceSelector:       namespaceSelector,
		ObjectSelector:          objectSelector,
		MatchConditions:         matchConditions,
	}

	for group, resources := range resourcesMap {
		for _, resource := range resources {
			webhook.Rules = append(webhook.Rules, admiv1beta1.RuleWithOperations{
				Operations: operations,
				Rule: admiv1beta1.Rule{
					APIGroups:   []string{group},
					APIVersions: []string{"v1"},
					Resources:   []string{resource},
				},
			})
		}
	}

	return webhook
}

func (c *ControllerV1beta1) getMutatingWebhookSkeleton(nameSuffix, path string, operations []admiv1beta1.OperationType, resourcesMap map[string][]string, namespaceSelector, objectSelector *metav1.LabelSelector, matchConditions []admiv1beta1.MatchCondition) admiv1beta1.MutatingWebhook {
	matchPolicy := admiv1beta1.Exact
	sideEffects := admiv1beta1.SideEffectClassNone
	port := c.config.getServicePort()
	timeout := c.config.getTimeout()
	failurePolicy := c.getFailurePolicy()
	reinvocationPolicy := c.getReinvocationPolicy()

	webhook := admiv1beta1.MutatingWebhook{
		Name: c.config.configName(nameSuffix),
		ClientConfig: admiv1beta1.WebhookClientConfig{
			Service: &admiv1beta1.ServiceReference{
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
		AdmissionReviewVersions: []string{"v1beta1"},
		NamespaceSelector:       namespaceSelector,
		ObjectSelector:          objectSelector,
		MatchConditions:         matchConditions,
	}

	for group, resources := range resourcesMap {
		for _, resource := range resources {
			webhook.Rules = append(webhook.Rules, admiv1beta1.RuleWithOperations{
				Operations: operations,
				Rule: admiv1beta1.Rule{
					APIGroups:   []string{group},
					APIVersions: []string{"v1"},
					Resources:   []string{resource},
				},
			})
		}
	}

	return webhook
}

func (c *ControllerV1beta1) getFailurePolicy() admiv1beta1.FailurePolicyType {
	policy := strings.ToLower(c.config.getFailurePolicy())
	switch policy {
	case "ignore":
		return admiv1beta1.Ignore
	case "fail":
		return admiv1beta1.Fail
	default:
		log.Warnf("Unknown failure policy %s - defaulting to 'Ignore'", policy)
		return admiv1beta1.Ignore
	}
}

func (c *ControllerV1beta1) getReinvocationPolicy() admiv1beta1.ReinvocationPolicyType {
	policy := strings.ToLower(c.config.getReinvocationPolicy())
	switch policy {
	case "ifneeded":
		return admiv1beta1.IfNeededReinvocationPolicy
	case "never":
		return admiv1beta1.NeverReinvocationPolicy
	default:
		log.Warnf("Unknown reinvocation policy %q - defaulting to %q", c.config.getReinvocationPolicy(), admiv1beta1.IfNeededReinvocationPolicy)
		return admiv1beta1.IfNeededReinvocationPolicy
	}
}

// convertMatchConditions converts the match conditions from the v1 API to the v1beta1 API.
func convertMatchConditions(v1MatchConditions []admiv1.MatchCondition) []admiv1beta1.MatchCondition {
	v1beta1MatchConditions := []admiv1beta1.MatchCondition{}
	for _, matchCondition := range v1MatchConditions {
		v1beta1MatchConditions = append(v1beta1MatchConditions, admiv1beta1.MatchCondition{
			Name:       matchCondition.Name,
			Expression: matchCondition.Expression,
		})
	}
	return v1beta1MatchConditions
}
