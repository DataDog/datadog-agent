// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package otelstandalone provides a Pulumi component that deploys the Datadog
// otel-agent as a standalone DaemonSet, without a co-located core Datadog Agent.
// Use this instead of the Datadog Helm chart when testing DD_OTEL_STANDALONE=true,
// because the Helm chart always includes the core-agent container and does not
// expose a values path to set env vars on the otel-agent sidecar.
package otelstandalone

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

const (
	appLabel   = "standalone-otel-agent"
	saName     = "standalone-otel-agent"
	crName     = "standalone-otel-agent"
	crbName    = "standalone-otel-agent"
	configKey  = "otel-config.yaml"
	configDir  = "/etc/datadog-agent"
	configPath = configDir + "/" + configKey
	binaryPath = "/opt/datadog-agent/embedded/bin/otel-agent"
)

// AppOption is a functional option for K8sAppDefinition that controls
// application-level deployment (env vars, volumes, K8s Secrets, etc.).
type AppOption func(*appConfig)

// appConfig holds the accumulated application-level options.
type appConfig struct {
	extraEnvVars      corev1.EnvVarArray
	extraVolumes      corev1.VolumeArray
	extraVolumeMounts corev1.VolumeMountArray
	k8sSecrets        []appSecretSpec
	skipDefaultHostname bool
}

// appSecretSpec describes a Kubernetes Opaque secret to create before the DaemonSet.
type appSecretSpec struct {
	name string
	data map[string]string
}

// WithExtraEnvVars appends env vars to the otel-agent container.
// If you need to override DD_HOSTNAME, use WithoutDefaultHostname() so the
// downward-API DD_HOSTNAME does not shadow your value.
func WithExtraEnvVars(vars ...corev1.EnvVarInput) AppOption {
	return func(o *appConfig) { o.extraEnvVars = append(o.extraEnvVars, vars...) }
}

// WithExtraVolumes appends volumes to the DaemonSet pod spec.
func WithExtraVolumes(vols ...corev1.VolumeInput) AppOption {
	return func(o *appConfig) { o.extraVolumes = append(o.extraVolumes, vols...) }
}

// WithExtraVolumeMounts appends volume mounts to the otel-agent container.
func WithExtraVolumeMounts(mounts ...corev1.VolumeMountInput) AppOption {
	return func(o *appConfig) { o.extraVolumeMounts = append(o.extraVolumeMounts, mounts...) }
}

// WithK8sSecret creates a Kubernetes Opaque secret in the DaemonSet namespace
// before the DaemonSet starts so pods can mount it immediately.
// data maps secret keys to their plaintext values (Kubernetes handles base64).
func WithK8sSecret(name string, data map[string]string) AppOption {
	return func(o *appConfig) {
		o.k8sSecrets = append(o.k8sSecrets, appSecretSpec{name: name, data: data})
	}
}

// WithoutDefaultHostname disables the built-in DD_HOSTNAME downward-API env var
// (which normally resolves to the node name via spec.nodeName).
// Use this when you want to supply DD_HOSTNAME yourself via WithExtraEnvVars so
// that the agent reads your value first — Go's os.Getenv returns the first
// matching variable, and the downward-API entry would otherwise shadow yours.
func WithoutDefaultHostname() AppOption {
	return func(o *appConfig) { o.skipDefaultHostname = true }
}

// K8sAppDefinition deploys the Datadog otel-agent as a standalone DaemonSet.
// It merges the fakeintake URL into the OTel exporter config so that telemetry
// is captured by the fakeintake during E2E tests.
//
// The returned *agent.KubernetesAgent has LinuxNodeAgent.LabelSelectors["app"]
// set to "standalone-otel-agent", which is what test utilities such as
// getAgentPod use to locate the pod.
//
// Pass AppOption values to customise the deployment (extra env vars, volumes,
// K8s Secrets, etc.).  Pulumi resource options are always added internally.
func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, otelConfig string, fakeIntake *fakeintake.Fakeintake, appOpts ...AppOption) (*agent.KubernetesAgent, error) {
	// Apply functional options.
	acfg := &appConfig{}
	for _, opt := range appOpts {
		opt(acfg)
	}

	return components.NewComponent(e, "standalone-otel-agent", func(comp *agent.KubernetesAgent) error {
		opts := []pulumi.ResourceOption{
			pulumi.Provider(kubeProvider),
			pulumi.Parent(kubeProvider),
			pulumi.DeletedWith(kubeProvider),
		}

		// Build the merged OTel config ConfigMap data, injecting the fakeintake URL.
		configMapData, err := buildConfigMapData(otelConfig, fakeIntake)
		if err != nil {
			return err
		}

		// Namespace
		ns, err := corev1.NewNamespace(e.Ctx(), namespace, &corev1.NamespaceArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.String(namespace),
			},
		}, opts...)
		if err != nil {
			return err
		}

		nsOpts := append(opts, utils.PulumiDependsOn(ns))

		// Create any K8s Secrets requested via WithK8sSecret AppOptions.
		// These are created before the DaemonSet so that pods can mount them on startup.
		for _, spec := range acfg.k8sSecrets {
			stringData := make(pulumi.StringMap, len(spec.data))
			for k, v := range spec.data {
				stringData[k] = pulumi.String(v)
			}
			_, err := corev1.NewSecret(e.Ctx(), spec.name, &corev1.SecretArgs{
				Metadata: metav1.ObjectMetaArgs{
					Name:      pulumi.String(spec.name),
					Namespace: pulumi.String(namespace),
				},
				StringData: stringData,
			}, nsOpts...)
			if err != nil {
				return err
			}
		}

		// Image pull secret (required in CI where images are pulled from internal registry)
		var imagePullSecrets corev1.LocalObjectReferenceArray
		if e.ImagePullRegistry() != "" {
			secret, err := utils.NewImagePullSecret(e, namespace, nsOpts...)
			if err != nil {
				return err
			}
			imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReferenceArgs{
				Name: secret.Metadata.Name(),
			})
		}

		// ConfigMap carrying the merged OTel config YAML
		cm, err := corev1.NewConfigMap(e.Ctx(), "standalone-otel-agent-config", &corev1.ConfigMapArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String("standalone-otel-agent-config"),
				Namespace: pulumi.String(namespace),
			},
			Data: configMapData,
		}, nsOpts...)
		if err != nil {
			return err
		}

		// Service — exposes OTLP gRPC (4317) and HTTP (4318) endpoints so that
		// workloads in the cluster can resolve "standalone-otel-agent" via DNS.
		_, err = corev1.NewService(e.Ctx(), appLabel, &corev1.ServiceArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(appLabel),
				Namespace: pulumi.String(namespace),
			},
			Spec: &corev1.ServiceSpecArgs{
				Selector: pulumi.StringMap{
					"app": pulumi.String(appLabel),
				},
				Ports: corev1.ServicePortArray{
					&corev1.ServicePortArgs{
						Name:     pulumi.String("otlp-grpc"),
						Port:     pulumi.Int(4317),
						Protocol: pulumi.String("TCP"),
					},
					&corev1.ServicePortArgs{
						Name:     pulumi.String("otlp-http"),
						Port:     pulumi.Int(4318),
						Protocol: pulumi.String("TCP"),
					},
				},
			},
		}, nsOpts...)
		if err != nil {
			return err
		}

		// ServiceAccount
		sa, err := corev1.NewServiceAccount(e.Ctx(), saName, &corev1.ServiceAccountArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(saName),
				Namespace: pulumi.String(namespace),
			},
		}, nsOpts...)
		if err != nil {
			return err
		}

		// ClusterRole: permissions needed by workloadmeta (pods, nodes, namespaces,
		// deployments, replicasets, statefulsets, daemonsets) plus kubelet API access.
		cr, err := rbacv1.NewClusterRole(e.Ctx(), crName, &rbacv1.ClusterRoleArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.String(crName),
			},
			Rules: rbacv1.PolicyRuleArray{
				&rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.StringArray{pulumi.String("")},
					Resources: pulumi.StringArray{
						pulumi.String("pods"),
						pulumi.String("nodes"),
						pulumi.String("namespaces"),
						pulumi.String("services"),
						pulumi.String("endpoints"),
						pulumi.String("events"),
						pulumi.String("componentstatuses"),
						pulumi.String("nodes/metrics"),
						pulumi.String("nodes/spec"),
						pulumi.String("nodes/proxy"),
						pulumi.String("nodes/stats"),
					},
					Verbs: pulumi.StringArray{
						pulumi.String("get"),
						pulumi.String("list"),
						pulumi.String("watch"),
					},
				},
				&rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.StringArray{pulumi.String("apps")},
					Resources: pulumi.StringArray{
						pulumi.String("deployments"),
						pulumi.String("replicasets"),
						pulumi.String("statefulsets"),
						pulumi.String("daemonsets"),
					},
					Verbs: pulumi.StringArray{
						pulumi.String("get"),
						pulumi.String("list"),
						pulumi.String("watch"),
					},
				},
				&rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.StringArray{pulumi.String("batch")},
					Resources: pulumi.StringArray{
						pulumi.String("jobs"),
						pulumi.String("cronjobs"),
					},
					Verbs: pulumi.StringArray{
						pulumi.String("get"),
						pulumi.String("list"),
						pulumi.String("watch"),
					},
				},
			},
		}, opts...) // cluster-scoped: no namespace dep
		if err != nil {
			return err
		}

		// ClusterRoleBinding
		_, err = rbacv1.NewClusterRoleBinding(e.Ctx(), crbName, &rbacv1.ClusterRoleBindingArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.String(crbName),
			},
			RoleRef: rbacv1.RoleRefArgs{
				ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
				Kind:     pulumi.String("ClusterRole"),
				Name:     pulumi.String(crName),
			},
			Subjects: rbacv1.SubjectArray{
				&rbacv1.SubjectArgs{
					Kind:      pulumi.String("ServiceAccount"),
					Name:      pulumi.String(saName),
					Namespace: pulumi.String(namespace),
				},
			},
		}, append(opts, utils.PulumiDependsOn(cr), utils.PulumiDependsOn(sa))...)
		if err != nil {
			return err
		}

		// DaemonSet
		imagePath := dockerOTelAgentFullImagePath(e)

		// Build the env var list. AppOption env vars come first so that they take
		// precedence over defaults when the caller overrides a variable such as
		// DD_HOSTNAME (Go's os.Getenv returns the first match).
		var envVars corev1.EnvVarArray
		envVars = append(envVars, acfg.extraEnvVars...)
		envVars = append(envVars,
			&corev1.EnvVarArgs{
				Name:  pulumi.String("DD_API_KEY"),
				Value: e.AgentAPIKey(),
			},
			&corev1.EnvVarArgs{
				Name:  pulumi.String("DD_OTEL_STANDALONE"),
				Value: pulumi.String("true"),
			},
			&corev1.EnvVarArgs{
				Name:  pulumi.String("DD_OTELCOLLECTOR_ENABLED"),
				Value: pulumi.String("true"),
			},
			&corev1.EnvVarArgs{
				Name:  pulumi.String("DD_KUBELET_TLS_VERIFY"),
				Value: pulumi.String("false"),
			},
			&corev1.EnvVarArgs{
				Name: pulumi.String("DD_KUBERNETES_KUBELET_HOST"),
				ValueFrom: &corev1.EnvVarSourceArgs{
					FieldRef: &corev1.ObjectFieldSelectorArgs{
						FieldPath: pulumi.String("status.hostIP"),
					},
				},
			},
		)
		if !acfg.skipDefaultHostname {
			// Provide an explicit hostname so standalone mode does not have to
			// wait for workloadmeta to resolve the node name at startup.
			// Callers that need to set their own DD_HOSTNAME (e.g. for ENC[]
			// secrets resolution tests) should pass WithoutDefaultHostname() to
			// prevent this entry from shadowing their value.
			envVars = append(envVars, &corev1.EnvVarArgs{
				Name: pulumi.String("DD_HOSTNAME"),
				ValueFrom: &corev1.EnvVarSourceArgs{
					FieldRef: &corev1.ObjectFieldSelectorArgs{
						FieldPath: pulumi.String("spec.nodeName"),
					},
				},
			})
		}
		// When testing against fakeintake, route the agent serializer (used for
		// dogtelextension liveness metrics) to the fakeintake URL so that metrics
		// emitted via the DD serializer pipeline are captured by the test.
		if fakeIntake != nil {
			envVars = append(envVars, &corev1.EnvVarArgs{
				Name:  pulumi.String("DD_DD_URL"),
				Value: fakeIntake.URL,
			})
		}

		_, err = appsv1.NewDaemonSet(e.Ctx(), "standalone-otel-agent", &appsv1.DaemonSetArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("standalone-otel-agent"),
				Namespace: pulumi.String(namespace),
			},
			Spec: &appsv1.DaemonSetSpecArgs{
				Selector: &metav1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{
						"app": pulumi.String(appLabel),
					},
				},
				Template: &corev1.PodTemplateSpecArgs{
					Metadata: &metav1.ObjectMetaArgs{
						Labels: pulumi.StringMap{
							"app": pulumi.String(appLabel),
						},
					},
					Spec: &corev1.PodSpecArgs{
						ServiceAccountName: pulumi.String(saName),
						ImagePullSecrets:   imagePullSecrets,
						Containers: corev1.ContainerArray{
							&corev1.ContainerArgs{
								Name:    pulumi.String("otel-agent"),
								Image:   pulumi.String(imagePath),
								Command: pulumi.StringArray{pulumi.String(binaryPath)},
								Args: pulumi.StringArray{
									pulumi.String("--config"),
									pulumi.String(configPath),
								},
								Env: envVars,
								VolumeMounts: append(corev1.VolumeMountArray{
									&corev1.VolumeMountArgs{
										Name:      pulumi.String("otel-config"),
										MountPath: pulumi.String(configDir),
										ReadOnly:  pulumi.BoolPtr(true),
									},
								}, acfg.extraVolumeMounts...),
								Resources: &corev1.ResourceRequirementsArgs{
									Limits: pulumi.StringMap{
										"cpu":    pulumi.String("500m"),
										"memory": pulumi.String("512Mi"),
									},
									Requests: pulumi.StringMap{
										"cpu":    pulumi.String("200m"),
										"memory": pulumi.String("256Mi"),
									},
								},
							},
						},
						Volumes: append(corev1.VolumeArray{
							&corev1.VolumeArgs{
								Name: pulumi.String("otel-config"),
								ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
									Name: cm.Metadata.Name(),
								},
							},
						}, acfg.extraVolumes...),
					},
				},
			},
		}, append(nsOpts, utils.PulumiDependsOn(cr), utils.PulumiDependsOn(sa))...)
		if err != nil {
			return err
		}

		// Wire up the KubernetesAgent output so test utilities can find the pod.
		comp.LinuxNodeAgent, err = componentskube.NewKubernetesObjRef(
			e,
			"standalone-otel-agent-nodeAgent",
			namespace,
			"Pod",
			pulumi.String("").ToStringOutput(), // appVersion — not applicable for raw DaemonSet
			pulumi.String("").ToStringOutput(), // version   — not applicable for raw DaemonSet
			map[string]string{
				"app": appLabel,
			},
		)
		return err
	})
}

// buildConfigMapData returns a Pulumi map whose single key is the OTel config
// file name. If fakeIntake is set, the datadog exporter endpoints are merged in
// so that telemetry is captured by the fakeintake during the test.
func buildConfigMapData(otelConfig string, fakeIntake *fakeintake.Fakeintake) (pulumi.StringMapInput, error) {
	if fakeIntake == nil {
		return pulumi.StringMap{configKey: pulumi.String(otelConfig)}, nil
	}

	// Merge fakeintake URL at apply time (URL is a Pulumi output).
	merged := fakeIntake.URL.ApplyT(func(url string) (map[string]string, error) {
		override := map[string]any{
			"exporters": map[string]any{
				"datadog": map[string]any{
					"metrics": map[string]any{"endpoint": url},
					"traces":  map[string]any{"endpoint": url},
					"logs":    map[string]any{"endpoint": url},
				},
			},
		}
		base := map[string]any{}
		if err := yaml.Unmarshal([]byte(otelConfig), &base); err != nil {
			return nil, fmt.Errorf("parse otelConfig: %w", err)
		}
		merged := utils.MergeMaps(base, override, false)
		out, err := yaml.Marshal(merged)
		if err != nil {
			return nil, fmt.Errorf("marshal merged otelConfig: %w", err)
		}
		return map[string]string{configKey: string(out)}, nil
	}).(pulumi.StringMapOutput)

	return merged, nil
}

// dockerOTelAgentFullImagePath returns the agent image to use for the standalone
// otel-agent DaemonSet.  It reuses the same image-selection logic as the Helm
// path: CI uses the pipeline QA image, local runs fall back to the nightly OTel
// image.
func dockerOTelAgentFullImagePath(e config.Env) string {
	if e.AgentFullImagePath() != "" {
		return e.AgentFullImagePath()
	}

	if e.PipelineID() != "" && e.CommitSHA() != "" {
		tag := fmt.Sprintf("%s-%s-7-full", e.PipelineID(), e.CommitSHA())
		exists, err := e.InternalRegistryImageTagExists(fmt.Sprintf("%s/agent-qa", e.InternalRegistry()), tag)
		if err != nil || !exists {
			panic(fmt.Sprintf("image %s/agent-qa:%s not found in the internal registry", e.InternalRegistry(), tag))
		}
		return utils.BuildDockerImagePath(fmt.Sprintf("%s/agent-qa", e.InternalRegistry()), tag)
	}

	return utils.BuildDockerImagePath("datadog/agent-dev", "nightly-full-main-jmx")
}
