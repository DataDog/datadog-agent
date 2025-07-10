// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"fmt"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/gke"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/scenarios/gcp/fakeintake"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/common"
)

const (
	defaultProvisionerName = "k8s"
)

// KubernetesProvisionerParams contains all the parameters needed to create a Kubernetes environment
type KubernetesProvisionerParams struct {
	name               string
	testName           string
	operatorOptions    []operatorparams.Option
	ddaOptions         []agentwithoperatorparams.Option
	k8sVersion         string
	kustomizeResources []string

	gkeOptions        []gke.Option
	fakeintakeOptions []fakeintake.Option
	extraConfigParams runner.ConfigMap
	yamlWorkloads     []YAMLWorkload
	workloadAppFuncs  []func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error)
	local             bool
}

func newKubernetesProvisionerParams() *KubernetesProvisionerParams {
	return &KubernetesProvisionerParams{
		name:               defaultProvisionerName,
		testName:           "",
		ddaOptions:         []agentwithoperatorparams.Option{},
		operatorOptions:    []operatorparams.Option{},
		k8sVersion:         common.K8sVersion,
		kustomizeResources: nil,
		fakeintakeOptions:  []fakeintake.Option{},
		extraConfigParams:  runner.ConfigMap{},
		yamlWorkloads:      []YAMLWorkload{},
		workloadAppFuncs:   []func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error){},
		local:              false,
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

// YAMLWorkload defines the parameters for a Kubernetes resource's YAML file
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

// WithWorkloadApp adds a workload app to the environment
func WithWorkloadApp(appFunc func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error)) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.workloadAppFuncs = append(params.workloadAppFuncs, appFunc)
		return nil
	}
}

// KubernetesProvisioner generic Kubernetes provisioner wrapper that creates a new provisioner
// Inspired by https://github.com/DataDog/datadog-agent/blob/main/test/new-e2e/pkg/environments/local/kubernetes/kind.go
func KubernetesProvisioner(opts ...KubernetesProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	var provisioner provisioners.TypedProvisioner[environments.Kubernetes]

	params := newKubernetesProvisionerParams()
	k8sVersion := common.K8sVersion
	if params.k8sVersion != "" {
		k8sVersion = params.k8sVersion
	}
	params.extraConfigParams.Set("ddinfra:kubernetesVersion", k8sVersion, false)
	_ = optional.ApplyOptions(params, opts)

	if params.local {
		provisioner = provisioners.NewTypedPulumiProvisioner("local-kind", func(ctx *pulumi.Context, env *environments.Kubernetes) error {
			// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
			// and it's easy to forget about it, leading to hard to debug issues.
			pprams := newKubernetesProvisionerParams()
			_ = optional.ApplyOptions(pprams, opts)

			return LocalKindRunFunc(ctx, env, pprams)

		}, params.extraConfigParams)
		return provisioner
	}

	inCI := os.Getenv("GITLAB_CI")

	if strings.ToLower(inCI) == "true" {
		params.extraConfigParams.Set("ddagent:imagePullRegistry", "669783387624.dkr.ecr.us-east-1.amazonaws.com", false)
		params.extraConfigParams.Set("ddagent:imagePullUsername", "AWS", false)
		params.extraConfigParams.Set("ddagent:imagePullPassword", common.ImgPullPassword, true)
		params.extraConfigParams.Set("ddinfra:env", "gcp/agent-qa", false)
	} else {
		params.extraConfigParams.Set("ddinfra:env", "gcp/agent-sandbox", false)
	}

	provisioner = provisioners.NewTypedPulumiProvisioner("gke", func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		pprams := newKubernetesProvisionerParams()
		_ = optional.ApplyOptions(pprams, opts)

		return GkeRunFunc(ctx, env, pprams)

	}, params.extraConfigParams)

	return provisioner
}

// KustomizeWorkloadAppFunc Installs the operator e2e kustomize directory and any extra kustomize resources
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

// YAMLWorkloadAppFunc Applies a Kubernetes resource yaml file
func YAMLWorkloadAppFunc(yamlWorkload YAMLWorkload) func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	return func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		k8sComponent := &kubeComp.Workload{}
		if err := e.Ctx().RegisterComponentResource("dd:apps", "k8s-apply", k8sComponent); err != nil {
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
