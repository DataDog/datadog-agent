// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllers

import "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

type controllerName string

const (
	metadataControllerName    controllerName = "metadata"
	autoscalersControllerName controllerName = "autoscalers"
	servicesControllerName    controllerName = "services"
	endpointsControllerName   controllerName = "endpoints"
)

const (
	endpointsInformer apiserver.InformerName = "v1/endpoints"
	servicesInformer  apiserver.InformerName = "v1/services"
)
