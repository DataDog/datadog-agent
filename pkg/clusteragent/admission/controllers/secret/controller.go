// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package secret

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controller is responsible for creating and refreshing the Secret object
// that contains the certificate of the Admission Webhook.
type Controller struct {
	clientSet     kubernetes.Interface
	secretsLister corelisters.SecretLister
	secretsSynced cache.InformerSynced
	config        Config
	hosts         []string
	queue         workqueue.RateLimitingInterface
	isLeaderFunc  func() bool
}

// NewController returns a new Secret Controller.
func NewController(client kubernetes.Interface, secretInformer coreinformers.SecretInformer, isLeaderFunc func() bool, config Config) *Controller {
	controller := &Controller{
		clientSet:     client,
		config:        config,
		secretsLister: secretInformer.Lister(),
		secretsSynced: secretInformer.Informer().HasSynced,
		hosts:         generateHosts(config.GetNs(), config.GetSvc()),
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "secrets"),
		isLeaderFunc:  isLeaderFunc,
	}
	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.handleObject,
		UpdateFunc: controller.handleUpdate,
		DeleteFunc: controller.handleObject,
	})
	return controller
}

// Run starts the controller to process Secret
// events after sync'ing the informer's cache.
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer c.queue.ShutDown()

	log.Infof("Starting secrets controller for %s/%s", c.config.GetNs(), c.config.GetName())
	defer log.Infof("Stopping secrets controller for %s/%s", c.config.GetNs(), c.config.GetName())

	if !cache.WaitForCacheSync(stopCh, c.secretsSynced) {
		return
	}

	go wait.Until(c.run, time.Second, stopCh)

	// Trigger a reconciliation to create the Secret if it doesn't exist
	c.queue.Add(fmt.Sprintf("%s/%s", c.config.GetNs(), c.config.GetName()))

	<-stopCh
}

// handleObject enqueues the targeted Secret object when an event occurs.
// It can be a callback function for deletion and addition events.
func (c *Controller) handleObject(obj interface{}) {
	if !c.isLeaderFunc() {
		return
	}
	if object, ok := obj.(metav1.Object); ok {
		if object.GetNamespace() == c.config.GetNs() && object.GetName() == c.config.GetName() {
			c.enqueue(object)
		}
	}
}

// handleUpdate handles the new object reported in update events.
// It can be a callback function for update events.
func (c *Controller) handleUpdate(oldObj, newObj interface{}) {
	if !c.isLeaderFunc() {
		return
	}
	c.handleObject(newObj)
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
// of the Secret when new item is added to the work queue.
// Always returns true unless the work queue was shutdown.
func (c *Controller) processNextWorkItem() bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	if err := c.reconcile(); err != nil {
		c.requeue(key)
		log.Errorf("Couldn't reconcile Secret %s/%s: %v", c.config.GetNs(), c.config.GetName(), err)
		metrics.ReconcileErrors.Inc(metrics.SecretControllerName)
		return true
	}

	c.queue.Forget(key)
	log.Debugf("Secret %s/%s reconciled successfully", c.config.GetNs(), c.config.GetName())
	metrics.ReconcileSuccess.Inc(metrics.SecretControllerName)

	return true
}

// reconcile reconciles the current state of the Secret with its desired state.
func (c *Controller) reconcile() error {
	secret, err := c.secretsLister.Secrets(c.config.GetNs()).Get(c.config.GetName())
	if err != nil {
		if errors.IsNotFound(err) {
			// Create the Secret if it doesn't exist
			log.Debugf("Secret %s/%s was not found, creating it", c.config.GetNs(), c.config.GetName())
			return c.createSecret()
		}
		return err
	}

	// Check certificate and refresh it if needed
	durationBeforeExpiration, err := certificate.GetDurationBeforeExpiration(secret.Data)
	if err != nil {
		return err
	}

	metrics.CertificateDuration.Set(durationBeforeExpiration.Hours())
	if durationBeforeExpiration < c.config.GetCertExpiration() {
		log.Debugf("The certificate is expiring soon (%v), refreshing it", durationBeforeExpiration)
		return c.updateSecret(secret)
	}

	log.Debugf("The certificate is not expiring soon (%v), doing nothing", durationBeforeExpiration)
	return nil
}

// createSecret creates a new Secret object with a new certificate
func (c *Controller) createSecret() error {
	data, err := certificate.GenerateSecretData(notBefore(), c.notAfter(), c.hosts)
	if err != nil {
		return fmt.Errorf("failed to generate the Secret data: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.config.GetNs(),
			Name:      c.config.GetName(),
		},
		Data: data,
	}

	_, err = c.clientSet.CoreV1().Secrets(c.config.GetNs()).Create(secret)
	return err
}

// updateSecret stores a new certificate in the Secret object
func (c *Controller) updateSecret(secret *corev1.Secret) error {
	data, err := certificate.GenerateSecretData(notBefore(), c.notAfter(), c.hosts)
	if err != nil {
		return fmt.Errorf("failed to generate the Secret data: %v", err)
	}

	secret = secret.DeepCopy()
	secret.Data = data
	_, err = c.clientSet.CoreV1().Secrets(c.config.GetNs()).Update(secret)
	return err
}

// notAfter defines the validity bounds when creating a new certificate
func (c *Controller) notAfter() time.Time {
	return time.Now().Add(c.config.GetCertValidityBound())
}

// notBefore defines the validity bounds when creating a new certificate
func notBefore() time.Time {
	return time.Now().Add(-5 * time.Minute)
}

// generateHosts returns the hosts used as DNS
// names for the certificate creation.
func generateHosts(ns, svc string) []string {
	return []string{
		svc,
		svc + "." + ns,
		svc + "." + ns + ".svc",
		svc + "." + ns + ".svc.cluster.local",
	}
}
