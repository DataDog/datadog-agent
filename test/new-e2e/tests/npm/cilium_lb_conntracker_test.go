// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/test-infra-definitions/common/config"
	npmtools "github.com/DataDog/test-infra-definitions/components/datadog/apps/npm-tools"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/components/kubernetes/cilium"
	"github.com/DataDog/test-infra-definitions/components/kubernetes/istio"
)

type ciliumLBConntrackerTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]

	httpBinService *corev1.Service
}

func TestCiliumLBConntracker(t *testing.T) {
	// TODO: find a way to update this list dynamically
	versionsToTest := []string{"1.15.17", "1.16.10", "1.17.4"}
	for _, v := range versionsToTest {
		t.Run(fmt.Sprintf("version %s", v), func(_t *testing.T) {
			_t.Parallel()

			testCiliumLBConntracker(t, v)
		})
	}
}

func testCiliumLBConntracker(t *testing.T, ciliumVersion string) {
	t.Helper()

	suite := &ciliumLBConntrackerTestSuite{}

	httpBinServiceInstall := func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		var err error
		suite.httpBinService, err = istio.NewHttpbinServiceInstallation(e, pulumi.Provider(kubeProvider))
		return &kubeComp.Workload{}, err
	}

	npmToolsWorkload := func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		// NPM tools Workload
		return npmtools.K8sAppDefinition(e, kubeProvider, "npmtools", "http://httpbin.default.svc.cluster.local:8000")
	}

	ciliumHelmValues := map[string]pulumi.Input{
		"kubeProxyReplacement": pulumi.BoolPtr(true),
		"ipam": pulumi.Map{
			"method": pulumi.StringPtr("kubernetes"),
		},
		"socketLB": pulumi.Map{
			"hostNamespaceOnly": pulumi.BoolPtr(true),
		},
		"image": pulumi.Map{
			"tag": pulumi.StringPtr(ciliumVersion),
		},
	}

	name := strings.ReplaceAll(fmt.Sprintf("cilium-lb-%s", ciliumVersion), ".", "-")
	e2e.Run(t, suite,
		e2e.WithStackName(fmt.Sprintf("stack-%s", name)),
		e2e.WithProvisioner(
			awskubernetes.KindProvisioner(
				awskubernetes.WithName(name),
				awskubernetes.WithCiliumOptions(cilium.WithHelmValues(ciliumHelmValues), cilium.WithVersion(ciliumVersion)),
				awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(systemProbeConfigNPMHelmValues)),
				awskubernetes.WithWorkloadApp(httpBinServiceInstall),
				awskubernetes.WithWorkloadApp(npmToolsWorkload),
			),
		),
	)
}

// BeforeTest will be called before each test
func (suite *ciliumLBConntrackerTestSuite) BeforeTest(suiteName, testName string) {
	suite.BaseSuite.BeforeTest(suiteName, testName)
	// default is to reset the current state of the fakeintake aggregators
	if !suite.BaseSuite.IsDevMode() {
		suite.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// AfterTest will be called after each test
func (suite *ciliumLBConntrackerTestSuite) AfterTest(suiteName, testName string) {
	test1HostFakeIntakeNPMDumpInfo(suite.T(), suite.Env().FakeIntake)

	suite.BaseSuite.AfterTest(suiteName, testName)
}

func (suite *ciliumLBConntrackerTestSuite) TestCiliumConntracker() {
	fakeIntake := suite.Env().FakeIntake

	var hostname string
	suite.Require().EventuallyWithT(func(collect *assert.CollectT) {
		names, err := fakeIntake.Client().GetConnectionsNames()
		if assert.NoError(collect, err, "error getting connection names") &&
			assert.NotEmpty(collect, names) {
			hostname = names[0]
		}
	}, time.Minute, time.Second, "timed out getting connection names")

	var svcConns []*process.Connection
	suite.Require().EventuallyWithT(func(collect *assert.CollectT) {
		cnx, err := fakeIntake.Client().GetConnections()
		require.NoError(collect, err, "error getting connections")
		payloads := cnx.GetPayloadsByName(hostname)
		// only look at the last two payloads
		require.Greater(collect, len(payloads), 1, "at least 2 payloads not present")

		svcConns = nil
		for _, c := range append(payloads[len(payloads)-2].Connections, payloads[len(payloads)-1].Connections...) {
			if c.Raddr.Port != 8000 {
				return
			}

			if !assert.NotNil(collect, c.IpTranslation, "ip translation is nil for service connection") {
				return
			}

			svcConns = append(svcConns, c)
		}

		assert.NotEmpty(collect, svcConns, "no connections for service found")
	}, time.Minute, time.Second, "could not find connections for service")

	backends, frontendIP := suite.httpBinCiliumService()
	for _, c := range svcConns {
		suite.Assert().Equalf(frontendIP, c.Raddr.Ip, "front end address not equal to connection raddr")
		suite.Assert().Conditionf(func() bool {
			for _, be := range backends {
				if be.ip == c.IpTranslation.ReplSrcIP && be.port == uint16(c.IpTranslation.ReplSrcPort) {
					return true
				}
			}

			return false
		}, "")
	}
}

type ciliumBackend struct {
	ip   string
	port uint16
}

func (suite *ciliumLBConntrackerTestSuite) httpBinCiliumService() (backends []ciliumBackend, frontendIP string) {
	t := suite.T()
	t.Helper()

	var stdout string
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		ciliumPods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("kube-system").List(context.Background(), v1.ListOptions{
			LabelSelector: "k8s-app=cilium",
		})
		require.NoError(collect, err, "could no get cilium pods")
		require.NotNil(collect, ciliumPods, "cilium pods object is nil")
		require.NotEmpty(collect, ciliumPods.Items, "no cilium pods found")

		pod := ciliumPods.Items[0]
		var stderr string
		stdout, stderr, err = suite.Env().KubernetesCluster.KubernetesClient.PodExec("kube-system", pod.Name, "cilium-agent", []string{"cilium-dbg", "service", "list", "-o", "json"})
		require.NoError(collect, err, "error getting cilium service list")
		require.Empty(collect, stderr, "got output on stderr from cilium service list command", stderr)
		require.NotEmpty(collect, stdout, "empty output from cilium-dbg service list command")
	}, 20*time.Second, time.Second, "could not get cilium-agent pod")

	var services []interface{}
	err := json.Unmarshal([]byte(stdout), &services)
	suite.Require().NoError(err, "error deserializing output of cilium-dbg service list command")
	for _, svc := range services {
		spec := svc.(map[string]interface{})["spec"].(map[string]interface{})
		frontendAddr := spec["frontend-address"].(map[string]interface{})
		if frontendAddrPort := frontendAddr["port"].(float64); frontendAddrPort != 8000 {
			continue
		}
		if frontendAddrProto, ok := frontendAddr["protocol"]; ok && frontendAddrProto.(string) != "TCP" {
			continue
		}

		frontendIP = frontendAddr["ip"].(string)
		_backendAddrs := spec["backend-addresses"].([]interface{})
		for _, be := range _backendAddrs {
			be := be.(map[string]interface{})
			backends = append(backends, ciliumBackend{
				ip:   be["ip"].(string),
				port: uint16(be["port"].(float64)),
			})
		}

		break
	}

	return backends, frontendIP

}
