// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	endpointsInformer InformerName = "endpoints"
	servicesInformer  InformerName = "services"
	// SecretsInformer holds the name of the informer
	SecretsInformer InformerName = "secrets"
	// WebhooksInformer holds the name of the informer
	WebhooksInformer InformerName = "webhooks"
	// PodsInformer holds the name of the informer
	PodsInformer InformerName = "pods"
	// DeploysInformer holds the name of the informer
	DeploysInformer InformerName = "deploys"
	// ReplicasetsInformer holds the name of the informer
	ReplicasetsInformer InformerName = "replicasets"
)
