// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"testing"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/components/kubernetes/cilium"
	"github.com/DataDog/test-infra-definitions/components/kubernetes/istio"
)

type ciliumLBConntrackerTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]

	component      *cilium.HelmComponent
	httpBinService *corev1.Service
}

func TestCiliumLBConntracker(t *testing.T) {
	suite := &ciliumLBConntrackerTestSuite{}
	ciliumInstall := func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		var err error
		suite.component, err = cilium.NewHelmInstallation(e, pulumi.Provider(kubeProvider))
		return &kubeComp.Workload{}, err
	}

	httpBinServiceInstall := func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		var err error
		suite.httpBinService, err = istio.NewHttpbinServiceInstallation(e, pulumi.Provider(kubeProvider))
		return &kubeComp.Workload{}, err
	}

	e2e.Run(t, suite, e2e.WithProvisioner(
		awskubernetes.KindProvisioner(
			awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(systemProbeConfigWithCiliumLB)),
			awskubernetes.WithWorkloadApp(ciliumInstall),
			awskubernetes.WithWorkloadApp(httpBinServiceInstall),
		),
	))
}

func (suite *ciliumLBConntrackerTestSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()

	suite.Require().NotNil(suite.component)
	suite.Require().NotNil(suite.httpBinService)
}

func (suite *ciliumLBConntrackerTestSuite) TestFoo() {}
