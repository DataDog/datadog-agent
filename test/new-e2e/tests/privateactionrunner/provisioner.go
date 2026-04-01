// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

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
	fakeOpmsImageTag  = "fakeopms:e2e"
	fakeOpmsName      = "fake-opms"
	fakeOpmsNamespace = "datadog"
	fakeOpmsPort      = 8080
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
//   - The fake OPMS server built locally and shipped to the EC2 VM
//   - The Datadog Agent with PAR enabled, ddUrl pointing at fake OPMS
func parK8sProvisioner(runnerURN, privateKeyB64 string) provisioners.Provisioner {
	p := provisioners.NewTypedPulumiProvisioner[environments.Kubernetes]("par-k8s",
		func(ctx *pulumi.Context, env *environments.Kubernetes) error {
			name := "kind"
			awsEnv, err := aws.NewEnvironment(ctx)
			if err != nil {
				return fmt.Errorf("aws.NewEnvironment: %w", err)
			}

			// 1. Build fake OPMS Docker image locally, save as tar.
			tarPath := filepath.Join(os.TempDir(), "fakeopms-e2e.tar")
			buildCmd, saveCmd, err := buildFakeOpmsLocally(ctx, &awsEnv, tarPath)
			if err != nil {
				return fmt.Errorf("buildFakeOpmsLocally: %w", err)
			}

			// 2. Provision EC2 VM
			host, err := ec2.NewVM(awsEnv, name)
			if err != nil {
				return fmt.Errorf("ec2.NewVM: %w", err)
			}

			installEcrCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
			if err != nil {
				return fmt.Errorf("ec2.InstallECRCredentialsHelper: %w", err)
			}

			// 3. Copy image tar to EC2 and load into Docker + Kind
			kindReadyDeps, err := shipFakeOpmsToEC2(&awsEnv, host, tarPath, utils.PulumiDependsOn(saveCmd, installEcrCmd))
			if err != nil {
				return fmt.Errorf("shipFakeOpmsToEC2: %w", err)
			}

			// 4. Create standard Kind cluster (no GPU)
			kindCluster, err := kubeComp.NewKindCluster(&awsEnv, host, name,
				awsEnv.KubernetesVersion(),
				utils.PulumiDependsOn(kindReadyDeps...),
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

			// 5. Load image into Kind nodes
			kindLoadCmd, err := host.OS.Runner().Command(
				awsEnv.CommonNamer().ResourceName("kind-load-fakeopms"),
				&command.Args{
					Create: pulumi.Sprintf("kind load docker-image %s --name %s", fakeOpmsImageTag, kindCluster.ClusterName),
				},
				utils.PulumiDependsOn(kindCluster),
			)
			if err != nil {
				return fmt.Errorf("kind load docker-image: %w", err)
			}

			// 6. Deploy fake OPMS as K8s Deployment + ClusterIP Service
			if err = deployFakeOpms(ctx, kubeProvider, utils.PulumiDependsOn(kindLoadCmd)); err != nil {
				return fmt.Errorf("deployFakeOpms: %w", err)
			}

			// 7. Deploy Datadog agent via Helm, PAR enabled
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

			_ = buildCmd // ensure the build step is tracked by Pulumi
			return nil
		}, nil)

	p.SetDiagnoseFunc(awskubernetes.DiagnoseFunc)
	return p
}

// buildFakeOpmsLocally builds the fakeopms Docker image on the local machine and saves it
// as a tar archive at tarPath. Returns the build and save Pulumi resources.
func buildFakeOpmsLocally(ctx *pulumi.Context, e *aws.Environment, tarPath string) (pulumi.Resource, pulumi.Resource, error) {
	fakeOpmsDir := getFakeOpmsDir()

	// Use the e2e framework's LocalRunner so that Pulumi uses an explicit provider,
	// as default providers are disabled in the e2e framework.
	localRunner := command.NewLocalRunner(e, command.LocalRunnerArgs{
		OSCommand: command.NewUnixOSCommand(),
	})

	buildCmd, err := localRunner.Command("build-fakeopms", &command.Args{
		Create:  pulumi.Sprintf("docker build -t %s %s", fakeOpmsImageTag, fakeOpmsDir),
		Triggers: pulumi.Array{pulumi.String(fakeOpmsDir)},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("local docker build: %w", err)
	}

	saveCmd, err := localRunner.Command("save-fakeopms", &command.Args{
		Create: pulumi.Sprintf("docker save %s -o %s", fakeOpmsImageTag, tarPath),
	}, utils.PulumiDependsOn(buildCmd))
	if err != nil {
		return nil, nil, fmt.Errorf("local docker save: %w", err)
	}

	return buildCmd, saveCmd, nil
}

// shipFakeOpmsToEC2 copies the image tar to the EC2 VM and loads it into Docker.
func shipFakeOpmsToEC2(e *aws.Environment, vm *componentsremote.Host, tarPath string, opts ...pulumi.ResourceOption) ([]pulumi.Resource, error) {
	remoteTar := "/tmp/fakeopms-e2e.tar"

	copyCmd, err := vm.OS.FileManager().CopyFile("fakeopms-tar", pulumi.String(tarPath), pulumi.String(remoteTar), opts...)
	if err != nil {
		return nil, fmt.Errorf("copy fakeopms tar: %w", err)
	}

	loadCmd, err := vm.OS.Runner().Command(
		e.CommonNamer().ResourceName("docker-load-fakeopms"),
		&command.Args{
			Create: pulumi.Sprintf("docker load -i %s", remoteTar),
		},
		utils.PulumiDependsOn(copyCmd),
	)
	if err != nil {
		return nil, fmt.Errorf("docker load fakeopms: %w", err)
	}

	return []pulumi.Resource{loadCmd}, nil
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
							Image: pulumi.String(fakeOpmsImageTag),
							// Use image already loaded on the node; don't attempt to pull from registry.
							ImagePullPolicy: pulumi.String("Never"),
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

// getFakeOpmsDir returns the absolute path to the test/fakeopms/ directory.
// Uses runtime.Caller to locate the source tree from this file's location.
func getFakeOpmsDir() string {
	// This file is at: test/new-e2e/tests/privateactionrunner/provisioner.go
	// fakeopms is at:  test/fakeopms/
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "fakeopms")
}
