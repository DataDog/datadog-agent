// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package apiserver

const (
	autoscalerNowHandleMsgEvent = "Autoscaler is now handled by the Cluster-Agent"
)

// controllerName represents the cluster agent controller names
type controllerName string

const (
	metadataController    controllerName = "metadata"
	autoscalersController controllerName = "autoscalers"
	servicesController    controllerName = "services"
	endpointsController   controllerName = "endpoints"
)

// InformerName represents the kubernetes informer names
type InformerName string

const (
	endpointsInformer InformerName = "v1/endpoints"
	// SecretsInformer holds the name of the informer
	SecretsInformer InformerName = "v1/secrets"
	// WebhooksInformer holds the name of the informer
	WebhooksInformer InformerName = "admissionregistration.k8s.io/v1/mutatingwebhookconfigurations"
	// ServicesInformer holds the name of the informer
	ServicesInformer InformerName = "v1/services"
)
