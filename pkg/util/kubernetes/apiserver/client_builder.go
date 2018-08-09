// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientBuilder allows you to get clients and configs for the apiserver.
type ClientBuilder struct {
	// ClientConfig is a default config to clone and use as the basis for each controller client.
	ClientConfig *rest.Config
}

func NewClientBuilder() (*ClientBuilder, error) {
	var clientConfig *rest.Config
	var err error
	cfgPath := config.Datadog.GetString("kubernetes_kubeconfig_path")
	if cfgPath == "" {
		clientConfig, err = rest.InClusterConfig()
		if err != nil {
			log.Debugf("Can't create a config for the official client from the service account's token: %s", err)
			return nil, err
		}
	} else {
		// use the current context in kubeconfig
		clientConfig, err = clientcmd.BuildConfigFromFlags("", cfgPath)
		if err != nil {
			log.Debugf("Can't create a config for the official client from the configured path to the kubeconfig: %s, %s", cfgPath, err)
			return nil, err
		}
	}
	return &ClientBuilder{
		ClientConfig: clientConfig,
	}, nil
}

func (b *ClientBuilder) Config(timeout time.Duration) (*rest.Config, error) {
	clientConfig := *b.ClientConfig // shallow copy
	clientConfig.Timeout = timeout
	return &clientConfig, nil
}

func (b *ClientBuilder) Client(timeout time.Duration) (clientset.Interface, error) {
	clientConfig, err := b.Config(timeout)
	if err != nil {
		log.Debugf("Could not create the clientset: %v", err)
		return nil, err
	}
	return clientset.NewForConfig(clientConfig)
}
