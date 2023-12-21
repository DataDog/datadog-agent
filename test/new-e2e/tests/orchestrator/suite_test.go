package orchestrator

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"github.com/zorkian/go-datadog-api"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
)

var replaceStacks = flag.Bool("replace-stacks", false, "Attempt to destroy the Pulumi stacks at the beginning of the tests")
var keepStacks = flag.Bool("keep-stacks", false, "Do not destroy the Pulumi stacks at the end of the tests")

func TestMain(m *testing.M) {
	code := m.Run()
	fmt.Println("in main")
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

	startTime     time.Time
	endTime       time.Time
	datadogClient *datadog.Client
	Fakeintake    *fakeintake.Client
	clusterName   string

	KubeClusterName             string
	AgentLinuxHelmInstallName   string
	AgentWindowsHelmInstallName string

	K8sConfig *restclient.Config
	K8sClient *kubernetes.Clientset
}

func TestKindSuite(t *testing.T) {
	fmt.Println("in kind suite")
	suite.Run(t, &k8sSuite{})
}

func (suite *k8sSuite) SetupSuite() {
	fmt.Println("in setup")
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
	if !suite.Assert().NoError(err) {
		stackName, err := infra.GetStackManager().GetPulumiStackName("kind-cluster")
		suite.Require().NoError(err)
		suite.T().Log(dumpKindClusterState(ctx, stackName))
		if !runner.GetProfile().AllowDevMode() || !*keepStacks {
			infra.GetStackManager().DeleteStack(ctx, "kind-cluster", nil)
		}
		suite.T().FailNow()
	}

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	suite.Fakeintake = fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost))
	suite.KubeClusterName = stackOutput.Outputs["kube-cluster-name"].Value.(string)
	suite.AgentLinuxHelmInstallName = stackOutput.Outputs["agent-linux-helm-install-name"].Value.(string)
	suite.AgentWindowsHelmInstallName = "none"

	kubeconfig := stackOutput.Outputs["kubeconfig"].Value.(string)
	// useful for setting up your local kubeconfig
	//fmt.Println("LOCAL KUBECONFIG")
	//fmt.Println(kubeconfig)

	kubeconfigFile := path.Join(suite.T().TempDir(), "kubeconfig")
	suite.Require().NoError(os.WriteFile(kubeconfigFile, []byte(kubeconfig), 0600))

	suite.K8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigFile)
	suite.Require().NoError(err)

	suite.K8sClient = kubernetes.NewForConfigOrDie(suite.K8sConfig)

	suite.clusterName = suite.KubeClusterName

	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	suite.Require().NoError(err)
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	suite.Require().NoError(err)
	suite.datadogClient = datadog.NewClient(apiKey, appKey)

	suite.startTime = time.Now()
}

func (suite *k8sSuite) TearDownSuite() {
	fmt.Println("in teardown")
	suite.endTime = time.Now()
	ctx := context.Background()
	stackName, err := infra.GetStackManager().GetPulumiStackName("kind-cluster")
	suite.Require().NoError(err)
	suite.T().Log(dumpKindClusterState(ctx, stackName))
}
