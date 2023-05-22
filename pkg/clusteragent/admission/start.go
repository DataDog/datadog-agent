// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/controllers/secret"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/controllers/webhook"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// ControllerContext holds necessary context for the admission controllers
type ControllerContext struct {
	IsLeaderFunc        func() bool
	LeaderSubscribeFunc func() <-chan struct{}
	SecretInformers     informers.SharedInformerFactory
	WebhookInformers    informers.SharedInformerFactory
	Client              kubernetes.Interface
	DiscoveryClient     discovery.DiscoveryInterface
	StopCh              chan struct{}
}

// StartControllers starts the secret and webhook controllers
func StartControllers(ctx ControllerContext) error {
	if !config.Datadog.GetBool("admission_controller.enabled") {
		log.Info("Admission controller is disabled")
		return nil
	}

	certConfig := secret.NewCertConfig(
		config.Datadog.GetDuration("admission_controller.certificate.expiration_threshold")*time.Hour,
		config.Datadog.GetDuration("admission_controller.certificate.validity_bound")*time.Hour)
	secretConfig := secret.NewConfig(
		common.GetResourcesNamespace(),
		config.Datadog.GetString("admission_controller.certificate.secret_name"),
		config.Datadog.GetString("admission_controller.service_name"),
		certConfig)
	secretController := secret.NewController(
		ctx.Client,
		ctx.SecretInformers.Core().V1().Secrets(),
		ctx.IsLeaderFunc,
		ctx.LeaderSubscribeFunc(),
		secretConfig,
	)

	nsSelectorEnabled, err := useNamespaceSelector(ctx.DiscoveryClient)
	if err != nil {
		return err
	}

	v1Enabled, err := useAdmissionV1(ctx)
	if err != nil {
		return err
	}

	webhookConfig := webhook.NewConfig(v1Enabled, nsSelectorEnabled)
	webhookController := webhook.NewController(
		ctx.Client,
		ctx.SecretInformers.Core().V1().Secrets(),
		ctx.WebhookInformers.Admissionregistration(),
		ctx.IsLeaderFunc,
		ctx.LeaderSubscribeFunc(),
		webhookConfig,
	)

	go secretController.Run(ctx.StopCh)
	go webhookController.Run(ctx.StopCh)

	ctx.SecretInformers.Start(ctx.StopCh)
	ctx.WebhookInformers.Start(ctx.StopCh)

	informers := map[apiserver.InformerName]cache.SharedInformer{
		apiserver.SecretsInformer: ctx.SecretInformers.Core().V1().Secrets().Informer(),
	}

	if v1Enabled {
		informers[apiserver.WebhooksInformer] = ctx.WebhookInformers.Admissionregistration().V1().MutatingWebhookConfigurations().Informer()
		getWebhookStatus = getWebhookStatusV1
	} else {
		informers[apiserver.WebhooksInformer] = ctx.WebhookInformers.Admissionregistration().V1beta1().MutatingWebhookConfigurations().Informer()
		getWebhookStatus = getWebhookStatusV1beta1
	}

	return apiserver.SyncInformers(informers, 0)
}
