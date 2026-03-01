// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gensim

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	e2econfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	helmresource "github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	pulumiKubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

//go:embed gensim_runner.sh
var gensimRunnerScript string

// Run creates an EC2+Kind cluster and deploys a gensim episode with custom Datadog Agent
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// Get gensim-specific configuration
	cfg := config.New(ctx, "gensim")

	episodeName := cfg.Require("episodeName")         // e.g., "002_AWS_S3_Service_Disruption"
	episodeChartPath := cfg.Require("chartPath")      // Path to episode's Helm chart
	episodePath := cfg.Require("episodePath")         // Full path to episode dir (contains play-episode.sh)
	datadogValuesPath := cfg.Get("datadogValuesPath") // Path to datadog-values.yaml (optional)
	scenario := cfg.Get("scenario")                   // Scenario to run (empty = skip run)
	s3Bucket := cfg.Get("s3Bucket")
	namespace := cfg.Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	// Create EC2 VM for the Kind cluster
	host, err := ec2.NewVM(awsEnv, "gensim")
	if err != nil {
		return err
	}

	if err := host.Export(ctx, nil); err != nil {
		return err
	}

	// Attach an inline S3 write policy to the instance role so the runner can upload results.
	// Only created when s3Bucket is configured.
	if s3Bucket != "" {
		_, err = awsIam.NewRolePolicy(ctx, awsEnv.Namer.ResourceName("gensim-s3-upload"),
			&awsIam.RolePolicyArgs{
				Role: pulumi.String(awsEnv.DefaultInstanceProfileName()),
				Policy: pulumi.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:PutObject"],
    "Resource": "arn:aws:s3:::%s/*"
  }]
}`, s3Bucket),
			},
			awsEnv.WithProviders(e2econfig.ProviderAWS),
		)
		if err != nil {
			return err
		}
	}

	// Install ECR credentials helper to allow pulling images from ECR
	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
	if err != nil {
		return err
	}

	// Install tools needed by the runner: aws-cli (S3 upload), zip (archiving), jq (JSON parsing).
	// All in one command to avoid parallel apt-get lock contention.
	// snap install is idempotent via the fallback to snap refresh.
	installAwsCliCmd, err := host.OS.Runner().Command(
		awsEnv.Namer.ResourceName("install-aws-cli"),
		&command.Args{
			Create: pulumi.String(`snap install aws-cli --classic`),
			Sudo:   true,
		},
	)
	if err != nil {
		return err
	}

	// Install tools needed by the runner: aws-cli (S3 upload), zip (archiving), jq (JSON parsing).
	// All in one command to avoid parallel apt-get lock contention.
	// snap install is idempotent via the fallback to snap refresh.
	installToolsCmd, err := host.OS.Runner().Command(
		awsEnv.Namer.ResourceName("install-tools"),
		&command.Args{
			Create: pulumi.String(`apt-get install -y zip jq`),
			Sudo:   true,
		},
	)
	if err != nil {
		return err
	}

	// Create Kind cluster on the EC2 VM
	kindCluster, err := kubeComp.NewKindCluster(&awsEnv, host, "gensim", awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return err
	}

	if err := kindCluster.Export(ctx, nil); err != nil {
		return err
	}

	// Create Kubernetes provider from the Kind cluster kubeconfig
	kubeProvider, err := pulumiKubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &pulumiKubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            kindCluster.KubeConfig,
	})
	if err != nil {
		return err
	}

	// If the episode has custom Docker images, build them and load into Kind before deploying the chart.
	// Episodes with a docker-compose.yaml in their root directory define service images that must be
	// built locally on the VM and loaded into the Kind cluster before the Helm chart can reference them.
	var dependsOnImages pulumi.ResourceOption
	dockerComposePath := filepath.Join(episodePath, "docker-compose.yaml")
	if _, statErr := os.Stat(dockerComposePath); statErr == nil {
		// Copy services/ directory to a dedicated build staging area to avoid conflicting
		// with the /tmp/gensim-episode/ directory created later for the runner scripts.
		servicesCopyResources, err := host.OS.FileManager().CopyAbsoluteFolder(
			filepath.Join(episodePath, "services"), "/tmp/gensim-build/",
		)
		if err != nil {
			return err
		}

		// Copy docker-compose.yaml to the same staging area
		dockerComposeFile, err := host.OS.FileManager().CopyFile(
			"docker-compose-yaml",
			pulumi.String(dockerComposePath),
			pulumi.String("/tmp/gensim-build/docker-compose.yaml"),
			utils.PulumiDependsOn(servicesCopyResources...),
		)
		if err != nil {
			return err
		}

		// Chown so the SSH user owns the build directory for docker build output
		fixServicesPermsCmd, err := host.OS.Runner().Command(
			awsEnv.Namer.ResourceName("fix-services-dir-perms"),
			&command.Args{
				Create: pulumi.String("chown -R $(id -un):$(id -gn) /tmp/gensim-build/"),
				Sudo:   true,
			},
			utils.PulumiDependsOn(append(servicesCopyResources, dockerComposeFile)...),
		)
		if err != nil {
			return err
		}

		// Build custom images and load them into Kind.
		// Depends on kindCluster to ensure docker-compose plugin is installed and Kind is running.
		buildLoadImagesCmd, err := host.OS.Runner().Command(
			awsEnv.Namer.ResourceName("build-load-images"),
			&command.Args{
				Create: pulumi.Sprintf(
					`cd /tmp/gensim-build && docker-compose build && `+
						`docker-compose config --images | xargs -I{} kind load docker-image {} --name %s`,
					kindCluster.ClusterName,
				),
			},
			utils.PulumiDependsOn(fixServicesPermsCmd, kindCluster),
		)
		if err != nil {
			return err
		}

		dependsOnImages = utils.PulumiDependsOn(buildLoadImagesCmd)
	}

	// Deploy custom Datadog Agent DaemonSet with observer capabilities
	// This replaces the episode's basic agent Deployment
	var dependsOnDDAgent pulumi.ResourceOption

	if awsEnv.AgentDeploy() {
		k8sAgentOptions := make([]kubernetesagentparams.Option, 0)

		// Deploy to the same namespace as the episode so services can reach the agent
		k8sAgentOptions = append(
			k8sAgentOptions,
			kubernetesagentparams.WithNamespace(namespace),
		)

		// Handle fakeintake if needed
		if awsEnv.AgentUseFakeintake() {
			fakeIntakeOptions := []fakeintake.Option{fakeintake.WithLoadBalancer()}
			if awsEnv.AgentUseDualShipping() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithoutDDDevForwarding())
			}

			fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "gensim-fakeintake", fakeIntakeOptions...)
			if err != nil {
				return err
			}
			if err := fakeIntake.Export(ctx, nil); err != nil {
				return err
			}
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithFakeintake(fakeIntake))
		}

		// Load custom helm values from datadog-values.yaml if it exists
		// This includes observer configuration (DD_OBSERVER_* env vars)
		if datadogValuesPath != "" {
			valuesContent, err := os.ReadFile(datadogValuesPath)
			if err != nil {
				return err
			}
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithHelmValues(string(valuesContent)))
		}

		if awsEnv.AgentFullImagePath() != "" {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithAgentFullImagePath(awsEnv.AgentFullImagePath()))
		}

		if awsEnv.ClusterAgentFullImagePath() != "" {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithClusterAgentFullImagePath(awsEnv.ClusterAgentFullImagePath()))
		}

		k8sAgentComponent, err := helm.NewKubernetesAgent(&awsEnv, awsEnv.Namer.ResourceName("datadog-agent"), kubeProvider, k8sAgentOptions...)
		if err != nil {
			return err
		}

		if err := k8sAgentComponent.Export(ctx, nil); err != nil {
			return err
		}

		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentComponent)
	}

	// Deploy the episode's Helm chart
	// Helm release names must be lowercase alphanumeric with hyphens only, max 53 chars.
	sanitizedEpisodeName := strings.ToLower(strings.ReplaceAll(episodeName, "_", "-"))
	episodeReleaseName := fmt.Sprintf("gensim-%s", sanitizedEpisodeName)
	if len(episodeReleaseName) > 53 {
		episodeReleaseName = episodeReleaseName[:53]
	}

	episodeChart, err := helmresource.NewInstallation(&awsEnv, helmresource.InstallArgs{
		RepoURL:     "", // Local chart, no repo
		ChartName:   episodeChartPath,
		InstallName: episodeReleaseName,
		Namespace:   namespace,
		Values: pulumi.Map{
			"namespace": pulumi.String(namespace),
			"datadog": pulumi.Map{
				"apiKey": awsEnv.AgentAPIKey(),
				"appKey": awsEnv.AgentAPPKey(),
				"site":   pulumi.String(awsEnv.Site()),
				"env":    pulumi.String(episodeReleaseName),
			},
		},
	}, pulumi.Provider(kubeProvider), dependsOnDDAgent, dependsOnImages)

	if err != nil {
		return err
	}

	// The episode chart includes its own basic datadog-agent Deployment. Now that we deploy
	// the full DaemonSet-based agent, remove the episode's duplicate agent using
	// `docker exec` into the Kind control plane (which has kubectl pre-installed).
	if awsEnv.AgentDeploy() {
		_, err = host.OS.Runner().Command(
			awsEnv.Namer.ResourceName("delete-episode-agent"),
			&command.Args{
				Create: pulumi.Sprintf(
					"docker exec %s-control-plane kubectl delete deploy/datadog-agent svc/datadog-agent sa/datadog-agent --ignore-not-found=true -n %s",
					kindCluster.ClusterName, namespace,
				),
			},
			utils.PulumiDependsOn(episodeChart),
		)
		if err != nil {
			return err
		}
	}

	// Copy play-episode.sh and the episodes/ subdirectory to the VM
	// (the chart is already deployed via Helm; results/ is generated at runtime)
	playScript, err := host.OS.FileManager().CopyFile(
		"play-episode-sh",
		pulumi.String(filepath.Join(episodePath, "play-episode.sh")),
		pulumi.String("/tmp/gensim-episode/play-episode.sh"),
		utils.PulumiDependsOn(episodeChart),
	)
	if err != nil {
		return err
	}

	// CopyAbsoluteFolder preserves the base folder name, so passing "/tmp/gensim-episode/"
	// as remote root will produce /tmp/gensim-episode/episodes/<files>.
	episodesCopyResources, err := host.OS.FileManager().CopyAbsoluteFolder(
		filepath.Join(episodePath, "episodes"), "/tmp/gensim-episode/",
		utils.PulumiDependsOn(episodeChart),
	)
	if err != nil {
		return err
	}

	episodeCopyResources := append(episodesCopyResources, playScript)

	// All copied files are root-owned (MoveFile always uses sudo cp).
	// Chown the episode directory back to the SSH user so the runner can write into it
	// (e.g. play-episode.sh creates a results/ subdirectory).
	fixPermsCmd, err := host.OS.Runner().Command(
		awsEnv.Namer.ResourceName("fix-episode-dir-perms"),
		&command.Args{
			Create: pulumi.String("chown -R $(id -un):$(id -gn) /tmp/gensim-episode/"),
			Sudo:   true,
		},
		utils.PulumiDependsOn(episodeCopyResources...),
	)
	if err != nil {
		return err
	}

	// Write the embedded runner script to the VM
	runnerFile, err := host.OS.FileManager().CopyInlineFile(
		pulumi.String(gensimRunnerScript), "/tmp/gensim_runner.sh",
		utils.PulumiDependsOn(fixPermsCmd),
	)
	if err != nil {
		return err
	}

	// Write secrets to a separate env file (content is a Pulumi secret, redacted in CLI output)
	secretsContent := pulumi.Sprintf(
		"export DD_API_KEY=%s\nexport DD_APP_KEY=%s\n",
		awsEnv.AgentAPIKey(),
		awsEnv.AgentAPPKey(),
	)
	secretsFile, err := host.OS.FileManager().CopyInlineFile(
		secretsContent, "/tmp/gensim-secrets.env",
		utils.PulumiDependsOn(fixPermsCmd),
	)
	if err != nil {
		return err
	}

	// Start the runner in the background if a scenario was specified
	if scenario != "" {
		allCopyDeps := utils.PulumiDependsOn(append(episodeCopyResources, runnerFile, secretsFile, installAwsCliCmd, installToolsCmd)...)
		_, err = host.OS.Runner().Command(
			awsEnv.Namer.ResourceName("gensim-run-episode"),
			&command.Args{
				Create: pulumi.Sprintf(
					`nohup env `+
						`CLUSTER_NAME=%s `+
						`EPISODE_NAME=%s `+
						`SCENARIO=%s `+
						`KUBE_NAMESPACE=%s `+
						`DD_ENV=gensim-%s `+
						`DD_SITE=%s `+
						`S3_BUCKET=%s `+
						`bash /tmp/gensim_runner.sh > /tmp/gensim-runner-boot.log 2>&1 & `+
						`PID=$! && sleep 5 && `+
						`kill -0 $PID 2>/dev/null && echo "Runner PID: $PID" || `+
						`(echo "Runner failed within 5s, boot log:" && tail -30 /tmp/gensim-runner-boot.log && exit 1)`,
					kindCluster.ClusterName,
					episodeName,
					scenario,
					namespace,
					episodeName,
					awsEnv.Site(),
					s3Bucket,
				),
			},
			allCopyDeps,
		)
		if err != nil {
			return err
		}
	}

	// Export episode information
	ctx.Export("episode-name", pulumi.String(episodeName))
	ctx.Export("episode-namespace", pulumi.String(namespace))
	ctx.Export("episode-release", episodeChart.Status.Name())

	return nil
}
