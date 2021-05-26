// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	// SecretsInformer holds the name of the informer
	SecretsInformer InformerName = "secrets"
	// WebhooksInformer holds the name of the informer
	WebhooksInformer InformerName = "webhooks"
	// PodsInformer holds the name of the informer
	PodsInformer InformerName = "pods"
	// DeploysInformer holds the name of the informer
	DeploysInformer InformerName = "deploys"
	// ReplicaSetsInformer holds the name of the informer
	ReplicaSetsInformer InformerName = "replicaSets"
	// ServicesInformer holds the name of the informer
	ServicesInformer InformerName = "services"
	// NodesInformer holds the name of the informer
	NodesInformer InformerName = "nodes"
	// JobsInformer holds the name of the informer
	JobsInformer InformerName = "jobs"
	// CronJobsInformer holds the name of the informer
	CronJobsInformer InformerName = "cronJobs"
	// DaemonSetsInformer holds the name of the informer
	DaemonSetsInformer InformerName = "daemonSets"
)
