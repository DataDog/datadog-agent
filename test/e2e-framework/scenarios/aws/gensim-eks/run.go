// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gensimeks provisions a persistent EKS cluster and deploys an
// orchestrator Job that drives gensim episode execution.
//
// Architecture:
//
//	Persistent layer (always created by Pulumi):
//	  - EKS cluster with EC2 node group
//	  - Kubernetes provider
//	  - ServiceAccount, ClusterRoleBinding, Secret
//	  - S3 IAM policy (when s3Bucket is set)
//
//	Orchestrator Job (created when episodes are submitted):
//	  - Per-episode ConfigMaps (play-episode.sh + scenario YAML)
//	  - Post-renderer ConfigMap
//	  - gensim-orchestrator Job (alpine/k8s)
package gensimeks

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	e2econfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	eksscenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"

	awsIam "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	pulumiKubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
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

	// ── Config ───────────────────────────────────────────────────────────────
	cfg := config.New(ctx, "gensim")
	episodes := cfg.Get("episodes")             // comma-separated "ep1:scen1,ep2:scen2"
	agentImage := cfg.Get("agentImage")         // full agent Docker image path
	gensimSha := cfg.Get("gensimSha")           // gensim-episodes git SHA
	s3Bucket := cfg.Get("s3Bucket")             // optional S3 bucket
	imageRegistry := cfg.Get("imageRegistry")   // ECR registry URL
	episodeDataDir := cfg.Get("episodeDataDir") // local path to postmortems directory
	namespace := cfg.Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	// If no episodes are specified, stop here — cluster-only mode.
	if episodes == "" {
		return nil
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

	// ── Persistent resources ─────────────────────────────────────────────────
	sa, ddSecret, err := deployPersistentResources(ctx, &awsEnv, kubeProvider, namespace)
	if err != nil {
		return err
	}

	// ── S3 IAM policy ────────────────────────────────────────────────────────
	// Attach write access to the EKS Linux node role so pods can push results.
	if s3Bucket != "" {
		_, err = awsIam.NewRolePolicy(ctx, awsEnv.Namer.ResourceName("gensim-s3-upload"),
			&awsIam.RolePolicyArgs{
				Role: awsEnv.CommonNamer().DisplayName(64, pulumi.String("eks-linux-node-role")),
				Policy: pulumi.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:PutObject", "s3:GetObject", "s3:ListBucket"],
    "Resource": ["arn:aws:s3:::%s", "arn:aws:s3:::%s/*"]
  }]
}`, s3Bucket, s3Bucket),
			},
			awsEnv.WithProviders(e2econfig.ProviderAWS),
		)
		if err != nil {
			return err
		}
	}

	// ── Orchestrator Job ─────────────────────────────────────────────────────
	if err := deployOrchestratorJob(
		ctx, &awsEnv, kubeProvider, sa, ddSecret,
		episodes, agentImage, gensimSha, namespace, s3Bucket, imageRegistry, episodeDataDir,
	); err != nil {
		return err
	}

	return nil
}

// deployPersistentResources creates the ServiceAccount, ClusterRoleBinding
// (to the built-in cluster-admin ClusterRole), and Secret that the orchestrator
// Job and future components need.
func deployPersistentResources(
	ctx *pulumi.Context,
	awsEnv *resAws.Environment,
	kubeProvider *pulumiKubernetes.Provider,
	namespace string,
) (sa *corev1.ServiceAccount, ddSecret *corev1.Secret, err error) {
	kubeOpts := []pulumi.ResourceOption{pulumi.Provider(kubeProvider)}

	// ── ServiceAccount ───────────────────────────────────────────────────────
	sa, err = corev1.NewServiceAccount(ctx, awsEnv.Namer.ResourceName("runner-sa"),
		&corev1.ServiceAccountArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("gensim-orchestrator"),
				Namespace: pulumi.String(namespace),
			},
		}, kubeOpts...)
	if err != nil {
		return nil, nil, err
	}

	// ── ClusterRoleBinding ────────────────────────────────────────────────────
	// The orchestrator needs cluster-admin because it helm-installs charts that
	// create arbitrary resource types (CRDs, RBAC, etc.).
	_, err = rbacv1.NewClusterRoleBinding(ctx, awsEnv.Namer.ResourceName("runner-binding"),
		&rbacv1.ClusterRoleBindingArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.String("gensim-orchestrator"),
			},
			RoleRef: rbacv1.RoleRefArgs{
				ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
				Kind:     pulumi.String("ClusterRole"),
				Name:     pulumi.String("cluster-admin"),
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
		return nil, nil, err
	}

	// ── Secret (DD credentials) ───────────────────────────────────────────────
	ddSecret, err = corev1.NewSecret(ctx, awsEnv.Namer.ResourceName("runner-secret"),
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
		return nil, nil, err
	}

	return sa, ddSecret, nil
}

// deployOrchestratorJob creates per-episode ConfigMaps, a post-renderer ConfigMap,
// and the orchestrator Job that drives episode execution inside the cluster.
func deployOrchestratorJob(
	ctx *pulumi.Context,
	awsEnv *resAws.Environment,
	kubeProvider *pulumiKubernetes.Provider,
	sa *corev1.ServiceAccount,
	ddSecret *corev1.Secret,
	episodes string,
	agentImage string,
	gensimSha string,
	namespace string,
	s3Bucket string,
	imageRegistry string,
	episodeDataDir string,
) error {
	kubeOpts := []pulumi.ResourceOption{pulumi.Provider(kubeProvider)}

	// Parse episode:scenario pairs.
	pairs := strings.Split(episodes, ",")
	type epPair struct {
		episode  string
		scenario string
	}
	var parsed []epPair
	for _, p := range pairs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		parts := strings.SplitN(p, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid episode:scenario pair %q (expected episode:scenario)", p)
		}
		parsed = append(parsed, epPair{episode: parts[0], scenario: parts[1]})
	}

	// ── Per-episode ConfigMaps ──────────────────────────────────────────────
	var episodeVolumes corev1.VolumeArray
	var episodeVolumeMounts corev1.VolumeMountArray
	var episodeConfigMaps []pulumi.Resource

	for _, ep := range parsed {
		playScriptContent, err := os.ReadFile(filepath.Join(episodeDataDir, ep.episode, "play-episode.sh"))
		if err != nil {
			return fmt.Errorf("reading play-episode.sh for episode %q: %w", ep.episode, err)
		}
		scenarioContent, err := os.ReadFile(filepath.Join(episodeDataDir, ep.episode, "episodes", ep.scenario+".yaml"))
		if err != nil {
			return fmt.Errorf("reading scenario %q for episode %q: %w", ep.scenario, ep.episode, err)
		}

		// Create chart tarball from the episode's chart/ directory.
		chartDir := filepath.Join(episodeDataDir, ep.episode, "chart")
		chartTarball, err := createTarGz(chartDir)
		if err != nil {
			return fmt.Errorf("creating chart tarball for episode %q: %w", ep.episode, err)
		}
		if len(chartTarball) > 500*1024 {
			fmt.Printf("WARNING: chart tarball for episode %q is %d bytes (>500KB); ConfigMap limit is 1MiB total\n", ep.episode, len(chartTarball))
		}
		chartTarballB64 := base64.StdEncoding.EncodeToString(chartTarball)

		cmName := "gensim-ep-" + ep.episode
		volName := "ep-" + ep.episode

		cm, err := corev1.NewConfigMap(ctx, awsEnv.Namer.ResourceName("ep-cm-"+ep.episode),
			&corev1.ConfigMapArgs{
				Metadata: metav1.ObjectMetaArgs{
					Name:      pulumi.String(cmName),
					Namespace: pulumi.String(namespace),
				},
				Data: pulumi.StringMap{
					"play-episode.sh":     pulumi.String(string(playScriptContent)),
					ep.scenario + ".yaml": pulumi.String(string(scenarioContent)),
				},
				BinaryData: pulumi.StringMap{
					"chart.tar.gz": pulumi.String(chartTarballB64),
				},
			}, kubeOpts...)
		if err != nil {
			return err
		}
		episodeConfigMaps = append(episodeConfigMaps, cm)

		episodeVolumes = append(episodeVolumes, corev1.VolumeArgs{
			Name: pulumi.String(volName),
			ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
				Name: pulumi.String(cmName),
			},
		})
		episodeVolumeMounts = append(episodeVolumeMounts, corev1.VolumeMountArgs{
			Name:      pulumi.String(volName),
			MountPath: pulumi.String("/episodes/" + ep.episode),
		})
	}

	// ── Post-renderer ConfigMap ──────────────────────────────────────────────
	// Pure awk implementation: no python3 dependency (alpine/k8s doesn't ship it).
	// Reads YAML documents separated by "---", drops stub agent resources
	// (DaemonSet/ServiceAccount/ClusterRole/ClusterRoleBinding named datadog-agent),
	// and patches imagePullPolicy: Never -> Always.
	postRendererScript := `#!/bin/sh
# Post-renderer: patch imagePullPolicy and strip stub agent resources.
# Helm pipes the rendered manifests through stdin; we write to stdout.
awk '
BEGIN { doc=""; first=1 }
/^---$/ {
  if (doc != "") { process_doc(doc) }
  doc = ""
  next
}
{ doc = doc (doc=="" ? "" : "\n") $0 }
END { if (doc != "") process_doc(doc) }

function process_doc(d) {
  # Drop stub agent resources.
  # Match name: datadog-agent followed by newline (not $, which only anchors
  # to end-of-string in awk, not end-of-line within a variable).
  if (d ~ /kind:[[:space:]]*(DaemonSet|ServiceAccount|ClusterRole|ClusterRoleBinding)/ &&
      (d ~ /name:[[:space:]]*datadog-agent[[:space:]]*\n/ || d ~ /name:[[:space:]]*datadog-agent[[:space:]]*$/)) {
    return
  }
  # Patch imagePullPolicy
  gsub(/imagePullPolicy: Never/, "imagePullPolicy: Always", d)
  if (!first) printf "\n---\n"
  printf "%s", d
  first = 0
}
'
`
	postRendererCM, err := corev1.NewConfigMap(ctx, awsEnv.Namer.ResourceName("post-renderer-cm"),
		&corev1.ConfigMapArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("gensim-post-renderer"),
				Namespace: pulumi.String(namespace),
			},
			Data: pulumi.StringMap{
				"post-renderer.sh": pulumi.String(postRendererScript),
			},
		}, kubeOpts...)
	if err != nil {
		return err
	}

	// ── Volumes ──────────────────────────────────────────────────────────────
	// Post-renderer volume
	episodeVolumes = append(episodeVolumes, corev1.VolumeArgs{
		Name: pulumi.String("post-renderer"),
		ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
			Name: postRendererCM.Metadata.Name().Elem(),
			Items: corev1.KeyToPathArray{
				corev1.KeyToPathArgs{
					Key:  pulumi.String("post-renderer.sh"),
					Path: pulumi.String("post-renderer.sh"),
					Mode: pulumi.IntPtr(0o755),
				},
			},
		},
	})
	// Workspace emptyDir
	episodeVolumes = append(episodeVolumes, corev1.VolumeArgs{
		Name:     pulumi.String("workspace"),
		EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
	})

	// ── Volume mounts ────────────────────────────────────────────────────────
	episodeVolumeMounts = append(episodeVolumeMounts, corev1.VolumeMountArgs{
		Name:      pulumi.String("post-renderer"),
		MountPath: pulumi.String("/scripts/post-renderer.sh"),
		SubPath:   pulumi.String("post-renderer.sh"),
	})
	episodeVolumeMounts = append(episodeVolumeMounts, corev1.VolumeMountArgs{
		Name:      pulumi.String("workspace"),
		MountPath: pulumi.String("/workspace"),
	})

	// ── Job dependencies ─────────────────────────────────────────────────────
	var jobDeps []pulumi.Resource
	jobDeps = append(jobDeps, sa, ddSecret, postRendererCM)
	jobDeps = append(jobDeps, episodeConfigMaps...)

	// ── Orchestrator Job ─────────────────────────────────────────────────────
	_, err = batchv1.NewJob(ctx, awsEnv.Namer.ResourceName("orchestrator-job"),
		&batchv1.JobArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("gensim-orchestrator"),
				Namespace: pulumi.String(namespace),
				Annotations: pulumi.StringMap{
					"pulumi.com/skipAwait": pulumi.String("true"),
				},
			},
			Spec: batchv1.JobSpecArgs{
				BackoffLimit: pulumi.IntPtr(0),
				Template: corev1.PodTemplateSpecArgs{
					Spec: corev1.PodSpecArgs{
						ServiceAccountName: sa.Metadata.Name().Elem(),
						RestartPolicy:      pulumi.String("Never"),
						Containers: corev1.ContainerArray{
							corev1.ContainerArgs{
								Name:    pulumi.String("orchestrator"),
								Image:   pulumi.String("alpine/k8s:1.31.0"),
								Command: pulumi.StringArray{pulumi.String("bash"), pulumi.String("-c")},
								Args:    pulumi.StringArray{pulumi.String(buildOrchestratorScript(episodes, agentImage, gensimSha, namespace, s3Bucket, imageRegistry))},
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
									corev1.EnvVarArgs{Name: pulumi.String("DD_SITE"), Value: pulumi.StringPtr(awsEnv.Site())},
									corev1.EnvVarArgs{Name: pulumi.String("EPISODES"), Value: pulumi.StringPtr(episodes)},
									corev1.EnvVarArgs{Name: pulumi.String("AGENT_IMAGE"), Value: pulumi.StringPtr(agentImage)},
									corev1.EnvVarArgs{Name: pulumi.String("GENSIM_SHA"), Value: pulumi.StringPtr(gensimSha)},
									corev1.EnvVarArgs{Name: pulumi.String("S3_BUCKET"), Value: pulumi.StringPtr(s3Bucket)},
									corev1.EnvVarArgs{Name: pulumi.String("KUBE_NAMESPACE"), Value: pulumi.StringPtr(namespace)},
									corev1.EnvVarArgs{Name: pulumi.String("IMAGE_REGISTRY"), Value: pulumi.StringPtr(imageRegistry)},
								},
								VolumeMounts: episodeVolumeMounts,
							},
						},
						Volumes: episodeVolumes,
					},
				},
			},
		},
		append(kubeOpts, utils.PulumiDependsOn(jobDeps...))...,
	)
	return err
}

// buildOrchestratorScript constructs the bash script that the orchestrator Job executes.
// All values come from environment variables set on the Job's env (EPISODES, AGENT_IMAGE,
// GENSIM_SHA, KUBE_NAMESPACE, S3_BUCKET, IMAGE_REGISTRY, DD_API_KEY, DD_APP_KEY, DD_SITE).
func buildOrchestratorScript(episodes, agentImage, gensimSha, namespace, s3Bucket, imageRegistry string) string {
	return "set -euo pipefail\n" +
		"apk add --no-cache aws-cli 2>/dev/null || true\n" +
		"helm repo add datadog https://helm.datadoghq.com && helm repo update\n" +
		"\n" +
		"# ── Parse image into repo + tag ──────────────────────────────────────────\n" +
		"IMAGE_REPO=\"${AGENT_IMAGE%:*}\"\n" +
		"IMAGE_TAG=\"${AGENT_IMAGE##*:}\"\n" +
		"RUN_ID=\"eval-$(date -u +%Y%m%d)-${GENSIM_SHA:0:7}\"\n" +
		"\n" +
		"echo \"Orchestrator starting\"\n" +
		"echo \"  Run ID:     $RUN_ID\"\n" +
		"echo \"  Episodes:   $EPISODES\"\n" +
		"echo \"  Image:      $AGENT_IMAGE\"\n" +
		"echo \"  Gensim SHA: $GENSIM_SHA\"\n" +
		"echo \"  S3 Bucket:  $S3_BUCKET\"\n" +
		"echo \"  Namespace:  $KUBE_NAMESPACE\"\n" +
		"\n" +
		"# ── Status ConfigMap helpers ────────────────────────────────────────────\n" +
		"init_status() {\n" +
		"  local episodes_json=\"[]\"\n" +
		"  IFS=',' read -ra _EP_INIT <<< \"$EPISODES\"\n" +
		"  for _EP in \"${_EP_INIT[@]}\"; do\n" +
		"    local _episode=\"${_EP%%:*}\"\n" +
		"    local _scenario=\"${_EP##*:}\"\n" +
		"    episodes_json=$(printf '%s' \"$episodes_json\" | jq -c --arg ep \"$_episode\" --arg sc \"$_scenario\" '. + [{\"episode\":$ep,\"scenario\":$sc,\"status\":\"queued\"}]')\n" +
		"  done\n" +
		"  local status_json\n" +
		"  status_json=$(jq -n -c \\\n" +
		"    --arg runId \"$RUN_ID\" \\\n" +
		"    --arg image \"$AGENT_IMAGE\" \\\n" +
		"    --arg sha \"$GENSIM_SHA\" \\\n" +
		"    --arg started \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\" \\\n" +
		"    --argjson eps \"$episodes_json\" \\\n" +
		"    '{runId:$runId,image:$image,gensimSha:$sha,startedAt:$started,episodes:$eps}')\n" +
		"  kubectl create configmap gensim-run-status \\\n" +
		"    --from-literal=status=\"$status_json\" \\\n" +
		"    -n \"$KUBE_NAMESPACE\" \\\n" +
		"    --dry-run=client -o yaml | kubectl apply -f -\n" +
		"}\n" +
		"\n" +
		"update_episode_status() {\n" +
		"  local ep_spec=\"$1\" new_status=\"$2\" extra=\"${3:-{}}\"\n" +
		"  local ep=\"${ep_spec%%:*}\"\n" +
		"  local sc=\"${ep_spec##*:}\"\n" +
		"  local current\n" +
		"  current=$(kubectl get configmap gensim-run-status -n \"$KUBE_NAMESPACE\" -o jsonpath='{.data.status}')\n" +
		"  local updated\n" +
		"  updated=$(printf '%s' \"$current\" | jq -c \\\n" +
		"    --arg ep \"$ep\" --arg sc \"$sc\" --arg st \"$new_status\" --argjson ex \"$extra\" \\\n" +
		"    '(.episodes[] | select(.episode==$ep and .scenario==$sc)) |= (. + $ex + {status:$st})')\n" +
		"  kubectl create configmap gensim-run-status \\\n" +
		"    --from-literal=status=\"$updated\" \\\n" +
		"    -n \"$KUBE_NAMESPACE\" \\\n" +
		"    --dry-run=client -o yaml | kubectl apply -f -\n" +
		"}\n" +
		"\n" +
		"set_run_complete() {\n" +
		"  local current\n" +
		"  current=$(kubectl get configmap gensim-run-status -n \"$KUBE_NAMESPACE\" -o jsonpath='{.data.status}')\n" +
		"  local updated\n" +
		"  updated=$(printf '%s' \"$current\" | jq -c --arg t \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\" '. + {completedAt:$t}')\n" +
		"  kubectl create configmap gensim-run-status \\\n" +
		"    --from-literal=status=\"$updated\" \\\n" +
		"    -n \"$KUBE_NAMESPACE\" \\\n" +
		"    --dry-run=client -o yaml | kubectl apply -f -\n" +
		"}\n" +
		"\n" +
		"# ── DD event + metrics helpers (Phase 4) ────────────────────────────────\n" +
		"emit_dd_event() {\n" +
		"  local episode=$1 scenario=$2 duration=$3 parquet_count=$4 outcome=$5\n" +
		"  local title=\"gensim: ${episode}/${scenario} ${outcome}\"\n" +
		"  local text=\"Duration: ${duration}s, Parquet files: ${parquet_count}, Image: ${AGENT_IMAGE}, SHA: ${GENSIM_SHA}\"\n" +
		"  local alert_type=\"success\"\n" +
		"  [ \"$outcome\" != \"success\" ] && alert_type=\"error\"\n" +
		"  curl -s -X POST \"https://api.${DD_SITE}/api/v1/events\" \\\n" +
		"    -H \"Content-Type: application/json\" \\\n" +
		"    -H \"DD-API-KEY: ${DD_API_KEY}\" \\\n" +
		"    -d \"{ \\\n" +
		"      \\\"title\\\": \\\"${title}\\\", \\\n" +
		"      \\\"text\\\": \\\"${text}\\\", \\\n" +
		"      \\\"alert_type\\\": \\\"${alert_type}\\\", \\\n" +
		"      \\\"tags\\\": [ \\\n" +
		"        \\\"episode:${episode}\\\", \\\n" +
		"        \\\"scenario:${scenario}\\\", \\\n" +
		"        \\\"image:${IMAGE_TAG}\\\", \\\n" +
		"        \\\"gensim_sha:${GENSIM_SHA}\\\", \\\n" +
		"        \\\"run_id:${RUN_ID}\\\" \\\n" +
		"      ] \\\n" +
		"    }\" || echo \"WARN: Failed to emit DD event\" >&2\n" +
		"}\n" +
		"\n" +
		"emit_dd_metrics() {\n" +
		"  local episode=$1 scenario=$2 duration=$3 parquet_count=$4\n" +
		"  local now=$(date +%s)\n" +
		"  local tags=\"[\\\"episode:${episode}\\\",\\\"scenario:${scenario}\\\",\\\"image:${IMAGE_TAG}\\\",\\\"gensim_sha:${GENSIM_SHA}\\\",\\\"run_id:${RUN_ID}\\\"]\"\n" +
		"  curl -s -X POST \"https://api.${DD_SITE}/api/v1/series\" \\\n" +
		"    -H \"Content-Type: application/json\" \\\n" +
		"    -H \"DD-API-KEY: ${DD_API_KEY}\" \\\n" +
		"    -d \"{ \\\n" +
		"      \\\"series\\\": [ \\\n" +
		"        {\\\"metric\\\":\\\"gensim.episode.duration_seconds\\\",\\\"type\\\":\\\"gauge\\\",\\\"points\\\":[[${now},${duration}]],\\\"tags\\\":${tags}}, \\\n" +
		"        {\\\"metric\\\":\\\"gensim.episode.parquet_files\\\",\\\"type\\\":\\\"gauge\\\",\\\"points\\\":[[${now},${parquet_count}]],\\\"tags\\\":${tags}} \\\n" +
		"      ] \\\n" +
		"    }\" || echo \"WARN: Failed to emit DD metrics\" >&2\n" +
		"}\n" +
		"\n" +
		"# ── Main loop ───────────────────────────────────────────────────────────\n" +
		"IFS=',' read -ra EP_LIST <<< \"$EPISODES\"\n" +
		"\n" +
		"init_status\n" +
		"\n" +
		"for EP_SPEC in \"${EP_LIST[@]}\"; do\n" +
		"  EPISODE=\"${EP_SPEC%%:*}\"\n" +
		"  SCENARIO=\"${EP_SPEC##*:}\"\n" +
		"  EP_START=$(date +%s)\n" +
		"\n" +
		"  echo \"=== Episode: $EPISODE / $SCENARIO ===\"\n" +
		"\n" +
		"  update_episode_status \"$EP_SPEC\" \"running\" '{\"phase\":\"agent-install\"}'\n" +
		"\n" +
		"  # 1. Write agent values YAML\n" +
		"  cat > /workspace/agent-values.yaml <<'AGENT_EOF'\n" +
		"datadog:\n" +
		"  apiKeyExistingSecret: gensim-secrets\n" +
		"  appKeyExistingSecret: gensim-secrets\n" +
		"  kubelet:\n" +
		"    tlsVerify: false\n" +
		"  clusterName: gensim\n" +
		"agents:\n" +
		"  enabled: true\n" +
		"  image:\n" +
		"    repository: PLACEHOLDER_IMAGE_REPO\n" +
		"    tag: PLACEHOLDER_IMAGE_TAG\n" +
		"    doNotCheckTag: true\n" +
		"  useConfigMap: true\n" +
		"  customAgentConfig:\n" +
		"    observer:\n" +
		"      recording:\n" +
		"        enabled: true\n" +
		"        parquet_output_dir: /tmp/observer-parquet\n" +
		"        parquet_flush_interval: 30s\n" +
		"clusterChecksRunner:\n" +
		"  enabled: false\n" +
		"clusterAgent:\n" +
		"  enabled: true\n" +
		"  image:\n" +
		"    repository: PLACEHOLDER_IMAGE_REPO\n" +
		"    tag: PLACEHOLDER_IMAGE_TAG\n" +
		"    doNotCheckTag: true\n" +
		"AGENT_EOF\n" +
		"  # Patch placeholders with actual values (avoids heredoc variable expansion issues)\n" +
		"  sed -i \"s|PLACEHOLDER_IMAGE_REPO|$IMAGE_REPO|g\" /workspace/agent-values.yaml\n" +
		"  sed -i \"s|PLACEHOLDER_IMAGE_TAG|$IMAGE_TAG|g\" /workspace/agent-values.yaml\n" +
		"\n" +
		"  # 2. Install agent\n" +
		"  helm install dda-linux datadog/datadog \\\n" +
		"    -f /workspace/agent-values.yaml \\\n" +
		"    -n \"$KUBE_NAMESPACE\" \\\n" +
		"    --wait --timeout 5m\n" +
		"\n" +
		"  update_episode_status \"$EP_SPEC\" \"running\" '{\"phase\":\"episode-install\"}'\n" +
		"\n" +
		"  # 3. Install episode chart (if chart tarball is available)\n" +
		"  EP_RELEASE=\"\"\n" +
		"  if [ -f \"/episodes/$EPISODE/chart.tar.gz\" ]; then\n" +
		"    mkdir -p \"/workspace/chart-$EPISODE\"\n" +
		"    tar xzf \"/episodes/$EPISODE/chart.tar.gz\" -C \"/workspace/chart-$EPISODE\"\n" +
		"    CHART_DIR=$(find \"/workspace/chart-$EPISODE\" -maxdepth 1 -mindepth 1 -type d | head -1)\n" +
		"    if [ -z \"$CHART_DIR\" ]; then\n" +
		"      CHART_DIR=\"/workspace/chart-$EPISODE\"\n" +
		"    fi\n" +
		"\n" +
		"    EP_RELEASE=\"gensim-$(echo \"$EPISODE\" | tr '_' '-' | tr '[:upper:]' '[:lower:]')\"\n" +
		"    helm install \"$EP_RELEASE\" \"$CHART_DIR\" \\\n" +
		"      -n \"$KUBE_NAMESPACE\" \\\n" +
		"      --set imageRegistry=\"$IMAGE_REGISTRY\" \\\n" +
		"      --set namespace=\"$KUBE_NAMESPACE\" \\\n" +
		"      --set datadog.apiKey=\"$DD_API_KEY\" \\\n" +
		"      --set datadog.appKey=\"$DD_APP_KEY\" \\\n" +
		"      --set datadog.site=\"$DD_SITE\" \\\n" +
		"      --set datadog.env=\"$EP_RELEASE\" \\\n" +
		"      --post-renderer /scripts/post-renderer.sh \\\n" +
		"      --skip-tests\n" +
		"  fi\n" +
		"\n" +
		"  update_episode_status \"$EP_SPEC\" \"running\" '{\"phase\":\"episode-running\"}'\n" +
		"\n" +
		"  # 4. Run play-episode.sh\n" +
		"  # play-episode.sh uses SCRIPT_DIR=$(cd \"$(dirname \"$0\")\" && pwd) and looks\n" +
		"  # for episodes/<scenario>.yaml relative to SCRIPT_DIR. Copy files so paths work.\n" +
		"  cp \"/episodes/$EPISODE/play-episode.sh\" /workspace/play-episode.sh\n" +
		"  chmod +x /workspace/play-episode.sh\n" +
		"  mkdir -p /workspace/episodes\n" +
		"  cp \"/episodes/$EPISODE/$SCENARIO.yaml\" \"/workspace/episodes/$SCENARIO.yaml\"\n" +
		"  mkdir -p /workspace/results\n" +
		"  cd /workspace\n" +
		"  EP_OUTCOME=\"success\"\n" +
		"  bash /workspace/play-episode.sh run-episode \"$SCENARIO\" || EP_OUTCOME=\"failure\"\n" +
		"  cd /\n" +
		"\n" +
		"  update_episode_status \"$EP_SPEC\" \"running\" '{\"phase\":\"collecting-parquet\"}'\n" +
		"\n" +
		"  # 5. Collect parquet from agent pod\n" +
		"  PARQUET_COUNT=0\n" +
		"  AGENT_POD=$(kubectl get pod -n \"$KUBE_NAMESPACE\" -l app=dda-linux-datadog -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)\n" +
		"  if [ -z \"$AGENT_POD\" ]; then\n" +
		"    echo \"ERROR: no agent pod found (label app=dda-linux-datadog) -- parquet not collected\" >&2\n" +
		"  else\n" +
		"    echo \"Collecting parquet from pod $AGENT_POD...\"\n" +
		"    mkdir -p /workspace/results/parquet\n" +
		"    if kubectl cp \"$KUBE_NAMESPACE/$AGENT_POD:/tmp/observer-parquet\" /workspace/results/parquet/; then\n" +
		"      PARQUET_COUNT=$(find /workspace/results/parquet -type f -name '*.parquet' | wc -l)\n" +
		"      echo \"Parquet collected: $PARQUET_COUNT files\"\n" +
		"    else\n" +
		"      echo \"ERROR: kubectl cp failed -- parquet not collected\" >&2\n" +
		"    fi\n" +
		"  fi\n" +
		"\n" +
		"  # 6. Upload to S3\n" +
		"  if [ -n \"$S3_BUCKET\" ]; then\n" +
		"    EP_SCENARIO=\"${EPISODE}--${SCENARIO}\"\n" +
		"    S3_PATH=\"${IMAGE_TAG}/${EP_SCENARIO}/gensim-${GENSIM_SHA}/$(date -u +%Y%m%d)\"\n" +
		"    DEST=\"s3://${S3_BUCKET}/${S3_PATH}\"\n" +
		"    echo \"Uploading results to $DEST/...\"\n" +
		"    aws s3 cp /workspace/results/ \"$DEST/\" --recursive || echo \"ERROR: S3 upload failed\" >&2\n" +
		"    echo \"Uploaded to $DEST/\"\n" +
		"  fi\n" +
		"\n" +
		"  EP_END=$(date +%s)\n" +
		"  EP_DURATION=$((EP_END - EP_START))\n" +
		"\n" +
		"  # 7. Emit DD event + metrics\n" +
		"  emit_dd_event \"$EPISODE\" \"$SCENARIO\" \"$EP_DURATION\" \"$PARQUET_COUNT\" \"$EP_OUTCOME\"\n" +
		"  emit_dd_metrics \"$EPISODE\" \"$SCENARIO\" \"$EP_DURATION\" \"$PARQUET_COUNT\"\n" +
		"\n" +
		"  # 8. Update status\n" +
		"  update_episode_status \"$EP_SPEC\" \"done\" \"{\\\"parquetFiles\\\":$PARQUET_COUNT,\\\"durationSeconds\\\":$EP_DURATION}\"\n" +
		"\n" +
		"  # 9. Teardown episode + agent\n" +
		"  echo \"Tearing down episode and agent...\"\n" +
		"  if [ -n \"${EP_RELEASE:-}\" ]; then\n" +
		"    helm uninstall \"$EP_RELEASE\" -n \"$KUBE_NAMESPACE\" --wait 2>/dev/null || true\n" +
		"  fi\n" +
		"  helm uninstall dda-linux -n \"$KUBE_NAMESPACE\" --wait 2>/dev/null || true\n" +
		"\n" +
		"  # Wait for pods to terminate\n" +
		"  echo \"Waiting for agent pods to terminate...\"\n" +
		"  kubectl wait --for=delete pod -l app=dda-linux-datadog -n \"$KUBE_NAMESPACE\" --timeout=120s 2>/dev/null || true\n" +
		"\n" +
		"  # Clean workspace for next episode\n" +
		"  rm -rf /workspace/results /workspace/chart-* /workspace/agent-values.yaml /workspace/play-episode.sh /workspace/episodes\n" +
		"\n" +
		"  echo \"=== Episode $EPISODE / $SCENARIO complete (${EP_DURATION}s) ===\"\n" +
		"done\n" +
		"\n" +
		"set_run_complete\n" +
		"echo \"All episodes complete.\"\n"
}

// createTarGz creates a gzip-compressed tar archive of the given directory.
// The archive preserves the directory structure relative to the parent of dir,
// so that when extracted, files appear under a top-level directory matching
// the base name of dir (e.g. "chart/").
func createTarGz(dir string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	baseDir := filepath.Base(dir) // e.g. "chart"

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Build the archive path relative to dir's parent so it starts with "chart/".
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		archivePath := filepath.Join(baseDir, rel)

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = archivePath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Close in order: tar writer, then gzip writer.
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
