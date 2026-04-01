// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	componentsremote "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

const (
	// fakeOpmsImage is the pre-built fake OPMS server image.
	// TODO: replace with actual registry path once the CI image build step is added.
	fakeOpmsImage = "669783387624.dkr.ecr.us-east-1.amazonaws.com/datadog/fakeopms:latest"

	fakeOpmsName      = "fake-opms"
	fakeOpmsNamespace = "datadog"
	fakeOpmsPort      = 8080

	fakeOpmsDNS = "fake-opms.datadog.svc.cluster.local"
)

// parHelmValuesTemplate configures the agent with PAR enabled and pointing at the fake OPMS.
const parHelmValuesTemplate = `
datadog:
  kubelet:
    tlsVerify: false
  clusterName: "%s"
  ddUrl: "http://fake-opms.datadog.svc.cluster.local:8080"
  privateActionRunner:
    enabled: true
    selfEnroll: false
    urn: "%s"
    privateKey: "%s"
agents:
  useHostNetwork: true
`

// parK8sProvisioner provisions a Kind-on-EC2 cluster with:
//   - The fake OPMS server deployed as a K8s Deployment+Service
//   - The Datadog Agent with PAR enabled, ddUrl pointing at fake OPMS
func parK8sProvisioner(runnerURN, privateKeyB64 string) provisioners.Provisioner {
	p := provisioners.NewTypedPulumiProvisioner[environments.Kubernetes]("par-k8s",
		func(ctx *pulumi.Context, env *environments.Kubernetes) error {
			name := "kind"
			awsEnv, err := aws.NewEnvironment(ctx)
			if err != nil {
				return fmt.Errorf("aws.NewEnvironment: %w", err)
			}

			// 1. Provision EC2 VM
			host, err := ec2.NewVM(awsEnv, name)
			if err != nil {
				return fmt.Errorf("ec2.NewVM: %w", err)
			}

			installEcrCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
			if err != nil {
				return fmt.Errorf("ec2.InstallECRCredentialsHelper: %w", err)
			}

			// 2. Create standard Kind cluster (no GPU)
			kindCluster, err := kubeComp.NewKindCluster(&awsEnv, host, name,
				awsEnv.KubernetesVersion(),
				utils.PulumiDependsOn(installEcrCmd),
			)
			if err != nil {
				return fmt.Errorf("kubeComp.NewKindCluster: %w", err)
			}
			if err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
				return fmt.Errorf("kindCluster.Export: %w", err)
			}

			kubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
				EnableServerSideApply: pulumi.Bool(true),
				Kubeconfig:            kindCluster.KubeConfig,
			})
			if err != nil {
				return fmt.Errorf("kubernetes.NewProvider: %w", err)
			}

			// 3. Pull fake OPMS image into Kind nodes
			fakeOpmsCmds, err := pullImagesIntoKindNodes(&awsEnv, host, kindCluster, []string{fakeOpmsImage})
			if err != nil {
				return fmt.Errorf("pullImagesIntoKindNodes: %w", err)
			}

			// 4. Deploy fake OPMS as K8s Deployment + ClusterIP Service
			if err = deployFakeOpms(ctx, kubeProvider, utils.PulumiDependsOn(fakeOpmsCmds...)); err != nil {
				return fmt.Errorf("deployFakeOpms: %w", err)
			}

			// 5. Deploy Datadog agent via Helm, PAR enabled
			helmValues := fmt.Sprintf(parHelmValuesTemplate, ctx.Stack(), runnerURN, privateKeyB64)
			agent, err := helm.NewKubernetesAgent(&awsEnv, name, kubeProvider,
				kubernetesagentparams.WithHelmValues(helmValues),
				kubernetesagentparams.WithClusterName(kindCluster.ClusterName),
				kubernetesagentparams.WithTags([]string{"stackid:" + ctx.Stack()}),
			)
			if err != nil {
				return fmt.Errorf("helm.NewKubernetesAgent: %w", err)
			}
			if err = agent.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
				return fmt.Errorf("agent.Export: %w", err)
			}

			return nil
		}, nil)

	p.SetDiagnoseFunc(awskubernetes.DiagnoseFunc)
	return p
}

// deployFakeOpms creates a K8s Deployment and ClusterIP Service for the fake OPMS.
// PAR accesses it via cluster-internal DNS: fake-opms.datadog.svc.cluster.local:8080
func deployFakeOpms(ctx *pulumi.Context, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) error {
	labels := pulumi.StringMap{"app": pulumi.String(fakeOpmsName)}
	ns := pulumi.String(fakeOpmsNamespace)
	pulumiOpts := append(opts, pulumi.Provider(kubeProvider))

	_, err := appsv1.NewDeployment(ctx, fakeOpmsName, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(fakeOpmsName), Namespace: ns, Labels: labels},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String(fakeOpmsName),
							Image: pulumi.String(fakeOpmsImage),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(fakeOpmsPort)},
							},
						},
					},
				},
			},
		},
	}, pulumiOpts...)
	if err != nil {
		return err
	}

	_, err = corev1.NewService(ctx, fakeOpmsName+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(fakeOpmsName), Namespace: ns},
		Spec: &corev1.ServiceSpecArgs{
			Selector: labels,
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{Port: pulumi.Int(fakeOpmsPort), TargetPort: pulumi.Any(fakeOpmsPort)},
			},
		},
	}, pulumiOpts...)
	return err
}

// pullImagesIntoKindNodes loads container images into Kind cluster nodes via crictl.
func pullImagesIntoKindNodes(e *aws.Environment, vm *componentsremote.Host, kindCluster *kubeComp.Cluster, images []string, dependsOn ...pulumi.Resource) ([]pulumi.Resource, error) {
	var cmds []pulumi.Resource
	for i, image := range images {
		retryPull := fmt.Sprintf(
			"counter=0; while [ \\$counter -lt 3 ] && ! crictl pull %s ; do sleep 1; counter=\\$((counter+1)); done; [ \\$counter -lt 3 ]",
			image,
		)
		cmd, err := vm.OS.Runner().Command(
			e.CommonNamer().ResourceName("kind-node-pull", fmt.Sprintf("image-%d", i)),
			&command.Args{
				Create: pulumi.Sprintf("kind get nodes --name %s | xargs -I {} docker exec {} /bin/bash -c %q",
					kindCluster.ClusterName, retryPull),
			},
			utils.PulumiDependsOn(dependsOn...),
		)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	}
	return cmds, nil
}
