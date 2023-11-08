// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/test"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/DataDog/test-infra-definitions/components"
	"github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/kindvm"

	clientGo "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type KubeCluster kubernetes.EKubernetesCluster

var _ components.Importable = &KubeCluster{}

func (kc *KubeCluster) GetClient() (*clientGo.Clientset, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kc.KubeConfig))
	if err != nil {
		return nil, err
	}

	return clientGo.NewForConfigOrDie(config), nil
}

type myEnv struct {
	KindCluster *KubeCluster `import:"dd-KubernetesCluster-kind"`
}

type mySuite struct {
	test.BaseSuite[myEnv]
}

func TestMySuite(t *testing.T) {
	test.Run(t, &mySuite{}, test.WithPulumiProvisioner(kindvm.Run, runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "false"},
		"ddtestworkload:deploy": auto.ConfigValue{Value: "false"},
	}), test.WithDevMode())
}

func (s *mySuite) TestRun() {
	fmt.Printf("%+v", s.Env().KindCluster)
	client, err := s.Env().KindCluster.GetClient()
	s.Assert().NoError(err)
	s.Assert().NotNil(client)
}
