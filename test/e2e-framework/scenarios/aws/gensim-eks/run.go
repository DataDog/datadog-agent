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
//	  - gensim-orchestrator Job (alpine/k8s)
package gensimeks

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	e2econfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	osComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
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

// episodeSubdirs lists the subdirectories within the gensim-episodes repo
// that may contain episode directories.
var episodeSubdirs = []string{"postmortems", "synthetics"}

// findEpisodeDir locates an episode directory by searching known subdirectories
// within the gensim-episodes repo root. Also supports legacy episodeDataDir
// pointing directly at a subdirectory (e.g. .../postmortems).
func findEpisodeDir(repoRoot, episodeName string) (string, error) {
	// Direct child (legacy: episodeDataDir=.../postmortems)
	direct := filepath.Join(repoRoot, episodeName)
	if info, err := os.Stat(direct); err == nil && info.IsDir() {
		return direct, nil
	}
	for _, subdir := range episodeSubdirs {
		candidate := filepath.Join(repoRoot, subdir, episodeName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("episode %q not found in %v under %s", episodeName, episodeSubdirs, repoRoot)
}

// Run is the Pulumi entry point for the aws/gensim-eks scenario.
// It is registered in registry/scenarios.go and invoked by the e2e-framework runner.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// ── Cluster ───────────────────────────────────────────────────────────────
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
	mode := cfg.Get("mode")
	if mode == "" {
		mode = "record-parquet"
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

	// ── Build VM (for episodes with custom service images) ──────────────
	// If any episode has a docker-compose.yaml, provision a build VM to
	// build and push images to ECR. This runs before the orchestrator Job
	// so images are available when helm installs the episode chart.
	if imageRegistry != "" && episodeDataDir != "" {
		var buildVM *remote.Host
		var installDep command.Command

		pairs := strings.Split(episodes, ",")
		for _, p := range pairs {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			episode := strings.SplitN(p, ":", 2)[0]
			episodePath, findErr := findEpisodeDir(episodeDataDir, episode)
			if findErr != nil {
				return findErr
			}
			dockerComposePath := filepath.Join(episodePath, "docker-compose.yaml")
			if _, statErr := os.Stat(dockerComposePath); statErr == nil {
				if buildVM == nil {
					buildVM, installDep, err = provisionBuildVM(ctx, awsEnv)
					if err != nil {
						return err
					}
				}
				if err = buildEpisodeImages(ctx, awsEnv, buildVM, installDep, episode, episodePath, imageRegistry); err != nil {
					return err
				}
			}
		}
	}

	// ── Orchestrator Job ─────────────────────────────────────────────────────
	if err := deployOrchestratorJob(
		ctx, &awsEnv, kubeProvider, sa, ddSecret,
		episodes, agentImage, gensimSha, namespace, s3Bucket, imageRegistry, episodeDataDir, mode,
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
				"api-key": awsEnv.AgentAPIKey(),
				"app-key": awsEnv.AgentAPPKey(),
			},
		}, kubeOpts...)
	if err != nil {
		return nil, nil, err
	}

	return sa, ddSecret, nil
}

// deployOrchestratorJob creates per-episode ConfigMaps and the orchestrator Job
// that drives episode execution inside the cluster.
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
	mode string,
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
		epDir, findErr := findEpisodeDir(episodeDataDir, ep.episode)
		if findErr != nil {
			return findErr
		}
		playScriptContent, err := os.ReadFile(filepath.Join(epDir, "play-episode.sh"))
		if err != nil {
			return fmt.Errorf("reading play-episode.sh for episode %q: %w", ep.episode, err)
		}
		scenarioContent, err := os.ReadFile(filepath.Join(epDir, "episodes", ep.scenario+".yaml"))
		if err != nil {
			return fmt.Errorf("reading scenario %q for episode %q: %w", ep.scenario, ep.episode, err)
		}

		// Create chart tarball from the episode's chart/ directory.
		chartDir := filepath.Join(epDir, "chart")
		chartTarball, err := createTarGz(chartDir)
		if err != nil {
			return fmt.Errorf("creating chart tarball for episode %q: %w", ep.episode, err)
		}
		if len(chartTarball) > 500*1024 {
			fmt.Printf("WARNING: chart tarball for episode %q is %d bytes (>500KB); ConfigMap limit is 1MiB total\n", ep.episode, len(chartTarball))
		}
		chartTarballB64 := base64.StdEncoding.EncodeToString(chartTarball)

		// Kubernetes names must be lowercase RFC 1123: [a-z0-9-.]
		sanitized := strings.ToLower(strings.ReplaceAll(ep.episode, "_", "-"))
		cmName := "gensim-ep-" + sanitized
		volName := "ep-" + sanitized

		cm, err := corev1.NewConfigMap(ctx, awsEnv.Namer.ResourceName("ep-cm-"+sanitized),
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

	// ── Agent values ConfigMap ───────────────────────────────────────────────
	renderedValues, err := renderAgentValues(agentImage, mode)
	if err != nil {
		return err
	}
	agentValuesCM, err := corev1.NewConfigMap(ctx, awsEnv.Namer.ResourceName("agent-values-cm"),
		&corev1.ConfigMapArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("gensim-agent-values"),
				Namespace: pulumi.String(namespace),
			},
			Data: pulumi.StringMap{
				"agent-values.yaml": pulumi.String(renderedValues),
			},
		}, kubeOpts...)
	if err != nil {
		return err
	}

	// ── Volumes ──────────────────────────────────────────────────────────────
	episodeVolumes = append(episodeVolumes, corev1.VolumeArgs{
		Name: pulumi.String("agent-values"),
		ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
			Name: pulumi.String("gensim-agent-values"),
		},
	})
	// Workspace emptyDir
	episodeVolumes = append(episodeVolumes, corev1.VolumeArgs{
		Name:     pulumi.String("workspace"),
		EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
	})

	// ── Volume mounts ────────────────────────────────────────────────────────
	episodeVolumeMounts = append(episodeVolumeMounts, corev1.VolumeMountArgs{
		Name:      pulumi.String("agent-values"),
		MountPath: pulumi.String("/config/agent-values.yaml"),
		SubPath:   pulumi.String("agent-values.yaml"),
	})
	episodeVolumeMounts = append(episodeVolumeMounts, corev1.VolumeMountArgs{
		Name:      pulumi.String("workspace"),
		MountPath: pulumi.String("/workspace"),
	})

	// ── Job dependencies ─────────────────────────────────────────────────────
	var jobDeps []pulumi.Resource
	jobDeps = append(jobDeps, sa, ddSecret, agentValuesCM)
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
								Args:    pulumi.StringArray{pulumi.String(buildOrchestratorScript(episodes, agentImage, gensimSha, namespace, s3Bucket, imageRegistry, mode))},
								Env: corev1.EnvVarArray{
									corev1.EnvVarArgs{
										Name: pulumi.String("DD_API_KEY"),
										ValueFrom: &corev1.EnvVarSourceArgs{
											SecretKeyRef: &corev1.SecretKeySelectorArgs{
												Name: ddSecret.Metadata.Name().Elem(),
												Key:  pulumi.String("api-key"),
											},
										},
									},
									corev1.EnvVarArgs{
										Name: pulumi.String("DD_APP_KEY"),
										ValueFrom: &corev1.EnvVarSourceArgs{
											SecretKeyRef: &corev1.SecretKeySelectorArgs{
												Name: ddSecret.Metadata.Name().Elem(),
												Key:  pulumi.String("app-key"),
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
									corev1.EnvVarArgs{Name: pulumi.String("GENSIM_MODE"), Value: pulumi.StringPtr(mode)},
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
//
//go:embed agent-values.yaml.tmpl
var agentValuesTmpl string

// renderAgentValues renders the agent Helm values template with the given image and mode.
// mode is one of "record-parquet" or "live-anomaly-detection".
func renderAgentValues(agentImage, mode string) (string, error) {
	idx := strings.LastIndex(agentImage, ":")
	if idx < 0 {
		return "", fmt.Errorf("invalid image reference %q: expected repo:tag format", agentImage)
	}
	repo := agentImage[:idx]
	tag := agentImage[idx+1:]

	tmpl, err := template.New("agent-values").Parse(agentValuesTmpl)
	if err != nil {
		return "", fmt.Errorf("parsing agent-values template: %w", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct{ ImageRepo, ImageTag, Mode string }{repo, tag, mode})
	if err != nil {
		return "", fmt.Errorf("rendering agent-values template: %w", err)
	}
	return buf.String(), nil
}

func buildOrchestratorScript(episodes, agentImage, gensimSha, namespace, s3Bucket, imageRegistry, mode string) string {
	return `set -euo pipefail
apk add --no-cache aws-cli 2>/dev/null || true
helm repo add datadog https://helm.datadoghq.com && helm repo update

# ── Parse image into repo + tag ──────────────────────────────────────────
IMAGE_REPO="${AGENT_IMAGE%:*}"
IMAGE_TAG="${AGENT_IMAGE##*:}"
RUN_ID="eval-$(date -u +%Y%m%d)-${GENSIM_SHA:0:7}"

echo "Orchestrator starting"
echo "  Run ID:     $RUN_ID"
echo "  Episodes:   $EPISODES"
echo "  Image:      $AGENT_IMAGE"
echo "  Gensim SHA: $GENSIM_SHA"
echo "  S3 Bucket:  $S3_BUCKET"
echo "  Namespace:  $KUBE_NAMESPACE"
echo "  Mode:       $GENSIM_MODE"

# ── Status ConfigMap helpers ────────────────────────────────────────────
init_status() {
  local episodes_json="[]"
  IFS=',' read -ra _EP_INIT <<< "$EPISODES"
  for _EP in "${_EP_INIT[@]}"; do
    local _episode="${_EP%%:*}"
    local _scenario="${_EP##*:}"
    episodes_json=$(printf '%s' "$episodes_json" | jq -c --arg ep "$_episode" --arg sc "$_scenario" '. + [{"episode":$ep,"scenario":$sc,"status":"queued"}]')
  done
  local status_json
  status_json=$(jq -n -c \
    --arg runId "$RUN_ID" \
    --arg image "$AGENT_IMAGE" \
    --arg sha "$GENSIM_SHA" \
    --arg started "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson eps "$episodes_json" \
    '{runId:$runId,image:$image,gensimSha:$sha,startedAt:$started,episodes:$eps}')
  kubectl create configmap gensim-run-status \
    --from-literal=status="$status_json" \
    -n "$KUBE_NAMESPACE" \
    --dry-run=client -o yaml | kubectl apply -f -
}

update_episode_status() {
  local ep_spec="$1" new_status="$2" extra="${3}"
  [ -z "$extra" ] && extra='{}'
  local ep="${ep_spec%%:*}"
  local sc="${ep_spec##*:}"
  local current
  current=$(kubectl get configmap gensim-run-status -n "$KUBE_NAMESPACE" -o jsonpath='{.data.status}')
  local updated
  updated=$(printf '%s' "$current" | jq -c \
    --arg ep "$ep" --arg sc "$sc" --arg st "$new_status" --argjson ex "$extra" \
    '(.episodes[] | select(.episode==$ep and .scenario==$sc)) |= (. + $ex + {status:$st})')
  kubectl create configmap gensim-run-status \
    --from-literal=status="$updated" \
    -n "$KUBE_NAMESPACE" \
    --dry-run=client -o yaml | kubectl apply -f -
}

set_run_complete() {
  local current
  current=$(kubectl get configmap gensim-run-status -n "$KUBE_NAMESPACE" -o jsonpath='{.data.status}')
  local updated
  updated=$(printf '%s' "$current" | jq -c --arg t "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '. + {completedAt:$t}')
  kubectl create configmap gensim-run-status \
    --from-literal=status="$updated" \
    -n "$KUBE_NAMESPACE" \
    --dry-run=client -o yaml | kubectl apply -f -
}

# ── DD event + metrics helpers ───────────────────────────────────────────
emit_dd_event() {
  local episode=$1 scenario=$2 duration=$3 parquet_count=$4 outcome=$5
  local title="gensim: ${episode}/${scenario} ${outcome}"
  local text="Duration: ${duration}s, Parquet files: ${parquet_count}, Image: ${AGENT_IMAGE}, SHA: ${GENSIM_SHA}"
  local alert_type="success"
  [ "$outcome" != "success" ] && alert_type="error"
  curl -s -X POST "https://api.${DD_SITE}/api/v1/events" \
    -H "Content-Type: application/json" \
    -H "DD-API-KEY: ${DD_API_KEY}" \
    -d "{ \
      \"title\": \"${title}\", \
      \"text\": \"${text}\", \
      \"alert_type\": \"${alert_type}\", \
      \"tags\": [ \
        \"episode:${episode}\", \
        \"scenario:${scenario}\", \
        \"image:${IMAGE_TAG}\", \
        \"gensim_sha:${GENSIM_SHA}\", \
        \"run_id:${RUN_ID}\" \
      ] \
    }" || echo "WARN: Failed to emit DD event" >&2
}

emit_dd_metrics() {
  local episode=$1 scenario=$2 duration=$3 parquet_count=$4
  local now=$(date +%s)
  local tags="[\"episode:${episode}\",\"scenario:${scenario}\",\"image:${IMAGE_TAG}\",\"gensim_sha:${GENSIM_SHA}\",\"run_id:${RUN_ID}\"]"
  curl -s -X POST "https://api.${DD_SITE}/api/v1/series" \
    -H "Content-Type: application/json" \
    -H "DD-API-KEY: ${DD_API_KEY}" \
    -d "{ \
      \"series\": [ \
        {\"metric\":\"gensim.episode.duration_seconds\",\"type\":\"gauge\",\"points\":[[${now},${duration}]],\"tags\":${tags}}, \
        {\"metric\":\"gensim.episode.parquet_files\",\"type\":\"gauge\",\"points\":[[${now},${parquet_count}]],\"tags\":${tags}} \
      ] \
    }" || echo "WARN: Failed to emit DD metrics" >&2
}

# ── Post-renderer: fix imagePullPolicy: Never -> Always ──────────────────
# Some episodes (e.g. 678_heroku) set imagePullPolicy: Never for local dev.
# This post-renderer patches it to Always for EKS.
cat > /workspace/fix-pull-policy.sh <<'FIXEOF'
#!/bin/sh
sed 's/imagePullPolicy: Never/imagePullPolicy: Always/g'
FIXEOF
chmod +x /workspace/fix-pull-policy.sh

# ── Main loop ───────────────────────────────────────────────────────────
IFS=',' read -ra EP_LIST <<< "$EPISODES"

init_status

for EP_SPEC in "${EP_LIST[@]}"; do
  EPISODE="${EP_SPEC%%:*}"
  SCENARIO="${EP_SPEC##*:}"
  EP_START=$(date +%s)

  echo "=== Episode: $EPISODE / $SCENARIO ==="

  update_episode_status "$EP_SPEC" "running" '{"phase":"episode-install"}'

  # 1. Install episode chart with agent resources disabled
  EP_RELEASE=""
  if [ -f "/episodes/$EPISODE/chart.tar.gz" ]; then
    mkdir -p "/workspace/chart-$EPISODE"
    tar xzf "/episodes/$EPISODE/chart.tar.gz" -C "/workspace/chart-$EPISODE"
    CHART_DIR=$(find "/workspace/chart-$EPISODE" -maxdepth 1 -mindepth 1 -type d | head -1)
    if [ -z "$CHART_DIR" ]; then
      CHART_DIR="/workspace/chart-$EPISODE"
    fi

    EP_RELEASE="gensim-$(echo "$EPISODE" | tr '_' '-' | tr '[:upper:]' '[:lower:]')"

    helm install "$EP_RELEASE" "$CHART_DIR" \
      --set agent.enabled=false \
      --set imageRegistry="$IMAGE_REGISTRY" \
      --set namespace="$KUBE_NAMESPACE" \
      --set datadog.apiKey="$DD_API_KEY" \
      --set datadog.appKey="$DD_APP_KEY" \
      --set datadog.site="$DD_SITE" \
      --set datadog.env="$EP_RELEASE" \
      -n "$KUBE_NAMESPACE" \
      --post-renderer /workspace/fix-pull-policy.sh \
      --wait --timeout 3m
  fi

  update_episode_status "$EP_SPEC" "running" '{"phase":"agent-install"}'

  # 2. Install agent (autodiscovery handles check configs)
  cp /config/agent-values.yaml /workspace/agent-values.yaml
  helm upgrade --install datadog-agent datadog/datadog \
    -f /workspace/agent-values.yaml \
    -n "$KUBE_NAMESPACE" \
    --wait --timeout 5m

  update_episode_status "$EP_SPEC" "running" '{"phase":"episode-running"}'

  # 3. Run play-episode.sh
  # play-episode.sh uses SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd) and looks
  # for episodes/<scenario>.yaml relative to SCRIPT_DIR. Copy files so paths work.
  cp "/episodes/$EPISODE/play-episode.sh" /workspace/play-episode.sh
  chmod +x /workspace/play-episode.sh
  mkdir -p /workspace/episodes
  cp "/episodes/$EPISODE/$SCENARIO.yaml" "/workspace/episodes/$SCENARIO.yaml"
  mkdir -p /workspace/results
  cd /workspace
  # play-episode.sh requires DD_ENV, DD_API_KEY, DD_APP_KEY, KUBE_NAMESPACE.
  # DD_API_KEY, DD_APP_KEY, KUBE_NAMESPACE are set on the Job env. DD_ENV must
  # be set to the episode helm release name (used for monitor scoping).
  export DD_ENV="gensim-$(echo "$EPISODE" | tr '_' '-' | tr '[:upper:]' '[:lower:]')"
  EP_OUTCOME="success"
  bash /workspace/play-episode.sh run-episode "$SCENARIO" || EP_OUTCOME="failure"
  cd /

  # 4. Collect results (mode-dependent)
  PARQUET_COUNT=0
  if [ "$GENSIM_MODE" = "record-parquet" ]; then
    update_episode_status "$EP_SPEC" "running" '{"phase":"collecting-parquet"}'
    AGENT_POD=$(kubectl get pod -n "$KUBE_NAMESPACE" -l app=datadog-agent -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    if [ -z "$AGENT_POD" ]; then
      echo "ERROR: no agent pod found (label app=datadog-agent) -- parquet not collected" >&2
    else
      echo "Collecting parquet from pod $AGENT_POD..."
      mkdir -p /workspace/results/parquet
      if kubectl cp "$KUBE_NAMESPACE/$AGENT_POD:/tmp/observer-parquet" /workspace/results/parquet/; then
        PARQUET_COUNT=$(find /workspace/results/parquet -type f -name '*.parquet' | wc -l)
        echo "Parquet collected: $PARQUET_COUNT files"
      else
        echo "ERROR: kubectl cp failed -- parquet not collected" >&2
      fi
    fi

    # 5. Upload to S3
    if [ -n "$S3_BUCKET" ]; then
      EP_SCENARIO="${EPISODE}--${SCENARIO}"
      S3_PATH="${IMAGE_TAG}/${EP_SCENARIO}/gensim-$(date -u +%Y%m%d)-${GENSIM_SHA}"
      DEST="s3://${S3_BUCKET}/${S3_PATH}"
      echo "Uploading results to $DEST/..."
      aws s3 cp /workspace/results/ "$DEST/" --recursive || echo "ERROR: S3 upload failed" >&2
      echo "Uploaded to $DEST/"
    fi
  else
    echo "Mode: $GENSIM_MODE -- skipping parquet collection and S3 upload"
  fi

  EP_END=$(date +%s)
  EP_DURATION=$((EP_END - EP_START))

  # 6. Emit DD event + metrics
  emit_dd_event "$EPISODE" "$SCENARIO" "$EP_DURATION" "$PARQUET_COUNT" "$EP_OUTCOME"
  emit_dd_metrics "$EPISODE" "$SCENARIO" "$EP_DURATION" "$PARQUET_COUNT"

  # 7. Update status
  update_episode_status "$EP_SPEC" "done" "{\"parquetFiles\":$PARQUET_COUNT,\"durationSeconds\":$EP_DURATION}"

  # 8. Teardown episode + agent
  echo "Tearing down episode and agent..."
  if [ -n "$EP_RELEASE" ]; then
    helm uninstall "$EP_RELEASE" -n "$KUBE_NAMESPACE" --wait 2>/dev/null || true
  fi
  helm uninstall datadog-agent -n "$KUBE_NAMESPACE" --wait 2>/dev/null || true

  # Wait for pods to terminate
  echo "Waiting for agent pods to terminate..."
  kubectl wait --for=delete pod -l app=datadog-agent -n "$KUBE_NAMESPACE" --timeout=120s 2>/dev/null || true

  # Clean workspace for next episode
  rm -rf /workspace/results /workspace/chart-* /workspace/agent-values.yaml /workspace/play-episode.sh /workspace/episodes

  echo "=== Episode $EPISODE / $SCENARIO complete (${EP_DURATION}s) ==="
done

set_run_complete
echo "All episodes complete."
`
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

// provisionBuildVM creates a single EC2 build VM with Docker pre-installed
// (Amazon Linux ECS AMI) and installs the AWS CLI. The VM is shared across
// all episodes that need image building.
func provisionBuildVM(ctx *pulumi.Context, awsEnv resAws.Environment) (*remote.Host, command.Command, error) {
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
		return nil, nil, err
	}

	// Use the Amazon Linux ECS AMI — Docker 25 (including buildx) is pre-installed
	// and the daemon is already running. Only AWS CLI needs to be added (~30s via yum).
	buildHost, err := ec2.NewVM(awsEnv, "gensim-builder", ec2.WithOS(osComp.AmazonLinuxECSDefault))
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}

	return buildHost, installToolsCmd, nil
}

// TODO: Remove once gensim-episodes publishes pre-built images for all episodes.
//
// buildEpisodeImages copies a single episode's service source code to the build VM,
// builds Docker images via docker buildx bake, and pushes them to ECR.
//
// Each episode gets unique Pulumi resource names (suffixed with episodeName) so
// multiple episodes can have their images built on the same VM without collisions.
func buildEpisodeImages(
	ctx *pulumi.Context,
	awsEnv resAws.Environment,
	buildHost *remote.Host,
	installDep command.Command,
	episodeName string,
	episodePath string,
	ecrRegistry string,
) error {
	buildDir := "/tmp/gensim-build-" + episodeName

	// Copy the episode's docker-compose.yaml and services/ directory to the build VM.
	servicesCopy, err := buildHost.OS.FileManager().CopyAbsoluteFolder(
		filepath.Join(episodePath, "services"), buildDir+"/",
	)
	if err != nil {
		return err
	}

	// Read docker-compose.yaml content locally and write it to the build VM via
	// inline command. This avoids CopyFile, whose resource naming can collide when
	// multiple episodes are built on the same VM in a single Pulumi run.
	composeContent, err := os.ReadFile(filepath.Join(episodePath, "docker-compose.yaml"))
	if err != nil {
		return fmt.Errorf("reading docker-compose.yaml for %s: %w", episodeName, err)
	}
	dockerComposeCopy, err := buildHost.OS.Runner().Command(
		awsEnv.Namer.ResourceName("write-compose-"+episodeName),
		&command.Args{
			Create: pulumi.Sprintf("tee %s/docker-compose.yaml > /dev/null <<'COMPOSE_EOF'\n%s\nCOMPOSE_EOF", buildDir, string(composeContent)),
			Sudo:   true,
		},
		utils.PulumiDependsOn(servicesCopy...),
	)
	if err != nil {
		return err
	}

	// Fix ownership so the build directory is writable by the SSH user.
	fixPermsCmd, err := buildHost.OS.Runner().Command(
		awsEnv.Namer.ResourceName("fix-build-dir-perms-"+episodeName),
		&command.Args{
			Create: pulumi.Sprintf(`chown -R $(id -un):$(id -gn) %s/`, buildDir),
			Sudo:   true,
		},
		utils.PulumiDependsOn(append(servicesCopy, dockerComposeCopy)...),
	)
	if err != nil {
		return err
	}

	// Compute a hash of the local services directory and docker-compose.yaml so
	// Pulumi can detect when source files change. Without this Trigger, DependsOn
	// only controls ordering and the build command would never re-run after the
	// initial create, even if app.py, a Dockerfile, or docker-compose.yaml changed.
	servicesHash, err := hashDir(filepath.Join(episodePath, "services"))
	if err != nil {
		return fmt.Errorf("hashing services directory for %s: %w", episodeName, err)
	}
	// Include docker-compose.yaml in the hash so adding/removing images or
	// changing build contexts also triggers a rebuild.
	composeFile := filepath.Join(episodePath, "docker-compose.yaml")
	if composeContent, err := os.ReadFile(composeFile); err == nil {
		h := sha256.Sum256(composeContent)
		servicesHash = servicesHash + hex.EncodeToString(h[:])
	}

	// Build all images and push to ECR.
	//
	// Caching: images are also tagged with the first 12 chars of servicesHash
	// (e.g. "abc123def456"). On re-runs — including fresh cluster recreations with
	// unchanged source — we check ECR for this hash tag first. If all images are
	// already present, we skip the docker-compose build entirely (saves 5-10 min).
	//
	// The :latest tag is always pushed so the Helm chart's imageRegistry prefix works
	// without any changes to chart values.
	_, err = buildHost.OS.Runner().Command(
		awsEnv.Namer.ResourceName("build-push-images-"+episodeName),
		&command.Args{
			// Triggers forces this command to re-run whenever the source files change.
			// Pulumi replaces the resource when any trigger value differs from state.
			Triggers: pulumi.Array{pulumi.String(servicesHash)},
			Create: pulumi.Sprintf(
				`set -euo pipefail

# AWS CLI is installed on the ECS AMI but not in the default SSH PATH.
export PATH=$PATH:/usr/bin:/usr/local/bin

cd %s

REGION="%s"
ECR_REGISTRY="%s"
CACHE_TAG="%s"  # first 12 chars of services hash — stable cache key

# Authenticate Docker with ECR using the instance IAM role.
# --password-stdin works without a TTY, which is how Pulumi executes remote commands.
aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "$ECR_REGISTRY"

# Parse image names from docker-compose.yaml using grep+awk.
# Avoids python3 yaml module which is not available on Amazon Linux ECS Python 3.7.
# grep without ^ anchor matches 4-space-indented image: lines correctly.
IMAGES=$(grep '  image:' docker-compose.yaml | awk '{print $2}')

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
				buildDir,
				awsEnv.Region(),
				ecrRegistry,
				servicesHash[:12],
			),
		},
		utils.PulumiDependsOn(installDep, fixPermsCmd),
	)
	if err != nil {
		return err
	}

	return nil
}

// hashDir computes a deterministic SHA256 hash of all files under a directory.
// Used as a Pulumi Trigger so the build command re-runs when source files change
// (Pulumi's DependsOn only controls ordering, not re-execution).
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
