// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"time"

	log "github.com/cihub/seelog"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const clientTimeout = 2 * time.Second

// GetKubeconfig returns a Kubeconfig needed for a Kubernetes client.
func GetKubeconfig() (*rest.Config, error) {
	cfgPath := config.Datadog.GetString("kubernetes_kubeconfig_path")
	if cfgPath == "" {
		Kubeconfig, err := rest.InClusterConfig()
		if err != nil {
			log.Debug("Can't create a config for the official client from the service account's token: %s", err)
			return nil, err
		}
		return Kubeconfig, nil
	}

	// use the current context in kubeconfig
	Kubeconfig, err := clientcmd.BuildConfigFromFlags("", cfgPath)
	if err != nil {
		log.Debug("Can't create a config for the official client from the configured path to the kubeconfig: %s, ", cfgPath, err)
		return nil, err
	}
	return Kubeconfig, nil
}

// GetCoreV1Client returns an official Kubernetes core v1 client
func GetCoreV1Client() (*corev1.CoreV1Client, error) {
	Kubeconfig, err := GetKubeconfig()
	if err != nil {
		return nil, err
	}
	Kubeconfig.Timeout = clientTimeout
	return corev1.NewForConfig(Kubeconfig)
}
