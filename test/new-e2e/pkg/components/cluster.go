package components

import (
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/test-infra-definitions/components/kubernetes"

	clientGo "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const kubeClientTimeout = 60 * time.Second

type KubernetesCluster struct {
	kubernetes.ClusterOutput

	client clientGo.Interface
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

	// Create cliens
	kc.client, err = clientGo.NewForConfig(config)
	if err != nil {
		return err
	}

	return nil
}

func (kc *KubernetesCluster) Client() clientGo.Interface {
	return kc.client
}
