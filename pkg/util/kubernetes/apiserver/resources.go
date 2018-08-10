// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver,!dca

package apiserver

import (
	"github.com/DataDog/datadog-agent/pkg/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// checkResources checks that we can query resources from the Kubernetes apiserver required
// by the Datadog Agent.
func (c *APIClient) checkResources() error {
	resources := make(map[string]ListFunc)
	resources["events"] = func(options metav1.ListOptions) (runtime.Object, error) {
		return c.Cl.CoreV1().Events("").List(options)
	}
	if config.Datadog.GetBool("kubernetes_collect_metadata_tags") {
		resources["services"] = func(options metav1.ListOptions) (runtime.Object, error) {
			return c.Cl.CoreV1().Services("").List(options)
		}
		resources["pods"] = func(options metav1.ListOptions) (runtime.Object, error) {
			return c.Cl.CoreV1().Pods("").List(options)
		}
		resources["nodes"] = func(options metav1.ListOptions) (runtime.Object, error) {
			return c.Cl.CoreV1().Nodes().List(options)
		}
	}
	return c.maybeListResources(resources)
}
