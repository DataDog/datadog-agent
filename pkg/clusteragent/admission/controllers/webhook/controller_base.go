// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"fmt"

	admiv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers/admissionregistration"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	agentsidecar "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/agent_sidecar"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/cwsinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/tagsfromlabels"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Controller is an interface implemented by ControllerV1 and ControllerV1beta1.
type Controller interface {
	Run(stopCh <-chan struct{})
	EnabledWebhooks() []MutatingWebhook
}

// NewController returns the adequate implementation of the Controller interface.
func NewController(client kubernetes.Interface, secretInformer coreinformers.SecretInformer, admissionInterface admissionregistration.Interface, isLeaderFunc func() bool, isLeaderNotif <-chan struct{}, config Config, wmeta workloadmeta.Component) Controller {
	if config.useAdmissionV1() {
		return NewControllerV1(client, secretInformer, admissionInterface.V1().MutatingWebhookConfigurations(), isLeaderFunc, isLeaderNotif, config, wmeta)
	}

	return NewControllerV1beta1(client, secretInformer, admissionInterface.V1beta1().MutatingWebhookConfigurations(), isLeaderFunc, isLeaderNotif, config, wmeta)
}

// MutatingWebhook represents a mutating webhook
type MutatingWebhook interface {
	// Name returns the name of the webhook
	Name() string
	// IsEnabled returns whether the webhook is enabled
	IsEnabled() bool
	// Endpoint returns the endpoint of the webhook
	Endpoint() string
	// Resources returns the kubernetes resources for which the webhook should
	// be invoked
	Resources() []string
	// Operations returns the operations on the resources specified for which
	// the webhook should be invoked
	Operations() []admiv1.OperationType
	// LabelSelectors returns the label selectors that specify when the webhook
	// should be invoked
	LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector)
	// MutateFunc returns the function that mutates the resources
	MutateFunc() admission.WebhookFunc
}

// mutatingWebhooks returns the list of mutating webhooks. Notice that the order
// of the webhooks returned is the order in which they will be executed. For
// now, the only restriction is that the agent sidecar webhook needs to go after
// the config one. The reason is that the volume mount for the APM socket added
// by the config webhook doesn't always work on Fargate (one of the envs where
// we use an agent sidecar), and the agent sidecar webhook needs to remove it.
func mutatingWebhooks(wmeta workloadmeta.Component) []MutatingWebhook {
	webhooks := []MutatingWebhook{
		config.NewWebhook(wmeta),
		tagsfromlabels.NewWebhook(wmeta),
		agentsidecar.NewWebhook(),
	}

	apm, err := autoinstrumentation.GetWebhook(wmeta)
	if err == nil {
		webhooks = append(webhooks, apm)
	} else {
		log.Errorf("failed to register APM Instrumentation webhook: %v", err)
	}

	cws, err := cwsinstrumentation.NewCWSInstrumentation()
	if err == nil {
		webhooks = append(webhooks, cws.WebhookForPods(), cws.WebhookForCommands())
	} else {
		log.Errorf("failed to register CWS Instrumentation webhook: %v", err)
	}

	return webhooks
}

// controllerBase acts as a base class for ControllerV1 and ControllerV1beta1.
// It contains the shared fields and provides shared methods.
// For the nolint:structcheck see https://github.com/golangci/golangci-lint/issues/537
type controllerBase struct {
	clientSet        kubernetes.Interface //nolint:structcheck
	config           Config
	secretsLister    corelisters.SecretLister
	secretsSynced    cache.InformerSynced //nolint:structcheck
	webhooksSynced   cache.InformerSynced //nolint:structcheck
	queue            workqueue.RateLimitingInterface
	isLeaderFunc     func() bool
	isLeaderNotif    <-chan struct{}
	mutatingWebhooks []MutatingWebhook
}

// EnabledWebhooks returns the list of enabled webhooks.
func (c *controllerBase) EnabledWebhooks() []MutatingWebhook {
	var res []MutatingWebhook

	for _, webhook := range c.mutatingWebhooks {
		if webhook.IsEnabled() {
			res = append(res, webhook)
		}
	}

	return res
}

// enqueueOnLeaderNotif watches leader notifications and triggers a
// reconciliation in case the current process becomes leader.
// This ensures that the latest configuration of the leader
// is applied to the webhook object. Typically, during a rolling update.
func (c *controllerBase) enqueueOnLeaderNotif(stop <-chan struct{}) {
	for {
		select {
		case <-c.isLeaderNotif:
			log.Infof("Got a leader notification, enqueuing a reconciliation for %q", c.config.getWebhookName())
			c.triggerReconciliation()
		case <-stop:
			return
		}
	}
}

// triggerReconciliation forces a reconciliation loop by enqueuing the webhook object name.
func (c *controllerBase) triggerReconciliation() {
	c.queue.Add(c.config.getWebhookName())
}

func (c *controllerBase) getSecret() (*corev1.Secret, error) {
	secret, err := c.secretsLister.Secrets(c.config.getSecretNs()).Get(c.config.getSecretName())
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("secret %s/%s was not found, aborting the reconciliation", c.config.getSecretNs(), c.config.getSecretName())
		}
	}

	return secret, err
}

// handleSecret enqueues the targeted Secret object when an event occurs.
// It can be a callback function for deletion and addition events.
func (c *controllerBase) handleSecret(obj interface{}) {
	if !c.isLeaderFunc() {
		return
	}
	if object, ok := obj.(metav1.Object); ok {
		if object.GetNamespace() == c.config.getSecretNs() && object.GetName() == c.config.getSecretName() {
			c.enqueue(object)
		}
	}
}

// handleSecretUpdate handles the new Secret reported in update events.
// It can be a callback function for update events.
func (c *controllerBase) handleSecretUpdate(oldObj, newObj interface{}) {
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
func (c *controllerBase) handleWebhook(obj interface{}) {
	if !c.isLeaderFunc() {
		return
	}
	if object, ok := obj.(metav1.Object); ok {
		if object.GetName() == c.config.getWebhookName() {
			c.enqueue(object)
		}
	}
}

// enqueue adds a given object to the work queue.
func (c *controllerBase) enqueue(obj interface{}) {
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
func (c *controllerBase) requeue(key interface{}) {
	c.queue.AddRateLimited(key)
}

// processNextWorkItem handle the reconciliation
// of the Webhook when new item is added to the work queue.
// Always returns true unless the work queue was shutdown.
func (c *controllerBase) processNextWorkItem(reconcile func() error) bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	if err := reconcile(); err != nil {
		c.requeue(key)
		log.Infof("Couldn't reconcile Webhook %s: %v", c.config.getWebhookName(), err)
		metrics.ReconcileErrors.Inc(metrics.WebhooksControllerName)
		return true
	}

	c.queue.Forget(key)
	log.Debugf("Webhook %s reconciled successfully", c.config.getWebhookName())
	metrics.ReconcileSuccess.Inc(metrics.WebhooksControllerName)

	return true
}
