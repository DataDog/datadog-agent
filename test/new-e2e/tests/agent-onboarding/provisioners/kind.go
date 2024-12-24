// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package provisioners

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/common"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/operator"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/local"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"path/filepath"
)

const (
	provisionerBaseID      = "aws-kind"
	defaultProvisionerName = "kind"
)

type K8sEnv struct {
	environments.Kubernetes
}

type KubernetesProvisionerParams struct {
	name               string
	testName           string
	operatorOptions    []operatorparams.Option
	ddaOptions         []agentwithoperatorparams.Option
	k8sVersion         string
	kustomizeResources []string

	fakeintakeOptions []fakeintake.Option
	extraConfigParams runner.ConfigMap
	yamlWorkloads     []YAMLWorkload
	local             bool
}

func newKubernetesProvisionerParams() *KubernetesProvisionerParams {
	return &KubernetesProvisionerParams{
		name:               defaultProvisionerName,
		testName:           "",
		ddaOptions:         []agentwithoperatorparams.Option{},
		operatorOptions:    []operatorparams.Option{},
		k8sVersion:         common.GetEnv("K8S_VERSION", "1.26"),
		kustomizeResources: nil,
		fakeintakeOptions:  []fakeintake.Option{},
		extraConfigParams:  runner.ConfigMap{},
		yamlWorkloads:      []YAMLWorkload{},
	}
}

// KubernetesProvisionerOption is a function that modifies the KubernetesProvisionerParams
type KubernetesProvisionerOption func(params *KubernetesProvisionerParams) error

// WithName sets the name of the provisioner
func WithName(name string) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithTestName sets the name of the test
func WithTestName(name string) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.testName = name
		return nil
	}
}

// WithK8sVersion sets the kubernetes version
func WithK8sVersion(k8sVersion string) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.k8sVersion = k8sVersion
		return nil
	}
}

// WithOperatorOptions adds options to the DatadogAgent resource
func WithOperatorOptions(opts ...operatorparams.Option) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.operatorOptions = opts
		return nil
	}
}

// WithoutOperator removes the Datadog Operator resource
func WithoutOperator() KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.operatorOptions = nil
		return nil
	}
}

// WithDDAOptions adds options to the DatadogAgent resource
func WithDDAOptions(opts ...agentwithoperatorparams.Option) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.ddaOptions = opts
		return nil
	}
}

// WithoutDDA removes the DatadogAgent resource
func WithoutDDA() KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.ddaOptions = nil
		return nil
	}
}

// WithExtraConfigParams adds extra config parameters to the environment
func WithExtraConfigParams(configMap runner.ConfigMap) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// WithKustomizeResources adds extra kustomize resources
func WithKustomizeResources(k []string) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.kustomizeResources = k
		return nil
	}
}

// WithoutFakeIntake removes the fake intake
func WithoutFakeIntake() KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithLocal uses the localKindRunFunc to create a local kind environment
func WithLocal(local bool) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.local = local
		return nil
	}
}

type YAMLWorkload struct {
	Name string
	Path string
}

// WithYAMLWorkload adds a workload app to the environment for given YAML file path
func WithYAMLWorkload(yamlWorkload YAMLWorkload) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.yamlWorkloads = append(params.yamlWorkloads, yamlWorkload)
		return nil
	}
}

// KubernetesProvisioner creates a new local Kubernetes w/operator provisioner
// Inspired by https://github.com/DataDog/datadog-agent/blob/main/test/new-e2e/pkg/environments/local/kubernetes/kind.go
func KubernetesProvisioner(opts ...KubernetesProvisionerOption) e2e.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	var awsK8sOpts []awskubernetes.ProvisionerOption
	var provisioner e2e.TypedProvisioner[environments.Kubernetes]

	params := newKubernetesProvisionerParams()
	_ = optional.ApplyOptions(params, opts)
	provisionerName := provisionerBaseID + params.name

	if !params.local {
		awsK8sOpts = newAWSK8sProvisionerOpts(params)
		provisioner = awskubernetes.KindProvisioner(awsK8sOpts...)
		return provisioner
	}

	provisioner = e2e.NewTypedPulumiProvisioner(provisionerName, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newKubernetesProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return localKindRunFunc(ctx, env, params)

	}, params.extraConfigParams)

	return provisioner
}

func newAWSK8sProvisionerOpts(params *KubernetesProvisionerParams) []awskubernetes.ProvisionerOption {
	extraConfig := params.extraConfigParams
	extraConfig.Merge(runner.ConfigMap{"ddinfra:kubernetesVersion": auto.ConfigValue{Value: params.k8sVersion}})

	newOpts := []awskubernetes.ProvisionerOption{
		awskubernetes.WithName(params.name),
		awskubernetes.WithOperator(),
		awskubernetes.WithOperatorDDAOptions(params.ddaOptions...),
		awskubernetes.WithOperatorOptions(params.operatorOptions...),
		awskubernetes.WithExtraConfigParams(extraConfig),
		awskubernetes.WithWorkloadApp(KustomizeWorkloadAppFunc(params.testName, params.kustomizeResources)),
		awskubernetes.WithFakeIntakeOptions(params.fakeintakeOptions...),
	}

	for _, yamlWorkload := range params.yamlWorkloads {
		newOpts = append(newOpts, awskubernetes.WithWorkloadApp(YAMLWorkloadAppFunc(yamlWorkload)))
	}

	return newOpts
}

func localKindRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *KubernetesProvisionerParams) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	kindCluster, err := kubeComp.NewLocalKindCluster(&localEnv, localEnv.CommonNamer().ResourceName("local-kind"), params.k8sVersion)
	if err != nil {
		return err
	}

	if err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	// Build Kubernetes provider
	kindKubeProvider, err := kubernetes.NewProvider(ctx, localEnv.CommonNamer().ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		Kubeconfig:            kindCluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	if params.fakeintakeOptions != nil {
		fakeintakeOpts := []fakeintake.Option{fakeintake.WithLoadBalancer()}
		params.fakeintakeOptions = append(fakeintakeOpts, params.fakeintakeOptions...)

		fakeIntake, err := fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
		if err != nil {
			return err
		}
		if err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}

		if params.ddaOptions != nil {
			params.ddaOptions = append(params.ddaOptions, agentwithoperatorparams.WithFakeIntake(fakeIntake))
		}
	} else {
		env.FakeIntake = nil
	}

	ns, err := corev1.NewNamespace(ctx, localEnv.CommonNamer().ResourceName("k8s-namespace"), &corev1.NamespaceArgs{Metadata: &metav1.ObjectMetaArgs{
		Name: pulumi.String("e2e-operator"),
	}}, pulumi.Provider(kindKubeProvider))

	if err != nil {
		return err
	}

	// Install kustomizations
	kustomizeDirPath, err := filepath.Abs(NewMgrKustomizeDirPath)
	if err != nil {
		return err
	}

	err = UpdateKustomization(kustomizeDirPath, params.kustomizeResources)
	if err != nil {
		return err
	}
	kustomizeOpts := []pulumi.ResourceOption{
		pulumi.DependsOn([]pulumi.Resource{ns}),
		pulumi.Provider(kindKubeProvider),
	}

	e2eKustomize, err := kustomize.NewDirectory(ctx, "e2e-manager",
		kustomize.DirectoryArgs{
			Directory: pulumi.String(kustomizeDirPath),
		}, kustomizeOpts...)
	if err != nil {
		return err
	}

	// Create Operator component
	var operatorComp *operator.Operator
	if params.operatorOptions != nil {
		operatorOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, ns}),
		}
		params.operatorOptions = append(params.operatorOptions, operatorparams.WithPulumiResourceOptions(operatorOpts...))

		operatorComp, err = operator.NewOperator(&localEnv, localEnv.CommonNamer().ResourceName("operator"), kindKubeProvider, params.operatorOptions...)
		if err != nil {
			return err
		}
	}

	// Setup DDA options
	if params.ddaOptions != nil && params.operatorOptions != nil {
		ddaResourceOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, operatorComp}),
		}
		params.ddaOptions = append(
			params.ddaOptions,
			agentwithoperatorparams.WithPulumiResourceOptions(ddaResourceOpts...))

		ddaComp, err := agent.NewDDAWithOperator(&localEnv, params.name, kindKubeProvider, params.ddaOptions...)
		if err != nil {
			return err
		}

		if err = ddaComp.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}
	} else {
		env.Agent = nil
	}

	for _, workload := range params.yamlWorkloads {
		_, err = yaml.NewConfigFile(ctx, workload.Name, &yaml.ConfigFileArgs{
			File: workload.Path,
		}, pulumi.Provider(kindKubeProvider))
		if err != nil {
			return err
		}
	}
	//
	//for _, appFunc := range params.workloadAppFuncs {
	//	_, err := appFunc(&awsEnv, kubeProvider)
	//	if err != nil {
	//		return err
	//	}
	//}

	return nil
}

func KustomizeWorkloadAppFunc(name string, extraKustomizeResources []string) func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	return func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		k8sComponent := &kubeComp.Workload{}
		if err := e.Ctx().RegisterComponentResource("dd:apps", fmt.Sprintf("kustomize-%s", name), k8sComponent, pulumi.DeleteBeforeReplace(true)); err != nil {
			return nil, err
		}

		// Install kustomizations
		kustomizeDirPath, err := filepath.Abs(NewMgrKustomizeDirPath)
		if err != nil {
			return nil, err
		}

		err = UpdateKustomization(kustomizeDirPath, extraKustomizeResources)
		if err != nil {
			return nil, err
		}
		kustomizeOpts := []pulumi.ResourceOption{
			pulumi.Provider(kubeProvider),
			pulumi.Parent(k8sComponent),
		}

		_, err = kustomize.NewDirectory(e.Ctx(), "e2e-manager",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(kustomizeDirPath),
			}, kustomizeOpts...)
		if err != nil {
			return nil, err
		}
		return k8sComponent, nil
	}
}

func YAMLWorkloadAppFunc(yamlWorkload YAMLWorkload) func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	return func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		k8sComponent := &kubeComp.Workload{}
		if err := e.Ctx().RegisterComponentResource("dd:apps", "kustomize", k8sComponent); err != nil {
			return nil, err
		}
		_, err := yaml.NewConfigFile(e.Ctx(), yamlWorkload.Name, &yaml.ConfigFileArgs{
			File: yamlWorkload.Path,
		}, pulumi.Provider(kubeProvider))
		if err != nil {
			return nil, err
		}
		return k8sComponent, nil
	}
}
