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

func (c *APIClient) checkResources(namespace string) error {
	resources := make(map[string]ListFunc)
	resources["events"] = func(options metav1.ListOptions) (runtime.Object, error) {
		return c.Cl.CoreV1().Events(namespace).List(options)
	}
	if config.Datadog.GetBool("kubernetes_collect_metadata_tags") {
		resources["services"] = func(options metav1.ListOptions) (runtime.Object, error) {
			return c.Cl.CoreV1().Services(namespace).List(options)
		}
		resources["pods"] = func(options metav1.ListOptions) (runtime.Object, error) {
			return c.Cl.CoreV1().Pods(namespace).List(options)
		}
		resources["nodes"] = func(options metav1.ListOptions) (runtime.Object, error) {
			return c.Cl.CoreV1().Nodes().List(options)
		}
	}
	return c.maybeListResources(resources)
}
