// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"fmt"
	"os"
	"path"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func KubeClient(c interface{}) (*kubernetes.Clientset, error) {
	configs, overrides, err := credentialsToConfigs(c)
	if err != nil {
		return nil, err
	}
	kconf, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(configs, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(kconf)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func DynamicKubeClient(c interface{}) (*dynamic.DynamicClient, error) {
	configs, overrides, err := credentialsToConfigs(c)
	if err != nil {
		return nil, err
	}
	kubeConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(configs, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	client, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func credentialsToConfigs(c interface{}) (*clientcmd.ClientConfigLoadingRules, *clientcmd.ConfigOverrides, error) {
	if credentials, ok := parseAsKubeConfigCredentials(c); ok {
		// Retrieve the kubeconfig file from the default location in the user's home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, err
		}
		configLoadingRules := &clientcmd.ClientConfigLoadingRules{
			ExplicitPath: path.Join(homeDir, ".kube/config"),
		}
		configOverrides := &clientcmd.ConfigOverrides{CurrentContext: credentials.Context}
		return configLoadingRules, configOverrides, nil
	}
	if ok := parseAsServiceAccountCredentials(c); ok {
		return &clientcmd.ClientConfigLoadingRules{}, &clientcmd.ConfigOverrides{}, nil
	}
	return nil, nil, fmt.Errorf("unsupported credential type")
}
