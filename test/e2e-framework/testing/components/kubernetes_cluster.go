// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	utilversion "k8s.io/apimachinery/pkg/util/version"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const kubeClientTimeout = 60 * time.Second

// KubernetesCluster represents a Kubernetes cluster
type KubernetesCluster struct {
	kubernetes.ClusterOutput

	KubernetesClient *client.KubernetesClient
}

var _ common.Initializable = &KubernetesCluster{}

// Init is called by e2e test Suite after the component is provisioned.
func (kc *KubernetesCluster) Init(common.Context) error {
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kc.KubeConfig))
	if err != nil {
		return err
	}

	// Always set a timeout for the client
	config.Timeout = kubeClientTimeout

	// Create client
	kc.KubernetesClient, err = client.NewKubernetesClient(config)
	if err != nil {
		return err
	}

	return nil
}

// Client returns the Kubernetes client
func (kc *KubernetesCluster) Client() kubeClient.Interface {
	return kc.KubernetesClient.K8sClient
}

// ServerVersion returns the version of the Kubernetes API server, as reported by the cluster itself.
// It works with every provider, including those for which no Kubernetes version is passed explicitly
// at provisioning time (e.g. EKS).
func (kc *KubernetesCluster) ServerVersion() (*utilversion.Version, error) {
	info, err := kc.Client().Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to request the Kubernetes server version: %w", err)
	}

	version, err := utilversion.ParseGeneric(info.GitVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the Kubernetes server version %q: %w", info.GitVersion, err)
	}

	return version, nil
}
