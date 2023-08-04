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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"

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
)

// ControllerV1 is responsible for watching the TLS certificate stored
// in a Secret and reconciling the webhook configuration based on it.
// It uses the admissionregistration/v1 API.
type ControllerV1 struct {
	controllerBase
	webhooksLister   admissionlisters.MutatingWebhookConfigurationLister
	webhookTemplates []admiv1.MutatingWebhook
}

// NewControllerV1 returns a new Webhook Controller using admissionregistration/v1.
func NewControllerV1(client kubernetes.Interface, secretInformer coreinformers.SecretInformer, webhookInformer admissioninformers.MutatingWebhookConfigurationInformer, isLeaderFunc func() bool, isLeaderNotif <-chan struct{}, config Config) *ControllerV1 {
	controller := &ControllerV1{}
	controller.clientSet = client
	controller.config = config
	controller.secretsLister = secretInformer.Lister()
	controller.secretsSynced = secretInformer.Informer().HasSynced
	controller.webhooksLister = webhookInformer.Lister()
	controller.webhooksSynced = webhookInformer.Informer().HasSynced
	controller.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "webhooks")
	controller.isLeaderFunc = isLeaderFunc
	controller.isLeaderNotif = isLeaderNotif
	controller.generateTemplates()

	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.handleSecret,
		UpdateFunc: controller.handleSecretUpdate,
		DeleteFunc: controller.handleSecret,
	})

	webhookInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.handleWebhook,
		UpdateFunc: controller.handleWebhookUpdate,
		DeleteFunc: controller.handleWebhook,
	})

	return controller
}

// Run starts the controller to process Secret and Webhook
// events after sync'ing the informer's cache.
func (c *ControllerV1) Run(stopCh <-chan struct{}) {
	defer c.queue.ShutDown()

	log.Infof("Starting webhook controller for secret %s/%s and webhook %s - Using admissionregistration/v1", c.config.getSecretNs(), c.config.getSecretName(), c.config.getWebhookName())
	defer log.Infof("Stopping webhook controller for secret %s/%s and webhook %s", c.config.getSecretNs(), c.config.getSecretName(), c.config.getWebhookName())

	if ok := cache.WaitForCacheSync(stopCh, c.secretsSynced, c.webhooksSynced); !ok {
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

	newWebhook, ok := newObj.(*admiv1.MutatingWebhookConfiguration)
	if !ok {
		log.Debugf("Expected MutatingWebhookConfiguration object, got: %v", newObj)
		return
	}

	oldWebhook, ok := oldObj.(*admiv1.MutatingWebhookConfiguration)
	if !ok {
		log.Debugf("Expected MutatingWebhookConfiguration object, got: %v", oldObj)
		return
	}

	if newWebhook.ResourceVersion == oldWebhook.ResourceVersion {
		return
	}

	c.handleWebhook(newObj)
}

// reconcile creates/updates the webhook object on new events.
func (c *ControllerV1) reconcile() error {
	secret, err := c.getSecret()
	if err != nil {
		return err
	}

	webhook, err := c.webhooksLister.Get(c.config.getWebhookName())
	if err != nil {
		if errors.IsNotFound(err) {
			log.Infof("Webhook %s was not found, creating it", c.config.getWebhookName())
			return c.createWebhook(secret)
		}

		return err
	}

	log.Debugf("The Webhook %s was found, updating it", c.config.getWebhookName())

	return c.updateWebhook(secret, webhook)
}

// createWebhook creates a new MutatingWebhookConfiguration object.
func (c *ControllerV1) createWebhook(secret *corev1.Secret) error {
	webhook := &admiv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.getWebhookName(),
		},
		Webhooks: c.newWebhooks(secret),
	}

	_, err := c.clientSet.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.TODO(), webhook, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		log.Infof("Webhook %s already exists", webhook.GetName())
		return nil
	}

	return err
}

// updateWebhook stores a new configuration in the MutatingWebhookConfiguration object.
func (c *ControllerV1) updateWebhook(secret *corev1.Secret, webhook *admiv1.MutatingWebhookConfiguration) error {
	webhook = webhook.DeepCopy()
	webhook.Webhooks = c.newWebhooks(secret)
	_, err := c.clientSet.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(context.TODO(), webhook, metav1.UpdateOptions{})
	return err
}

// newWebhooks generates MutatingWebhook objects from config templates with updated CABundle from Secret.
func (c *ControllerV1) newWebhooks(secret *corev1.Secret) []admiv1.MutatingWebhook {
	webhooks := []admiv1.MutatingWebhook{}
	for _, tpl := range c.webhookTemplates {
		tpl.ClientConfig.CABundle = certificate.GetCABundle(secret.Data)
		webhooks = append(webhooks, tpl)
	}

	return webhooks
}

func (c *ControllerV1) generateTemplates() {
	webhooks := []admiv1.MutatingWebhook{}

	// DD_AGENT_HOST injection
	if config.Datadog.GetBool("admission_controller.inject_config.enabled") {
		webhook := c.getWebhookSkeleton("config", config.Datadog.GetString("admission_controller.inject_config.endpoint"))
		webhooks = append(webhooks, webhook)
	}

	// DD_ENV, DD_VERSION, DD_SERVICE injection
	if config.Datadog.GetBool("admission_controller.inject_tags.enabled") {
		webhook := c.getWebhookSkeleton("tags", config.Datadog.GetString("admission_controller.inject_tags.endpoint"))
		webhooks = append(webhooks, webhook)
	}

	// Auto instrumentation - lib injection
	if config.Datadog.GetBool("admission_controller.auto_instrumentation.enabled") {
		webhook := c.getWebhookSkeleton("auto-instrumentation", config.Datadog.GetString("admission_controller.auto_instrumentation.endpoint"))
		webhooks = append(webhooks, webhook)
	}

	c.webhookTemplates = webhooks
}

func (c *ControllerV1) getWebhookSkeleton(nameSuffix, path string) admiv1.MutatingWebhook {
	matchPolicy := admiv1.Exact
	sideEffects := admiv1.SideEffectClassNone
	port := c.config.getServicePort()
	timeout := c.config.getTimeout()
	failurePolicy := c.getAdmiV1FailurePolicy()
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
		Rules: []admiv1.RuleWithOperations{
			{
				Operations: []admiv1.OperationType{
					admiv1.Create,
				},
				Rule: admiv1.Rule{
					APIGroups:   []string{""},
					APIVersions: []string{"v1"},
					Resources:   []string{"pods"},
				},
			},
		},
		ReinvocationPolicy:      &reinvocationPolicy,
		FailurePolicy:           &failurePolicy,
		MatchPolicy:             &matchPolicy,
		SideEffects:             &sideEffects,
		TimeoutSeconds:          &timeout,
		AdmissionReviewVersions: []string{"v1", "v1beta1"},
	}

	webhook.NamespaceSelector, webhook.ObjectSelector = buildLabelSelectors(c.config.useNamespaceSelector())

	return webhook
}

func (c *ControllerV1) getAdmiV1FailurePolicy() admiv1.FailurePolicyType {
	policy := strings.ToLower(c.config.getFailurePolicy())
	switch policy {
	case "ignore":
		return admiv1.Ignore
	case "fail":
		return admiv1.Fail
	default:
		_ = log.Warnf("Unknown failure policy %s - defaulting to 'Ignore'", policy)
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
