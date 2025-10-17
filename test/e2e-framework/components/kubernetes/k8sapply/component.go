package k8sapply

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type YAMLWorkload struct {
	Name string
	Path string
}

// K8sAppDefinition applies a generic Kubernetes resource from a YAML source file defined as a YAMLWorkload
func K8sAppDefinition(yamlWorkload YAMLWorkload) func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	return func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		k8sComponent := &kubeComp.Workload{}

		if err := e.Ctx().RegisterComponentResource("dd:apps", fmt.Sprintf("k8s-apply-%s", yamlWorkload.Name), k8sComponent); err != nil {
			return nil, err
		}
		_, err := yaml.NewConfigFile(e.Ctx(), yamlWorkload.Name, &yaml.ConfigFileArgs{
			File: yamlWorkload.Path,
		}, pulumi.Provider(kubeProvider), pulumi.Parent(k8sComponent))
		if err != nil {
			return nil, err
		}
		return k8sComponent, nil
	}
}
