// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gensimeks provisions an EKS cluster for running gensim episodes.
//
// Milestones:
//
//	M1 (✓): EKS cluster + kubeconfig export — no workloads.
//	M2 (✓): Episode services built on an EC2 build VM, pushed to ECR, deployed to EKS.
//	M3 (✓): Datadog Agent DaemonSet deployed; stub agent removed.
//	M4 (this file): Kubernetes Job running play-episode.sh autonomously.
//	M5: S3 results upload, destroy cleanup, full parity with Kind-based scenario.
package gensimeks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	e2econfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	osComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	eksscenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"

	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	pulumiKubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// Run is the Pulumi entry point for the aws/gensim-eks scenario.
// It is registered in registry/scenarios.go and invoked by the e2e-framework runner.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// ── Cluster ───────────────────────────────────────────────────────────────
	// WithLinuxNodeGroup is required: without it the cluster has only Fargate nodes
	// (used for system pods like CoreDNS) and episode workloads cannot be scheduled.
	// WithoutFargate: no Fargate nodes in this cluster. Fargate is used by default
	// for CoreDNS (provisioning speed), but the NoSchedule taint on Fargate nodes
	// causes DaemonSets (including the Datadog agent) to accumulate stuck-Pending
	// pods, blocking Helm readiness checks and adding operational complexity.
	// CoreDNS simply schedules on the EC2 node group once nodes join instead.
	cluster, err := eksscenario.NewCluster(awsEnv, "gensim",
		eksscenario.WithLinuxNodeGroup(),
		eksscenario.WithoutFargate(),
	)
	if err != nil {
		return err
	}
	if err := cluster.Export(ctx, nil); err != nil {
		return err
	}

	// ── Episode (M2+) ─────────────────────────────────────────────────────────
	// chartPath is only set when --episode is passed to the invoke task.
	// If absent, stop here — valid for M1 cluster-only runs.
	cfg := config.New(ctx, "gensim")
	chartPath := cfg.Get("chartPath")
	if chartPath == "" {
		return nil
	}

	episodeName := cfg.Require("episodeName")
	episodePath := cfg.Get("episodePath")     // full path to episode dir on the developer's machine
	imageRegistry := cfg.Get("imageRegistry") // ECR registry URL, computed by the invoke task
	namespace := cfg.Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	// ── Build VM (M2) ─────────────────────────────────────────────────────────
	// If the episode has custom service images (signalled by a docker-compose.yaml),
	// we build them on a dedicated EC2 VM rather than on the developer's laptop.
	//
	// Building on EC2 avoids two classes of problems:
	//   1. Cross-platform: Apple Silicon Macs build linux/arm64 by default; EKS nodes are x86_64.
	//   2. Credential friction: the instance IAM role authenticates to ECR automatically —
	//      no local credential setup or `aws sso login` required for the push.
	var dependsOnImages pulumi.ResourceOption
	dockerComposePath := filepath.Join(episodePath, "docker-compose.yaml")
	if _, statErr := os.Stat(dockerComposePath); statErr == nil {
		dependsOnImages, err = buildAndPushImages(ctx, awsEnv, episodePath, imageRegistry)
		if err != nil {
			return err
		}
	}

	// ── Kubernetes provider ───────────────────────────────────────────────────
	kubeProvider, err := pulumiKubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"),
		&pulumiKubernetes.ProviderArgs{
			EnableServerSideApply: pulumi.Bool(true),
			Kubeconfig:            cluster.KubeConfig,
		},
	)
	if err != nil {
		return err
	}

	// ── imagePullPolicy post-renderer ─────────────────────────────────────────
	// Episode charts hardcode imagePullPolicy: Never (a Kind artifact).
	// This script patches it to IfNotPresent so EKS nodes pull from ECR.
	patchScriptPath, err := writePatchScript()
	if err != nil {
		return err
	}

	// ── Episode Helm chart ────────────────────────────────────────────────────
	// Helm release names: lowercase alphanumeric + hyphens, max 53 chars.
	releaseName := "gensim-" + strings.ToLower(strings.ReplaceAll(episodeName, "_", "-"))
	if len(releaseName) > 53 {
		releaseName = releaseName[:53]
	}

	opts := []pulumi.ResourceOption{pulumi.Provider(kubeProvider)}
	if dependsOnImages != nil {
		opts = append(opts, dependsOnImages)
	}

	episodeRelease, err := helmv3.NewRelease(ctx, awsEnv.Namer.ResourceName("episode"),
		&helmv3.ReleaseArgs{
			Name:            pulumi.StringPtr(releaseName),
			Chart:           pulumi.String(chartPath),
			Namespace:       pulumi.StringPtr(namespace),
			CreateNamespace: pulumi.BoolPtr(true),
			Postrender:      pulumi.StringPtr(patchScriptPath),
			// Don't wait for all pods to become Ready. The episode chart includes a
			// built-in datadog-agent DaemonSet that tries to schedule on Fargate nodes
			// (which have NoSchedule taints) and will never become Ready. The episode
			// services we care about (svc-login, pgbouncer, etc.) start successfully.
			// M3 will delete this stub agent and deploy the proper DaemonSet-based one.
			SkipAwait: pulumi.BoolPtr(true),
			Values: pulumi.Map{
				// imageRegistry is prepended to all custom service images by the chart's _helpers.tpl.
				"imageRegistry": pulumi.String(imageRegistry),
				"namespace":     pulumi.String(namespace),
				// Pass credentials to the episode chart's built-in agent so it doesn't crash-loop.
				// M3 will replace it with a proper DaemonSet-based agent.
				"datadog": pulumi.Map{
					"apiKey": awsEnv.AgentAPIKey(),
					"appKey": awsEnv.AgentAPPKey(),
					"site":   pulumi.String(awsEnv.Site()),
					"env":    pulumi.String(releaseName),
				},
			},
		},
		opts...,
	)
	if err != nil {
		return err
	}

	ctx.Export("episode-release", pulumi.String(releaseName))

	// ── Datadog Agent (M3) ────────────────────────────────────────────────────
	// gated on ddagent:deploy=true, which is set by the invoke task when
	// install_agent=True (i.e. when --episode is passed).
	if awsEnv.AgentDeploy() {
		agentOpts := []kubernetesagentparams.Option{
			kubernetesagentparams.WithNamespace(namespace),
			// Wait for the episode chart to be deployed before the agent starts,
			// so the agent immediately sees the episode's pods and services.
			kubernetesagentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(episodeRelease)),
		}

		// Pass gensim-specific agent config via WithExtraHelmValues (pulumi.Map) rather
		// than WithHelmValues/WithHelmValuesFile (AssetOrArchiveArray). Map values flow
		// through the computed ToYAMLPulumiAssetOutput() path and survive local-backend
		// state round-trips correctly; asset values corrupt to []interface{} on update.
		agentOpts = append(agentOpts, kubernetesagentparams.WithExtraHelmValues(pulumi.Map{
			"datadog": pulumi.Map{
				// Required on EKS: kubelet uses a self-signed cert not trusted by the
				// agent's default CA bundle.
				"kubelet": pulumi.Map{
					"tlsVerify": pulumi.Bool(false),
				},
				"clusterName": pulumi.String("gensim"),
			},
		}))

		if awsEnv.AgentFullImagePath() != "" {
			agentOpts = append(agentOpts, kubernetesagentparams.WithAgentFullImagePath(awsEnv.AgentFullImagePath()))
		}

		_, err = helm.NewKubernetesAgent(&awsEnv, awsEnv.Namer.ResourceName("datadog-agent"), kubeProvider, agentOpts...)
		if err != nil {
			return err
		}
	}

	// ── Episode runner Job (M4) ──────────────────────────────────────────────
	// Runs play-episode.sh autonomously inside the cluster as a Kubernetes Job.
	// Only started when gensim:scenario is set; omitting it deploys infrastructure
	// only (useful for debugging before committing to a full run).
	scenario := cfg.Get("scenario")
	if scenario != "" && awsEnv.AgentDeploy() {
		if err := deployRunnerJob(ctx, &awsEnv, kubeProvider, episodePath, scenario, namespace, releaseName); err != nil {
			return err
		}
	}

	return nil
}

// deployRunnerJob creates the RBAC, Secret, ConfigMap, and Job needed to run
// play-episode.sh autonomously inside the cluster.
//
// The Job uses alpine/k8s which provides kubectl, bash, curl, and jq.
// play-episode.sh uses in-cluster config automatically (no explicit KUBECONFIG),
// authenticating via the gensim-runner ServiceAccount and ClusterRoleBinding.
func deployRunnerJob(
	ctx *pulumi.Context,
	awsEnv *resAws.Environment,
	kubeProvider *pulumiKubernetes.Provider,
	episodePath, scenario, namespace, ddEnv string,
) error {
	kubeOpts := []pulumi.ResourceOption{pulumi.Provider(kubeProvider)}

	// ── ServiceAccount ───────────────────────────────────────────────────────
	sa, err := corev1.NewServiceAccount(ctx, awsEnv.Namer.ResourceName("runner-sa"),
		&corev1.ServiceAccountArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("gensim-runner"),
				Namespace: pulumi.String(namespace),
			},
		}, kubeOpts...)
	if err != nil {
		return err
	}

	// ── ClusterRole ───────────────────────────────────────────────────────────
	// play-episode.sh requires:
	//   kubectl scale deployment   → update deployments/scale
	//   kubectl wait pods --for=condition=ready → get/list/watch pods
	clusterRole, err := rbacv1.NewClusterRole(ctx, awsEnv.Namer.ResourceName("runner-role"),
		&rbacv1.ClusterRoleArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.String("gensim-runner"),
			},
			Rules: rbacv1.PolicyRuleArray{
				rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.StringArray{pulumi.String("")},
					Resources: pulumi.StringArray{pulumi.String("pods")},
					Verbs:     pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("watch")},
				},
				rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.StringArray{pulumi.String("apps")},
					Resources: pulumi.StringArray{pulumi.String("deployments"), pulumi.String("deployments/scale")},
					Verbs:     pulumi.StringArray{pulumi.String("get"), pulumi.String("list"), pulumi.String("update")},
				},
			},
		}, kubeOpts...)
	if err != nil {
		return err
	}

	// ── ClusterRoleBinding ────────────────────────────────────────────────────
	_, err = rbacv1.NewClusterRoleBinding(ctx, awsEnv.Namer.ResourceName("runner-binding"),
		&rbacv1.ClusterRoleBindingArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.String("gensim-runner"),
			},
			RoleRef: rbacv1.RoleRefArgs{
				ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
				Kind:     pulumi.String("ClusterRole"),
				Name:     clusterRole.Metadata.Name().Elem(),
			},
			Subjects: rbacv1.SubjectArray{
				rbacv1.SubjectArgs{
					Kind:      pulumi.String("ServiceAccount"),
					Name:      sa.Metadata.Name().Elem(),
					Namespace: pulumi.String(namespace),
				},
			},
		}, kubeOpts...)
	if err != nil {
		return err
	}

	// ── Secret (DD credentials) ───────────────────────────────────────────────
	ddSecret, err := corev1.NewSecret(ctx, awsEnv.Namer.ResourceName("runner-secret"),
		&corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("gensim-secrets"),
				Namespace: pulumi.String(namespace),
			},
			StringData: pulumi.StringMap{
				"DD_API_KEY": awsEnv.AgentAPIKey(),
				"DD_APP_KEY": awsEnv.AgentAPPKey(),
			},
		}, kubeOpts...)
	if err != nil {
		return err
	}

	// ── ConfigMap (play-episode.sh + scenario YAML) ───────────────────────────
	// We only mount the specific scenario YAML rather than the whole episodes/
	// directory to keep the ConfigMap small.
	playScriptContent, err := os.ReadFile(filepath.Join(episodePath, "play-episode.sh"))
	if err != nil {
		return fmt.Errorf("reading play-episode.sh: %w", err)
	}
	scenarioContent, err := os.ReadFile(filepath.Join(episodePath, "episodes", scenario+".yaml"))
	if err != nil {
		return fmt.Errorf("reading episode scenario %q: %w", scenario, err)
	}

	configMap, err := corev1.NewConfigMap(ctx, awsEnv.Namer.ResourceName("runner-config"),
		&corev1.ConfigMapArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("gensim-episode"),
				Namespace: pulumi.String(namespace),
			},
			Data: pulumi.StringMap{
				"play-episode.sh":  pulumi.String(string(playScriptContent)),
				scenario + ".yaml": pulumi.String(string(scenarioContent)),
			},
		}, kubeOpts...)
	if err != nil {
		return err
	}

	// ── Job ───────────────────────────────────────────────────────────────────
	// alpine/k8s provides kubectl, bash, curl, and jq — everything play-episode.sh needs.
	// The Job uses in-cluster config (ServiceAccount token) so no KUBECONFIG is needed.
	// pulumi.com/skipAwait prevents Pulumi from waiting for Job completion (30-60 min).
	_, err = batchv1.NewJob(ctx, awsEnv.Namer.ResourceName("runner-job"),
		&batchv1.JobArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("gensim-runner"),
				Namespace: pulumi.String(namespace),
				Annotations: pulumi.StringMap{
					// Do not wait for the Job to complete — it runs for 30-60 min.
					// Pulumi returns as soon as the Job resource is created.
					"pulumi.com/skipAwait": pulumi.String("true"),
				},
			},
			Spec: batchv1.JobSpecArgs{
				BackoffLimit: pulumi.IntPtr(0), // no retries — a failed run is a failed run
				Template: corev1.PodTemplateSpecArgs{
					Spec: corev1.PodSpecArgs{
						ServiceAccountName: sa.Metadata.Name().Elem(),
						RestartPolicy:      pulumi.String("Never"),
						Containers: corev1.ContainerArray{
							corev1.ContainerArgs{
								Name:  pulumi.String("runner"),
								Image: pulumi.String("alpine/k8s:1.31.0"),
								Command: pulumi.StringArray{
									pulumi.String("bash"),
									pulumi.String("/episode/play-episode.sh"),
									pulumi.String("run-episode"),
									pulumi.String(scenario),
								},
								Env: corev1.EnvVarArray{
									corev1.EnvVarArgs{
										Name: pulumi.String("DD_API_KEY"),
										ValueFrom: &corev1.EnvVarSourceArgs{
											SecretKeyRef: &corev1.SecretKeySelectorArgs{
												Name: ddSecret.Metadata.Name().Elem(),
												Key:  pulumi.String("DD_API_KEY"),
											},
										},
									},
									corev1.EnvVarArgs{
										Name: pulumi.String("DD_APP_KEY"),
										ValueFrom: &corev1.EnvVarSourceArgs{
											SecretKeyRef: &corev1.SecretKeySelectorArgs{
												Name: ddSecret.Metadata.Name().Elem(),
												Key:  pulumi.String("DD_APP_KEY"),
											},
										},
									},
									corev1.EnvVarArgs{Name: pulumi.String("DD_ENV"), Value: pulumi.StringPtr(ddEnv)},
									corev1.EnvVarArgs{Name: pulumi.String("KUBE_NAMESPACE"), Value: pulumi.StringPtr(namespace)},
									corev1.EnvVarArgs{Name: pulumi.String("DD_SITE"), Value: pulumi.StringPtr(awsEnv.Site())},
									// play-episode.sh creates results/ relative to its script dir.
									// ConfigMap volumes are read-only, so redirect to a writable path.
									corev1.EnvVarArgs{Name: pulumi.String("RESULTS_DIR"), Value: pulumi.StringPtr("/tmp/results")},
								},
								VolumeMounts: corev1.VolumeMountArray{
									corev1.VolumeMountArgs{
										Name:      pulumi.String("episode-script"),
										MountPath: pulumi.String("/episode"),
									},
									corev1.VolumeMountArgs{
										Name:      pulumi.String("episode-scenarios"),
										MountPath: pulumi.String("/episode/episodes"),
									},
								},
							},
						},
						Volumes: corev1.VolumeArray{
							// play-episode.sh → /episode/play-episode.sh
							corev1.VolumeArgs{
								Name: pulumi.String("episode-script"),
								ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
									Name: configMap.Metadata.Name().Elem(),
									Items: corev1.KeyToPathArray{
										corev1.KeyToPathArgs{
											Key:  pulumi.String("play-episode.sh"),
											Path: pulumi.String("play-episode.sh"),
											Mode: pulumi.IntPtr(0o755),
										},
									},
								},
							},
							// <scenario>.yaml → /episode/episodes/<scenario>.yaml
							corev1.VolumeArgs{
								Name: pulumi.String("episode-scenarios"),
								ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
									Name: configMap.Metadata.Name().Elem(),
									Items: corev1.KeyToPathArray{
										corev1.KeyToPathArgs{
											Key:  pulumi.String(scenario + ".yaml"),
											Path: pulumi.String(scenario + ".yaml"),
										},
									},
								},
							},
						},
					},
				},
			},
		}, kubeOpts...)
	return err
}

// buildAndPushImages provisions a small EC2 build VM, copies the episode's service
// source code to it, builds Docker images for linux/amd64, and pushes them to ECR.
//
// The VM is kept in the Pulumi stack and destroyed when the stack is destroyed.
// Its only purpose is image building — it is not used for running the episode.
func buildAndPushImages(ctx *pulumi.Context, awsEnv resAws.Environment, episodePath, ecrRegistry string) (pulumi.ResourceOption, error) {
	// Grant the build VM's instance role permission to push images to ECR.
	// GetAuthorizationToken is account-level and must use "*" as the resource.
	_, err := awsIam.NewRolePolicy(ctx, awsEnv.Namer.ResourceName("gensim-ecr-push"),
		&awsIam.RolePolicyArgs{
			Role: pulumi.String(awsEnv.DefaultInstanceProfileName()),
			Policy: pulumi.String(`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "ecr:GetAuthorizationToken",
      "ecr:CreateRepository",
      "ecr:DescribeImages",
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:PutImage",
      "ecr:InitiateLayerUpload",
      "ecr:UploadLayerPart",
      "ecr:CompleteLayerUpload"
    ],
    "Resource": "*"
  }]
}`),
		},
		awsEnv.WithProviders(e2econfig.ProviderAWS),
	)
	if err != nil {
		return nil, err
	}

	// Use the Amazon Linux ECS AMI — Docker 25 (including buildx) is pre-installed
	// and the daemon is already running. Only AWS CLI needs to be added (~30s via yum).
	buildHost, err := ec2.NewVM(awsEnv, "gensim-builder", ec2.WithOS(osComp.AmazonLinuxECSDefault))
	if err != nil {
		return nil, err
	}

	// Install AWS CLI — Docker 25 (including buildx) is pre-installed on the ECS AMI,
	// but AWS CLI is not. This is the only setup step needed (~30s vs ~4 min on Ubuntu).
	installToolsCmd, err := buildHost.OS.Runner().Command(
		awsEnv.Namer.ResourceName("install-build-tools"),
		&command.Args{
			Create: pulumi.String(`yum install -y awscli`),
			Sudo:   true,
		},
	)
	if err != nil {
		return nil, err
	}

	// Copy the episode's docker-compose.yaml and services/ directory to the build VM.
	servicesCopy, err := buildHost.OS.FileManager().CopyAbsoluteFolder(
		filepath.Join(episodePath, "services"), "/tmp/gensim-build/",
	)
	if err != nil {
		return nil, err
	}

	dockerComposeCopy, err := buildHost.OS.FileManager().CopyFile(
		"docker-compose-yaml",
		pulumi.String(filepath.Join(episodePath, "docker-compose.yaml")),
		pulumi.String("/tmp/gensim-build/docker-compose.yaml"),
		utils.PulumiDependsOn(servicesCopy...),
	)
	if err != nil {
		return nil, err
	}

	// Fix ownership so the build directory is writable by the SSH user.
	fixPermsCmd, err := buildHost.OS.Runner().Command(
		awsEnv.Namer.ResourceName("fix-build-dir-perms"),
		&command.Args{
			Create: pulumi.String(`chown -R $(id -un):$(id -gn) /tmp/gensim-build/`),
			Sudo:   true,
		},
		utils.PulumiDependsOn(append(servicesCopy, dockerComposeCopy)...),
	)
	if err != nil {
		return nil, err
	}

	// Compute a hash of the local services directory so Pulumi can detect when
	// source files change. Without this Trigger, DependsOn only controls ordering
	// and the build command would never re-run after the initial create, even if
	// app.py or a Dockerfile changed.
	servicesHash, err := hashDir(filepath.Join(episodePath, "services"))
	if err != nil {
		return nil, fmt.Errorf("hashing services directory: %w", err)
	}

	// Build all images and push to ECR.
	//
	// Caching: images are also tagged with the first 12 chars of servicesHash
	// (e.g. "abc123def456"). On re-runs — including fresh cluster recreations with
	// unchanged source — we check ECR for this hash tag first. If all images are
	// already present, we skip the docker-compose build entirely (saves 5-10 min).
	//
	// TODO: the e2e infra-cleaner job purges ECR repositories weekly, which
	// defeats this cache between weeks. To make the cache durable, the ECR repos
	// need to be tagged so the cleaner skips them (e.g. a "gensim:keep" tag or
	// an exclusion rule in the cleaner config). Until then, the cache only helps
	// within the same week or on same-day cluster recreations.
	//
	// The :latest tag is always pushed so the Helm chart's imageRegistry prefix works
	// without any changes to chart values.
	buildAndPush, err := buildHost.OS.Runner().Command(
		awsEnv.Namer.ResourceName("build-push-images"),
		&command.Args{
			// Triggers forces this command to re-run whenever the source files change.
			// Pulumi replaces the resource when any trigger value differs from state.
			Triggers: pulumi.Array{pulumi.String(servicesHash)},
			Create: pulumi.Sprintf(
				`set -euo pipefail

# AWS CLI is installed on the ECS AMI but not in the default SSH PATH.
export PATH=$PATH:/usr/bin:/usr/local/bin

cd /tmp/gensim-build

REGION="%s"
ECR_REGISTRY="%s"
CACHE_TAG="%s"  # first 12 chars of services hash — stable cache key

# Authenticate Docker with ECR using the instance IAM role.
# --password-stdin works without a TTY, which is how Pulumi executes remote commands.
aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "$ECR_REGISTRY"

# Parse image names from docker-compose.yaml via Python (no docker-compose needed).
IMAGES=$(python3 -c "
import yaml
with open('docker-compose.yaml') as f:
    c = yaml.safe_load(f)
for svc in c['services'].values():
    if 'image' in svc:
        print(svc['image'])
")

# Check if all images are already cached in ECR under the hash tag.
# On a fresh cluster with unchanged source code this saves the full build.
ALL_CACHED=true
for IMAGE in $IMAGES; do
  REPO="${IMAGE%%:*}"
  if ! aws ecr describe-images \
      --repository-name "$REPO" \
      --image-ids "imageTag=$CACHE_TAG" \
      --region "$REGION" >/dev/null 2>&1; then
    ALL_CACHED=false
    break
  fi
done

if [ "$ALL_CACHED" = "true" ]; then
  echo "[cache hit] All images cached at $CACHE_TAG — pulling and retagging as :latest"
  for IMAGE in $IMAGES; do
    REPO="${IMAGE%%:*}"
    docker pull "$ECR_REGISTRY/$REPO:$CACHE_TAG"
    docker tag  "$ECR_REGISTRY/$REPO:$CACHE_TAG" "$ECR_REGISTRY/$IMAGE"
    docker push "$ECR_REGISTRY/$IMAGE"
  done
else
  echo "[cache miss] Building images with docker buildx bake..."
  # buildx bake understands docker-compose.yaml natively, builds all images
  # in parallel, and deduplicates shared base layers.
  docker buildx bake -f docker-compose.yaml

  for IMAGE in $IMAGES; do
    REPO="${IMAGE%%:*}"
    aws ecr create-repository --repository-name "$REPO" --region "$REGION" 2>/dev/null || true
    # :latest — consumed by the Helm chart via imageRegistry
    docker tag  "$IMAGE" "$ECR_REGISTRY/$IMAGE"
    docker push "$ECR_REGISTRY/$IMAGE"
    # :hash — cache key for future cluster recreations
    docker tag  "$IMAGE" "$ECR_REGISTRY/$REPO:$CACHE_TAG"
    docker push "$ECR_REGISTRY/$REPO:$CACHE_TAG"
  done
fi`,
				awsEnv.Region(),
				ecrRegistry,
				servicesHash[:12],
			),
		},
		utils.PulumiDependsOn(installToolsCmd, fixPermsCmd),
	)
	if err != nil {
		return nil, err
	}

	return utils.PulumiDependsOn(buildAndPush), nil
}

// hashDir computes a deterministic SHA256 hash of all files under a directory.
// It walks the tree, sorts paths for determinism, and hashes each file's
// contents. The result changes whenever any file is added, removed, or modified.
func hashDir(root string) (string, error) {
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, path := range paths {
		// Include the relative path so renames/moves are detected.
		rel, _ := filepath.Rel(root, path)
		fmt.Fprintf(h, "%s\n", rel)

		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writePatchScript writes a shell post-renderer that rewrites
// `imagePullPolicy: Never` → `imagePullPolicy: IfNotPresent` in Helm output.
// It is passed to helm as a post-renderer so the patch is transparent to the chart.
func writePatchScript() (string, error) {
	f, err := os.CreateTemp("", "patch-imagepullpolicy-*.sh")
	if err != nil {
		return "", fmt.Errorf("creating post-renderer script: %w", err)
	}
	defer f.Close()

	// Always pull from ECR on pod start. This ensures iterative redeploys
	// (with a mutable :latest tag) pick up the newly pushed image rather than
	// using a stale node-local cache. IfNotPresent would silently keep old code.
	script := "#!/bin/sh\nexec sed 's/imagePullPolicy: Never/imagePullPolicy: Always/g'\n"
	if _, err := f.WriteString(script); err != nil {
		return "", fmt.Errorf("writing post-renderer script: %w", err)
	}
	if err := os.Chmod(f.Name(), 0o755); err != nil {
		return "", fmt.Errorf("chmod post-renderer script: %w", err)
	}
	return f.Name(), nil
}
