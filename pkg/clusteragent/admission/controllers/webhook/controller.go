// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package webhook

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	admiv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	admissioninformers "k8s.io/client-go/informers/admissionregistration/v1beta1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	admissionlisters "k8s.io/client-go/listers/admissionregistration/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controller is responsible for watching the TLS certificate stored
// in a Secret and reconciling the webhook configuration based on it.
type Controller struct {
	clientSet      kubernetes.Interface
	config         Config
	secretsLister  corelisters.SecretLister
	secretsSynced  cache.InformerSynced
	webhooksLister admissionlisters.MutatingWebhookConfigurationLister
	webhooksSynced cache.InformerSynced
	queue          workqueue.RateLimitingInterface
	isLeaderFunc   func() bool
}

// NewController returns a new Webhook Controller.
func NewController(client kubernetes.Interface, secretInformer coreinformers.SecretInformer, webhookInformer admissioninformers.MutatingWebhookConfigurationInformer, isLeaderFunc func() bool, config Config) *Controller {
	controller := &Controller{
		clientSet:      client,
		config:         config,
		secretsLister:  secretInformer.Lister(),
		secretsSynced:  secretInformer.Informer().HasSynced,
		webhooksLister: webhookInformer.Lister(),
		webhooksSynced: webhookInformer.Informer().HasSynced,
		queue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "webhooks"),
		isLeaderFunc:   isLeaderFunc,
	}
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
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer c.queue.ShutDown()

	log.Infof("Starting webhook controller for secret %s/%s and webhook %s", c.config.GetSecretNs(), c.config.GetSecretName(), c.config.GetName())
	defer log.Infof("Stopping webhook controller for secret %s/%s and webhook %s", c.config.GetSecretNs(), c.config.GetSecretName(), c.config.GetName())

	if ok := cache.WaitForCacheSync(stopCh, c.secretsSynced, c.webhooksSynced); !ok {
		return
	}

	go wait.Until(c.run, time.Second, stopCh)

	// Trigger a reconciliation to create the Webhook if it doesn't exist
	c.queue.Add(c.config.GetName())

	<-stopCh
}

// handleSecret enqueues the targeted Secret object when an event occurs.
// It can be a callback function for deletion and addition events.
func (c *Controller) handleSecret(obj interface{}) {
	if !c.isLeaderFunc() {
		return
	}
	if object, ok := obj.(metav1.Object); ok {
		if object.GetNamespace() == c.config.GetSecretNs() && object.GetName() == c.config.GetSecretName() {
			c.enqueue(object)
		}
	}
}

// handleSecretUpdate handles the new Secret reported in update events.
// It can be a callback function for update events.
func (c *Controller) handleSecretUpdate(oldObj, newObj interface{}) {
	if !c.isLeaderFunc() {
		return
	}
	newSecret, ok := newObj.(*corev1.Secret)
	if !ok {
		log.Debugf("Expected Secret object, got: %v", newObj)
		return
	}
	oldSecret, ok := oldObj.(*corev1.Secret)
	if !ok {
		log.Debugf("Expected Secret object, got: %v", oldObj)
		return
	}
	if newSecret.ResourceVersion == oldSecret.ResourceVersion {
		return
	}
	c.handleSecret(newObj)
}

// handleWebhook enqueues the targeted Webhook object when an event occurs.
// It can be a callback function for deletion and addition events.
func (c *Controller) handleWebhook(obj interface{}) {
	if !c.isLeaderFunc() {
		return
	}
	if object, ok := obj.(metav1.Object); ok {
		if object.GetName() == c.config.GetName() {
			c.enqueue(object)
		}
	}
}

// handleWebhookUpdate handles the new Webhook reported in update events.
// It can be a callback function for update events.
func (c *Controller) handleWebhookUpdate(oldObj, newObj interface{}) {
	if !c.isLeaderFunc() {
		return
	}
	newWebhook, ok := newObj.(*admiv1beta1.MutatingWebhookConfiguration)
	if !ok {
		log.Debugf("Expected MutatingWebhookConfiguration object, got: %v", newObj)
		return
	}
	oldWebhook, ok := oldObj.(*admiv1beta1.MutatingWebhookConfiguration)
	if !ok {
		log.Debugf("Expected MutatingWebhookConfiguration object, got: %v", oldObj)
		return
	}
	if newWebhook.ResourceVersion == oldWebhook.ResourceVersion {
		return
	}
	c.handleWebhook(newObj)
}

// enqueue adds a given object to the work queue.
func (c *Controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Debugf("Couldn't get key for object %v: %v, adding it to the queue with an unnamed key", obj, err)
		c.queue.Add(struct{}{})
		return
	}
	log.Debugf("Adding object with key %s to the queue", key)
	c.queue.Add(key)
}

// requeue adds an object's key to the work queue for
// a retry if the rate limiter allows it.
func (c *Controller) requeue(key interface{}) {
	c.queue.AddRateLimited(key)
}

// run waits for items to process in the work queue.
func (c *Controller) run() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem handle the reconciliation
// of the Webhook when new item is added to the work queue.
// Always returns true unless the work queue was shutdown.
func (c *Controller) processNextWorkItem() bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	if err := c.reconcile(); err != nil {
		c.requeue(key)
		log.Infof("Couldn't reconcile Webhook %s: %v", c.config.GetName(), err)
		metrics.ReconcileErrors.Inc(metrics.WebhooksControllerName)
		return true
	}

	c.queue.Forget(key)
	log.Debugf("Webhook %s reconciled successfully", c.config.GetName())
	metrics.ReconcileSuccess.Inc(metrics.WebhooksControllerName)

	return true
}

// reconcile reconciles the current state of the Webhook with its desired state.
func (c *Controller) reconcile() error {
	secret, err := c.secretsLister.Secrets(c.config.GetSecretNs()).Get(c.config.GetSecretName())
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("Secret %s/%s was not found, aborting the reconciliation", c.config.GetSecretNs(), c.config.GetSecretName())
		}
		return err
	}

	webhook, err := c.webhooksLister.Get(c.config.GetName())
	if err != nil {
		if errors.IsNotFound(err) {
			log.Infof("Webhook %s was not found, creating it", c.config.GetName())
			return c.createWebhook(secret)
		}
		return err
	}

	log.Debugf("The Webhook %s was found, updating it", c.config.GetName())
	return c.updateWebhook(secret, webhook)
}

// createWebhook creates a new MutatingWebhookConfiguration object.
func (c *Controller) createWebhook(secret *corev1.Secret) error {
	webhook := &admiv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.GetName(),
		},
		Webhooks: c.newWebhooks(secret),
	}
	_, err := c.clientSet.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(webhook)
	if errors.IsAlreadyExists(err) {
		log.Infof("Webhook %s already exists", webhook.GetName())
		return nil
	}
	return err
}

// updateWebhook stores a new configuration in the MutatingWebhookConfiguration object.
func (c *Controller) updateWebhook(secret *corev1.Secret, webhook *admiv1beta1.MutatingWebhookConfiguration) error {
	webhook = webhook.DeepCopy()
	webhook.Webhooks = c.newWebhooks(secret)
	_, err := c.clientSet.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Update(webhook)
	return err
}

// newWebhooks generates MutatingWebhook objects from config templates with updated CABundle from Secret.
func (c *Controller) newWebhooks(secret *corev1.Secret) []admiv1beta1.MutatingWebhook {
	webhooks := []admiv1beta1.MutatingWebhook{}
	for _, tpl := range c.config.GetTemplates() {
		tpl.ClientConfig.CABundle = certificate.GetCABundle(secret.Data)
		webhooks = append(webhooks, tpl)
	}
	return webhooks
}
