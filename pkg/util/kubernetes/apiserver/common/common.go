// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package common

import (
	"io/ioutil"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetResourcesNamespace is used to fetch the namespace of the resources used by the Kubernetes check (e.g. Leader Election, Event collection).
func GetResourcesNamespace() string {
	namespace := config.Datadog.GetString("kube_resources_namespace")
	if namespace != "" {
		return namespace
	}
	log.Debugf("No configured namespace for the resource, fetching from the current context")
	return GetMyNamespace()
}

// GetMyNamespace returns the namespace our pod is running in
func GetMyNamespace() string {
	namespacePath := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	val, e := ioutil.ReadFile(namespacePath)
	if e == nil && val != nil {
		return string(val)
	}
	log.Warnf("There was an error fetching the namespace from the context, using default")
	return "default"
}
