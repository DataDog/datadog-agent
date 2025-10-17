package agent

import (
	"github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/components"
)

type KubernetesAgentOutput struct {
	components.JSONImporter

	LinuxNodeAgent     kubernetes.KubernetesObjRefOutput `json:"linuxNodeAgent"`
	LinuxClusterAgent  kubernetes.KubernetesObjRefOutput `json:"linuxClusterAgent"`
	LinuxClusterChecks kubernetes.KubernetesObjRefOutput `json:"linuxClusterChecks"`

	WindowsNodeAgent     kubernetes.KubernetesObjRefOutput `json:"windowsNodeAgent"`
	WindowsClusterAgent  kubernetes.KubernetesObjRefOutput `json:"windowsClusterAgent"`
	WindowsClusterChecks kubernetes.KubernetesObjRefOutput `json:"windowsClusterChecks"`

	FIPSEnabled bool `json:"fipsEnabled"`
}

// KubernetesAgent is an installer to install the Datadog Agent on a Kubernetes cluster.
type KubernetesAgent struct {
	pulumi.ResourceState
	components.Component

	LinuxNodeAgent     *kubernetes.KubernetesObjectRef `pulumi:"linuxNodeAgent"`
	LinuxClusterAgent  *kubernetes.KubernetesObjectRef `pulumi:"linuxClusterAgent"`
	LinuxClusterChecks *kubernetes.KubernetesObjectRef `pulumi:"linuxClusterChecks"`

	WindowsNodeAgent     *kubernetes.KubernetesObjectRef `pulumi:"windowsNodeAgent"`
	WindowsClusterAgent  *kubernetes.KubernetesObjectRef `pulumi:"windowsClusterAgent"`
	WindowsClusterChecks *kubernetes.KubernetesObjectRef `pulumi:"windowsClusterChecks"`

	ClusterAgentToken pulumi.StringOutput
	FIPSEnabled       pulumi.BoolOutput `pulumi:"fipsEnabled"`
}

func (h *KubernetesAgent) Export(ctx *pulumi.Context, out *KubernetesAgentOutput) error {
	return components.Export(ctx, h, out)
}
