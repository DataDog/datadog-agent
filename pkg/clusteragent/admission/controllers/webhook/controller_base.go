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
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	agentsidecar "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/agent_sidecar"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoscaling"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configWebhook "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/cwsinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/tagsfromlabels"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/validate/kubernetesadmissionevents"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Controller is an interface implemented by ControllerV1 and ControllerV1beta1.
type Controller interface {
	Run(stopCh <-chan struct{})
	EnabledWebhooks() []Webhook
}

// NewController returns the adequate implementation of the Controller interface.
func NewController(
	client kubernetes.Interface,
	secretInformer coreinformers.SecretInformer,
	validatingInformers admissionregistration.Interface,
	mutatingInformers admissionregistration.Interface,
	isLeaderFunc func() bool,
	leadershipStateNotif <-chan struct{},
	config Config,
	wmeta workloadmeta.Component,
	pa workload.PodPatcher,
	datadogConfig config.Component,
	demultiplexer demultiplexer.Component,
) Controller {
	if config.useAdmissionV1() {
		return NewControllerV1(client, secretInformer, validatingInformers.V1().ValidatingWebhookConfigurations(), mutatingInformers.V1().MutatingWebhookConfigurations(), isLeaderFunc, leadershipStateNotif, config, wmeta, pa, datadogConfig, demultiplexer)
	}
	return NewControllerV1beta1(client, secretInformer, validatingInformers.V1beta1().ValidatingWebhookConfigurations(), mutatingInformers.V1beta1().MutatingWebhookConfigurations(), isLeaderFunc, leadershipStateNotif, config, wmeta, pa, datadogConfig, demultiplexer)
}

// Webhook represents an admission webhook
type Webhook interface {
	// Name returns the name of the webhook
	Name() string
	// WebhookType Type returns the type of the webhook
	WebhookType() common.WebhookType
	// IsEnabled returns whether the webhook is enabled
	IsEnabled() bool
	// Endpoint returns the endpoint of the webhook
	Endpoint() string
	// Resources returns the kubernetes resources for which the webhook should
	// be invoked.
	// The key is the API group, and the value is a list of resources.
	Resources() map[string][]string
	// Operations returns the operations on the resources specified for which
	// the webhook should be invoked
	Operations() []admiv1.OperationType
	// LabelSelectors returns the label selectors that specify when the webhook
	// should be invoked
	LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector)
	// MatchConditions returns the Match Conditions used for fine-grained
	// request filtering
	MatchConditions() []admiv1.MatchCondition
	// WebhookFunc runs the logic of the webhook and returns the admission response
	WebhookFunc() admission.WebhookFunc
	// Timeout returns the timeout for the webhook
	Timeout() int32
}

// generateWebhooks returns the list of webhooks. The order of the webhooks returned
// is the order in which they will be executed. For now, the only restriction is that
// the agent sidecar webhook needs to go after the configWebhook one.
// The reason is that the volume mount for the APM socket added by the configWebhook webhook
// doesn't always work on Fargate (one of the envs where we use an agent sidecar), and
// the agent sidecar webhook needs to remove it.
func (c *controllerBase) generateWebhooks(wmeta workloadmeta.Component, pa workload.PodPatcher, datadogConfig config.Component, demultiplexer demultiplexer.Component) []Webhook {
	var webhooks []Webhook
	var validatingWebhooks []Webhook

	// Add Validating webhooks.
	if c.config.isValidationEnabled() {
		// Future validating webhooks can be added here.
		validatingWebhooks = []Webhook{
			kubernetesadmissionevents.NewWebhook(datadogConfig, demultiplexer, c.config.supportsMatchConditions()),
		}
		webhooks = append(webhooks, validatingWebhooks...)
	}

	// Skip mutating webhooks if the mutation feature is disabled.
	if !c.config.isMutationEnabled() {
		return webhooks
	}

	// Setup config webhook.
	configWebhook, err := generateConfigWebhook(wmeta, datadogConfig)
	if err != nil {
		log.Errorf("failed to register config webhook: %v", err)
	} else {
		webhooks = append(webhooks, configWebhook)
	}

	// Setup tags from labels webhook.
	tagsWebhook, err := generateTagsFromLabelsWebhook(wmeta, datadogConfig)
	if err != nil {
		log.Errorf("failed to register tags from labels webhook: %v", err)
	} else {
		webhooks = append(webhooks, tagsWebhook)
	}

	// Setup agents sidecar webhook.
	agentsWebhook := agentsidecar.NewWebhook(datadogConfig)
	webhooks = append(webhooks, agentsWebhook)

	// Setup autoscaling webhook.
	autoscalingWebhook := autoscaling.NewWebhook(pa, datadogConfig)
	webhooks = append(webhooks, autoscalingWebhook)

	// Setup APM Instrumentation webhook. APM Instrumentation webhook needs to be registered after the config webhook.
	apmWebhook, err := generateAutoInstrumentationWebhook(wmeta, datadogConfig)
	if err != nil {
		log.Errorf("failed to register APM Instrumentation webhook: %v", err)
	} else {
		webhooks = append(webhooks, apmWebhook)
	}

	isCWSInstrumentationEnabled := datadogConfig.GetBool("admission_controller.cws_instrumentation.enabled")
	if isCWSInstrumentationEnabled {
		cws, err := cwsinstrumentation.NewCWSInstrumentation(wmeta, datadogConfig)
		if err == nil {
			webhooks = append(webhooks, cws.WebhookForPods(), cws.WebhookForCommands())
		} else {
			log.Errorf("failed to register CWS Instrumentation webhook: %v", err)
		}
	}

	return webhooks
}

func generateConfigWebhook(wmeta workloadmeta.Component, datadogConfig config.Component) (*configWebhook.Webhook, error) {
	filter, err := configWebhook.NewFilter(datadogConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create config filter: %v", err)
	}
	mutatorCfg := configWebhook.NewMutatorConfig(datadogConfig)
	mutator := configWebhook.NewMutator(mutatorCfg, filter)
	return configWebhook.NewWebhook(wmeta, datadogConfig, mutator), nil
}

func generateTagsFromLabelsWebhook(wmeta workloadmeta.Component, datadogConfig config.Component) (*tagsfromlabels.Webhook, error) {
	filter, err := tagsfromlabels.NewFilter(datadogConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create tags from labels filter: %v", err)
	}
	mutator := tagsfromlabels.NewMutator(tagsfromlabels.NewMutatorConfig(datadogConfig), filter)
	return tagsfromlabels.NewWebhook(wmeta, datadogConfig, mutator), nil
}

func generateAutoInstrumentationWebhook(wmeta workloadmeta.Component, datadogConfig config.Component) (*autoinstrumentation.Webhook, error) {
	config, err := autoinstrumentation.NewConfig(datadogConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create auto instrumentation config: %v", err)
	}

	apm, err := autoinstrumentation.NewMutatorWithFilter(config, wmeta)
	if err != nil {
		return nil, fmt.Errorf("failed to create auto instrumentation namespace mutator: %v", err)
	}

	// For auto instrumentation, we need all the mutators to be applied for SSI to function. Specifically, we need
	// things like the Datadog socket to be mounted from the config webhook and the DD_ENV, DD_SERVICE, and DD_VERSION
	// env vars to be set from labels if they are available..
	mutator := mutatecommon.NewMutators(
		tagsfromlabels.NewMutator(tagsfromlabels.NewMutatorConfig(datadogConfig), apm),
		configWebhook.NewMutator(configWebhook.NewMutatorConfig(datadogConfig), apm),
		apm,
	)
	return autoinstrumentation.NewWebhook(config, wmeta, mutator)
}

// controllerBase acts as a base class for ControllerV1 and ControllerV1beta1.
// It contains the shared fields and provides shared methods.
// For the nolint:structcheck see https://github.com/golangci/golangci-lint/issues/537
type controllerBase struct {
	clientSet                kubernetes.Interface //nolint:structcheck
	config                   Config
	secretsLister            corelisters.SecretLister
	secretsSynced            cache.InformerSynced //nolint:structcheck
	validatingWebhooksSynced cache.InformerSynced //nolint:structcheck
	mutatingWebhooksSynced   cache.InformerSynced //nolint:structcheck
	queue                    workqueue.TypedRateLimitingInterface[string]
	isLeaderFunc             func() bool
	leadershipStateNotif     <-chan struct{}
	webhooks                 []Webhook
}

// EnabledWebhooks returns the list of enabled webhooks.
func (c *controllerBase) EnabledWebhooks() []Webhook {
	var res []Webhook

	for _, webhook := range c.webhooks {
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
		case <-c.leadershipStateNotif:
			if c.isLeaderFunc() {
				log.Infof("Got a leader notification, enqueuing a reconciliation for %q", c.config.getWebhookName())
				c.triggerReconciliation()
			}
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
		c.queue.Add("")
		return
	}
	log.Debugf("Adding object with key %s to the queue", key)
	c.queue.Add(key)
}

// requeue adds an object's key to the work queue for
// a retry if the rate limiter allows it.
func (c *controllerBase) requeue(key string) {
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
		log.Errorf("Couldn't reconcile webhook %s: %v", c.config.getWebhookName(), err)
		metrics.ReconcileErrors.Inc(metrics.WebhooksControllerName)
		return true
	}

	c.queue.Forget(key)
	log.Debugf("Webhook %s reconciled successfully", c.config.getWebhookName())
	metrics.ReconcileSuccess.Inc(metrics.WebhooksControllerName)

	return true
}
