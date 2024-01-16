// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
)

var replaceStacks = flag.Bool("replace-stacks", false, "Attempt to destroy the Pulumi stacks at the beginning of the tests")
var keepStacks = flag.Bool("keep-stacks", false, "Do not destroy the Pulumi stacks at the end of the tests")

func TestMain(m *testing.M) {
	code := m.Run()
	if runner.GetProfile().AllowDevMode() && *keepStacks {
		fmt.Fprintln(os.Stderr, "Keeping stacks")
	} else {
		fmt.Fprintln(os.Stderr, "Cleaning up stacks")
		errs := infra.GetStackManager().Cleanup(context.Background())
		for _, err := range errs {
			fmt.Fprint(os.Stderr, err.Error())
		}
	}
	os.Exit(code)
}

type k8sSuite struct {
	suite.Suite
	KubeClusterName string
	Fakeintake      *fakeintake.Client
	K8sConfig       *restclient.Config
	K8sClient       *kubernetes.Clientset
}

func TestKindSuite(t *testing.T) {
	suite.Run(t, &k8sSuite{})
}

func (suite *k8sSuite) SetupSuite() {
	ctx := context.Background()

	stackConfig := runner.ConfigMap{
		"ddagent:deploy":        auto.ConfigValue{Value: "true"},
		"ddagent:fakeintake":    auto.ConfigValue{Value: "true"},
		"ddtestworkload:deploy": auto.ConfigValue{Value: "true"},
		"dddogstatsd:deploy":    auto.ConfigValue{Value: "true"},
	}

	if runner.GetProfile().AllowDevMode() && *replaceStacks {
		fmt.Fprintln(os.Stderr, "Destroying existing stack")
		err := infra.GetStackManager().DeleteStack(ctx, "kind-cluster", nil)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
		}
	}
	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, "kind-cluster", stackConfig, Apply, false, nil)

	suite.printKubeConfig(stackOutput)

	if !suite.Assert().NoError(err) {
		if !runner.GetProfile().AllowDevMode() || !*keepStacks {
			infra.GetStackManager().DeleteStack(ctx, "kind-cluster", nil)
		}
		suite.T().FailNow()
	}

	kubeconfig := stackOutput.Outputs["kubeconfig"].Value.(string)

	suite.KubeClusterName = stackOutput.Outputs["kube-cluster-name"].Value.(string)

	var fakeintake components.FakeIntake
	fiSerialized, err := json.Marshal(stackOutput.Outputs["dd-Fakeintake-aws-kind"].Value)
	suite.Require().NoError(err)
	suite.Require().NoError(fakeintake.Import(fiSerialized, &fakeintake))
	suite.Require().NoError(fakeintake.Init(suite))
	suite.Fakeintake = fakeintake.Client()

	kubeconfigFile := path.Join(suite.T().TempDir(), "kubeconfig")
	suite.Require().NoError(os.WriteFile(kubeconfigFile, []byte(kubeconfig), 0600))

	suite.K8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigFile)
	suite.Require().NoError(err)

	suite.K8sClient = kubernetes.NewForConfigOrDie(suite.K8sConfig)
}

func (suite *k8sSuite) TearDownSuite() {
	suite.summarizeResources()
	suite.summarizeManifests()
	fmt.Printf("Link to view resources:\nhttps://ddstaging.datadoghq.com/orchestration/overview/pod?query=tag%%23kube_cluster_name%%3A%s\n", suite.KubeClusterName)
}

// printKubeConfig prints the command to update the local kubeconfig to point to the kind cluster
func (suite *k8sSuite) printKubeConfig(stackOutput auto.UpResult) {
	if out, ok := stackOutput.Outputs["kubeconfig"]; ok {
		//fmt.Println("LOCAL KUBECONFIG")
		//fmt.Println(out.Value.(string))
		var cfg struct {
			Clusters []struct {
				Cluster struct {
					Server string `yaml:"server"`
				} `yaml:"cluster"`
			} `yaml:"clusters"`
			Users []struct {
				User struct {
					Cert string `yaml:"client-certificate-data"`
					Key  string `yaml:"client-key-data"`
				} `yaml:"user"`
			} `yaml:"users"`
		}
		err := yaml.Unmarshal([]byte(out.Value.(string)), &cfg)
		if err != nil {
			fmt.Println("FAILED TO GENERATE: COMMAND TO UPDATE LOCAL KUBECONFIG !!!")
			fmt.Println(err.Error())
		} else {
			var server, cert, key string
			if len(cfg.Clusters) > 0 {
				server = cfg.Clusters[0].Cluster.Server
			}
			if len(cfg.Users) > 0 {
				cert = cfg.Users[0].User.Cert
				key = cfg.Users[0].User.Key
			}
			fmt.Println("COMMAND TO UPDATE LOCAL KUBECONFIG")
			fmt.Printf("cat ~/.kube/config | yq '( .clusters[] | select(.name == \"kind-kind\") ).cluster.server |= \"%s\"' | yq '( .users[] | select(.name == \"kind-kind\") ).user |= {\"client-certificate-data\": \"%s\", \"client-key-data\": \"%s\"}' > ~/.kube/config_updated && mv ~/.kube/config_updated ~/.kube/config\n", server, cert, key)
		}
	}
}

// summarizeResources prints a summary of the resources collected by the fake input
func (suite *k8sSuite) summarizeResources() {
	payloads, err := suite.Fakeintake.GetOrchestratorResources(nil)
	if err != nil {
		fmt.Println("failed to get manifest resource from intake")
		return
	}
	latest := map[agentmodel.MessageType]map[string]*aggregator.OrchestratorPayload{}
	for _, p := range payloads {
		if _, ok := latest[p.Type]; !ok {
			latest[p.Type] = map[string]*aggregator.OrchestratorPayload{}
		}
		existing, ok := latest[p.Type][p.UID]
		if !ok || existing.CollectedTime.Before(p.CollectedTime) {
			latest[p.Type][p.UID] = p
		}
	}
	fmt.Println("Most recently collected resources:")
	for typ, resources := range latest {
		for uid, p := range resources {
			fmt.Printf(" - type:%d, name:%s, uid:%s, collected:%s\n", typ, p.Name, uid, p.CollectedTime.Format(time.RFC3339))
		}
	}
}

// summarizeManifests prints a summary of the manifests collected by the fake input
func (suite *k8sSuite) summarizeManifests() {
	payloads, err := suite.Fakeintake.GetOrchestratorManifests()
	if err != nil {
		fmt.Println("failed to get manifest payloads from intake")
		return
	}
	latest := map[agentmodel.MessageType]map[string]*aggregator.OrchestratorManifestPayload{}
	for _, p := range payloads {
		if _, ok := latest[p.Type]; !ok {
			latest[p.Type] = map[string]*aggregator.OrchestratorManifestPayload{}
		}
		existing, ok := latest[p.Type][p.Manifest.Uid]
		if !ok || existing.CollectedTime.Before(p.CollectedTime) {
			latest[p.Type][p.Manifest.Uid] = p
		}
	}
	fmt.Println("Most recently collected manifests:")
	for typ, manifs := range latest {
		for uid, p := range manifs {
			manif := manifest{}
			err := yaml.Unmarshal(p.Manifest.Content, &manif)
			if err != nil {
				continue // unable to parse manifest content
			}
			fmt.Printf(" - type:%d, name:%s, ns:%s, kind:%s, apiVer:%s, uid:%s, collected:%s\n", typ, manif.Metadata.Name, manif.Metadata.Namespace, manif.Kind, manif.APIVersion, uid, p.CollectedTime.Format(time.RFC3339))
		}
	}
}
