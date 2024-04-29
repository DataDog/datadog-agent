// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/test-infra-definitions/components/kubernetes"

	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const kubeClientTimeout = 60 * time.Second

// KubernetesCluster represents a Kubernetes cluster
type KubernetesCluster struct {
	kubernetes.ClusterOutput

	client kubeClient.Interface
}

var _ e2e.Initializable = &KubernetesCluster{}

// Init is called by e2e test Suite after the component is provisioned.
func (kc *KubernetesCluster) Init(e2e.Context) error {
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kc.KubeConfig))
	if err != nil {
		return err
	}

	// Always set a timeout for the client
	config.Timeout = kubeClientTimeout

	// Create client
	kc.client, err = kubeClient.NewForConfig(config)
	if err != nil {
		return err
	}

	return nil
}

// Client returns the Kubernetes client
func (kc *KubernetesCluster) Client() kubeClient.Interface {
	return kc.client
}
