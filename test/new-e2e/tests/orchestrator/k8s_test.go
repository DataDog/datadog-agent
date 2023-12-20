package orchestrator

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/suite"
	"github.com/zorkian/go-datadog-api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
)

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

	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(ctx, "kind-cluster", stackConfig, ApplyNoReturn, false)
	if !suite.Assert().NoError(err) {
		stackName, err := infra.GetStackManager().GetPulumiStackName("kind-cluster")
		suite.Require().NoError(err)
		suite.T().Log(dumpKindClusterState(ctx, stackName))
		if !runner.GetProfile().AllowDevMode() || !*keepStacks {
			infra.GetStackManager().DeleteStack(ctx, "kind-cluster")
		}
		suite.T().FailNow()
	}

	fakeintakeHost := stackOutput.Outputs["fakeintake-host"].Value.(string)
	suite.Fakeintake = fakeintake.NewClient(fmt.Sprintf("http://%s", fakeintakeHost))
	suite.KubeClusterName = stackOutput.Outputs["kube-cluster-name"].Value.(string)
	suite.AgentLinuxHelmInstallName = stackOutput.Outputs["agent-linux-helm-install-name"].Value.(string)
	suite.AgentWindowsHelmInstallName = "none"

	kubeconfig := stackOutput.Outputs["kubeconfig"].Value.(string)

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

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	suite.T().Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/qcp-brm-ysc/e2e-tests-containers-k8s?refresh_mode=paused&tpl_var_kube_cluster_name%%5B0%%5D=%s&tpl_var_fake_intake_task_family%%5B0%%5D=%s-fakeintake-ecs&from_ts=%d&to_ts=%d&live=false",
		suite.KubeClusterName,
		suite.KubeClusterName,
		suite.startTime.UnixMilli(),
		suite.endTime.UnixMilli(),
	))

	ctx := context.Background()
	stackName, err := infra.GetStackManager().GetPulumiStackName("kind-cluster")
	suite.Require().NoError(err)
	suite.T().Log(dumpKindClusterState(ctx, stackName))
}

// TODO steal TestVersions

func (suite *k8sSuite) TestPayloads() {
	//ctx := context.Background()

	orchs, err := suite.Fakeintake.GetOrchestrators(nil)
	suite.NoError(err, "failed to get orch payloads from intake")
	fmt.Printf("Found %d orchs\n", len(orchs))
	for _, o := range orchs {
		fmt.Println("- " + o.Name)
	}

	pods, err := suite.Fakeintake.GetOrchestrators(&fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorPod})
	suite.NoError(err, "failed to get pods from intake")
	fmt.Printf("Found %d pods\n", len(pods))
	for _, p := range pods {
		fmt.Println("- " + p.Pod.Metadata.Name + " " + p.Pod.Status)
	}
}

func (suite *k8sSuite) podExec(namespace, pod, container string, cmd []string) (stdout, stderr string, err error) {
	req := suite.K8sClient.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod).SubResource("exec")
	option := &corev1.PodExecOptions{
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
		Container: container,
		Command:   cmd,
	}

	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(suite.K8sConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	var stdoutSb, stderrSb strings.Builder
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdoutSb,
		Stderr: &stderrSb,
	})
	if err != nil {
		return "", "", err
	}

	return stdoutSb.String(), stderrSb.String(), nil
}
