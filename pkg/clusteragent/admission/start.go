// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package admission implements the admission controller managed by the Cluster Agent.
package admission

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/controllers/secret"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/controllers/webhook"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// ControllerContext holds necessary context for the admission controllers
type ControllerContext struct {
	IsLeaderFunc        func() bool
	LeaderSubscribeFunc func() <-chan struct{}
	SecretInformers     informers.SharedInformerFactory
	ValidatingInformers informers.SharedInformerFactory
	MutatingInformers   informers.SharedInformerFactory
	Client              kubernetes.Interface
	StopCh              chan struct{}
	ValidatingStopCh    chan struct{}
	Demultiplexer       demultiplexer.Component
}

// StartControllers starts the secret and webhook controllers
func StartControllers(ctx ControllerContext, wmeta workloadmeta.Component, pa workload.PodPatcher, datadogConfig config.Component) ([]webhook.Webhook, error) {
	var webhooks []webhook.Webhook

	if !datadogConfig.GetBool("admission_controller.enabled") {
		log.Info("Admission controller is disabled")
		return webhooks, nil
	}

	certConfig := secret.NewCertConfig(
		datadogConfig.GetDuration("admission_controller.certificate.expiration_threshold")*time.Hour,
		datadogConfig.GetDuration("admission_controller.certificate.validity_bound")*time.Hour)
	secretConfig := secret.NewConfig(
		common.GetResourcesNamespace(),
		datadogConfig.GetString("admission_controller.certificate.secret_name"),
		datadogConfig.GetString("admission_controller.service_name"),
		certConfig)
	secretController := secret.NewController(
		ctx.Client,
		ctx.SecretInformers.Core().V1().Secrets(),
		ctx.IsLeaderFunc,
		ctx.LeaderSubscribeFunc(),
		secretConfig,
	)

	nsSelectorEnabled, err := useNamespaceSelector(ctx.Client.Discovery())
	if err != nil {
		return webhooks, err
	}

	matchConditionsSupported, err := supportsMatchConditions(ctx.Client.Discovery())
	if err != nil {
		return webhooks, err
	}

	v1Enabled, err := UseAdmissionV1(ctx.Client.Discovery())
	if err != nil {
		return webhooks, err
	}

	webhookConfig := webhook.NewConfig(v1Enabled, nsSelectorEnabled, matchConditionsSupported)
	webhookController := webhook.NewController(
		ctx.Client,
		ctx.SecretInformers.Core().V1().Secrets(),
		ctx.ValidatingInformers.Admissionregistration(),
		ctx.MutatingInformers.Admissionregistration(),
		ctx.IsLeaderFunc,
		ctx.LeaderSubscribeFunc(),
		webhookConfig,
		wmeta,
		pa,
		datadogConfig,
		ctx.Demultiplexer,
	)

	go secretController.Run(ctx.StopCh)
	go webhookController.Run(ctx.StopCh)

	ctx.SecretInformers.Start(ctx.StopCh)
	ctx.ValidatingInformers.Start(ctx.ValidatingStopCh)
	ctx.MutatingInformers.Start(ctx.StopCh)

	informers := map[apiserver.InformerName]cache.SharedInformer{
		apiserver.SecretsInformer: ctx.SecretInformers.Core().V1().Secrets().Informer(),
	}

	if v1Enabled {
		informers[apiserver.ValidatingWebhooksInformer] = ctx.ValidatingInformers.Admissionregistration().V1().ValidatingWebhookConfigurations().Informer()
		informers[apiserver.MutatingWebhooksInformer] = ctx.MutatingInformers.Admissionregistration().V1().MutatingWebhookConfigurations().Informer()
		getValidatingWebhookStatus = getValidatingWebhookStatusV1
		getMutatingWebhookStatus = getMutatingWebhookStatusV1
	} else {
		informers[apiserver.ValidatingWebhooksInformer] = ctx.ValidatingInformers.Admissionregistration().V1beta1().ValidatingWebhookConfigurations().Informer()
		informers[apiserver.MutatingWebhooksInformer] = ctx.MutatingInformers.Admissionregistration().V1beta1().MutatingWebhookConfigurations().Informer()
		getValidatingWebhookStatus = getValidatingWebhookStatusV1beta1
		getMutatingWebhookStatus = getMutatingWebhookStatusV1beta1
	}

	webhooks = append(webhooks, webhookController.EnabledWebhooks()...)

	return webhooks, apiserver.SyncInformers(informers, 0)
}
