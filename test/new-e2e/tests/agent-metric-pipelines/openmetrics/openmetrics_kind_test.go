// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package openmetrics

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	localkubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	e2eclient "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	fakeintakeaggregator "github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const localAgentImageEnv = "OPENMETRICS_E2E_AGENT_IMAGE"

const (
	configureOutcomeLoaded   = "loaded"
	configureOutcomeFallback = "fallback"
	configureReasonNone      = "none"
)

type kindOpenMetricsSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestKindOpenMetricsCoreLoaderSuite(t *testing.T) {
	t.Parallel()

	agentOptions := []kubernetesagentparams.Option{
		kubernetesagentparams.WithHelmValues(openMetricsCoreLoaderHelmValues),
	}
	provisionerOptions := []localkubernetes.ProvisionerOption{
		localkubernetes.WithName("openmetrics"),
		localkubernetes.WithWorkloadApp(openMetricsK8sAppDefinition),
	}

	if localAgentImage := os.Getenv(localAgentImageEnv); localAgentImage != "" {
		agentOptions = append(agentOptions,
			kubernetesagentparams.WithAgentFullImagePath(localAgentImage),
			kubernetesagentparams.WithHelmValues(openMetricsLocalAgentImageHelmValues),
		)
		provisionerOptions = append(provisionerOptions, localkubernetes.WithKindLoadImage(localAgentImage))
	}

	provisionerOptions = append(provisionerOptions, localkubernetes.WithAgentOptions(agentOptions...))

	e2e.Run(t, &kindOpenMetricsSuite{}, e2e.WithProvisioner(localkubernetes.Provisioner(provisionerOptions...)))
}

func (s *kindOpenMetricsSuite) TestAutodiscoveryInstancesUseCoreLoaderWithAgentFlag() {
	t := s.T()
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assertMetric(c, s, "openmetrics_e2e_one.prom_gauge", fakeintakeclient.WithTags[*fakeintakeaggregator.MetricSeries]([]string{"series:0"}))
		assertMetric(c, s, "openmetrics_e2e_two.prom_gauge", fakeintakeclient.WithTags[*fakeintakeaggregator.MetricSeries]([]string{"series:0"}))
		assertMetric(c, s, "openmetrics_e2e_one.prom_counter.count", fakeintakeclient.WithMetricValueHigherThan(0))
		assertMetric(c, s, "openmetrics_e2e_two.prom_counter.count", fakeintakeclient.WithMetricValueHigherThan(0))

		assertMetric(c, s, "openmetrics_e2e_fixture.target_interval_seconds.sum", fakeintakeclient.WithMetricValueHigherThan(19))
		assertMetric(c, s, "openmetrics_e2e_fixture.target_interval_seconds.count", fakeintakeclient.WithMetricValueHigherThan(1))
		assertMetric(c, s, "openmetrics_e2e_fixture.target_interval_seconds.quantile", fakeintakeclient.WithTags[*fakeintakeaggregator.MetricSeries]([]string{"quantile:0.5"}))
		assertMetric(c, s, "openmetrics_e2e_fixture.go_memstats_alloc_bytes", fakeintakeclient.WithMetricValueHigherThan(100))
		assertMetric(c, s, "openmetrics_e2e_fixture.http_req_duration_seconds.sum", fakeintakeclient.WithMetricValueHigherThan(1))
		assertMetric(c, s, "openmetrics_e2e_fixture.http_req_duration_seconds.count", fakeintakeclient.WithMetricValueHigherThan(3))
		assertMetric(c, s, "openmetrics_e2e_fixture.go_memstats_mallocs_total", fakeintakeclient.WithMetricValueHigherThan(0))
		assertMetric(c, s, "openmetrics_e2e_fallback.go_memstats_alloc_bytes", fakeintakeclient.WithMetricValueHigherThan(100))
	}, 5*time.Minute, 10*time.Second)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		telemetry, err := agentTelemetry(s, s.Env().Agent.LinuxNodeAgent)
		if !assert.NoError(c, err, "node Agent telemetry") {
			return
		}

		clusterChecksTelemetry, err := agentTelemetry(s, s.Env().Agent.LinuxClusterChecks)
		if !assert.NoError(c, err, "Cluster Checks Runner telemetry") {
			return
		}

		telemetry += "\n" + clusterChecksTelemetry
		assert.GreaterOrEqual(c, openMetricsConfigureTelemetryCount(telemetry, configureOutcomeLoaded, configureReasonNone), float64(3))
		assert.GreaterOrEqual(c, openMetricsConfigureTelemetryOutcomeCount(telemetry, configureOutcomeFallback), float64(1))
		assert.True(c, hasOpenMetricsExecutionTelemetry(telemetry, "core"))
		assert.True(c, hasOpenMetricsExecutionTelemetry(telemetry, "python"))
	}, time.Minute, 5*time.Second)
}

func agentTelemetry(s *kindOpenMetricsSuite, agentType componentskube.KubernetesObjRefOutput) (string, error) {
	agentClient, err := e2eclient.NewK8sAgentClient(
		s,
		e2eclient.AgentSelectorAnyPod(agentType),
		s.Env().KubernetesCluster.KubernetesClient,
	)
	if err != nil {
		return "", err
	}

	return agentClient.Diagnose(agentclient.WithArgs([]string{"show-metadata", "agent-full-telemetry"})), nil
}

func assertMetric(c *assert.CollectT, s *kindOpenMetricsSuite, metricName string, options ...fakeintakeclient.MatchOpt[*fakeintakeaggregator.MetricSeries]) {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(metricName, options...)
	assert.NoError(c, err)
	assert.NotEmpty(c, metrics, "no %s metrics found", metricName)
}

func openMetricsConfigureTelemetryCount(telemetry, outcome, reason string) float64 {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)^openmetrics_core__configure(?:_total)?\{outcome=%q,reason=%q\}\s+([0-9]+(?:\.[0-9]+)?)$`, outcome, reason))
	return sumTelemetryMatches(telemetry, pattern)
}

func openMetricsConfigureTelemetryOutcomeCount(telemetry, outcome string) float64 {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)^openmetrics_core__configure(?:_total)?\{outcome=%q,reason="[^"]+"\}\s+([0-9]+(?:\.[0-9]+)?)$`, outcome))
	return sumTelemetryMatches(telemetry, pattern)
}

func sumTelemetryMatches(telemetry string, pattern *regexp.Regexp) float64 {
	var total float64
	for _, match := range pattern.FindAllStringSubmatch(telemetry, -1) {
		value, err := strconv.ParseFloat(match[1], 64)
		if err == nil {
			total += value
		}
	}
	return total
}

func hasOpenMetricsExecutionTelemetry(telemetry, loader string) bool {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)^checks__execution_time\{(?:check_loader=%q,check_name="openmetrics"|check_name="openmetrics",check_loader=%q)\}\s+[0-9]+(?:\.[0-9]+)?$`, loader, loader))
	return pattern.MatchString(telemetry)
}

func openMetricsK8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider) (*componentskube.Workload, error) {
	opts := []pulumi.ResourceOption{pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider)}

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "openmetrics-e2e", k8sComponent, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(k8sComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), "openmetrics-e2e", &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String("openmetrics-e2e"),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	opts = append(opts, utils.PulumiDependsOn(ns))

	var imagePullSecrets corev1.LocalObjectReferenceArray
	if e.ImagePullRegistry() != "" {
		imgPullSecret, err := utils.NewImagePullSecret(e, "openmetrics-e2e", opts...)
		if err != nil {
			return nil, err
		}
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReferenceArgs{
			Name: imgPullSecret.Metadata.Name(),
		})
	}

	for _, workload := range []struct {
		name      string
		namespace string
	}{
		{name: "openmetrics-one", namespace: "openmetrics_e2e_one"},
		{name: "openmetrics-two", namespace: "openmetrics_e2e_two"},
	} {
		if err := newOpenMetricsDeployment(e, workload.name, workload.namespace, imagePullSecrets, opts...); err != nil {
			return nil, err
		}
	}
	if err := newOpenMetricsFixtureDeployment(e, imagePullSecrets, opts...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}

func newOpenMetricsDeployment(e config.Env, name string, metricNamespace string, imagePullSecrets corev1.LocalObjectReferenceArray, opts ...pulumi.ResourceOption) error {
	labels := pulumi.StringMap{
		"app": pulumi.String(name),
	}

	_, err := appsv1.NewDeployment(e.Ctx(), name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String("openmetrics-e2e"),
			Labels:    labels,
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: labels,
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: labels,
					Annotations: pulumi.StringMap{
						"ad.datadoghq.com/prometheus.checks": pulumi.String(openMetricsADAnnotation(metricNamespace)),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ImagePullSecrets: imagePullSecrets,
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("prometheus"),
							Image: pulumi.String("ghcr.io/datadog/apps-prometheus:" + apps.Version),
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("32Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("10m"),
									"memory": pulumi.String("32Mi"),
								},
							},
							Ports: &corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									Name:          pulumi.String("metrics"),
									ContainerPort: pulumi.Int(8080),
									Protocol:      pulumi.String("TCP"),
								},
							},
							LivenessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path: pulumi.StringPtr("/metrics"),
									Port: pulumi.Int(8080),
								},
							},
							ReadinessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path: pulumi.StringPtr("/metrics"),
									Port: pulumi.Int(8080),
								},
							},
						},
					},
				},
			},
		},
	}, opts...)
	return err
}

func newOpenMetricsFixtureDeployment(e config.Env, imagePullSecrets corev1.LocalObjectReferenceArray, opts ...pulumi.ResourceOption) error {
	labels := pulumi.StringMap{
		"app": pulumi.String("openmetrics-fixture"),
	}
	_, err := appsv1.NewDeployment(e.Ctx(), "openmetrics-fixture", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("openmetrics-fixture"),
			Namespace: pulumi.String("openmetrics-e2e"),
			Labels:    labels,
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: labels,
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: labels,
					Annotations: pulumi.StringMap{
						"ad.datadoghq.com/fixture.checks": pulumi.String(openMetricsFixtureADAnnotation()),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ImagePullSecrets: imagePullSecrets,
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:    pulumi.String("fixture"),
							Image:   pulumi.String(busyboxImage(e)),
							Command: pulumi.StringArray{pulumi.String("sh"), pulumi.String("-c"), pulumi.String(openMetricsFixtureCommand)},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("32Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("10m"),
									"memory": pulumi.String("32Mi"),
								},
							},
							Ports: &corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									Name:          pulumi.String("metrics"),
									ContainerPort: pulumi.Int(8080),
									Protocol:      pulumi.String("TCP"),
								},
							},
							LivenessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path: pulumi.StringPtr("/metrics"),
									Port: pulumi.Int(8080),
								},
							},
							ReadinessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Path: pulumi.StringPtr("/metrics"),
									Port: pulumi.Int(8080),
								},
							},
						},
					},
				},
			},
		},
	}, opts...)
	return err
}

func busyboxImage(e config.Env) string {
	if registry := e.ImagePullRegistry(); registry != "" {
		return strings.SplitN(registry, ",", 2)[0] + "/dockerhub/library/busybox:1.37.0"
	}
	return "docker.io/library/busybox:1.37.0"
}

func openMetricsADAnnotation(namespace string) string {
	return fmt.Sprintf(`{
  "openmetrics": {
    "init_config": {},
    "instances": [
      {
        "openmetrics_endpoint": "http://%%%%host%%%%:8080/metrics",
        "namespace": %q,
        "metrics": ["prom_gauge", "prom_counter"]
      }
    ]
  }
}`, namespace)
}

func openMetricsFixtureADAnnotation() string {
	return `{
  "openmetrics": {
    "init_config": {},
    "instances": [
      {
        "prometheus_url": "http://%%host%%:8080/metrics",
        "namespace": "openmetrics_e2e_fixture",
        "metrics": [
          {"prometheus_target_interval_length_seconds": "target_interval_seconds"},
          {"prometheus_http_request_duration_seconds": "http_req_duration_seconds"},
          "go_memstats_mallocs_total",
          "go_memstats_alloc_bytes"
        ]
      },
      {
        "prometheus_url": "http://%%host%%:8080/metrics",
        "namespace": "openmetrics_e2e_fallback",
        "metrics": ["go_memstats_alloc_bytes"],
        "use_legacy_auth_encoding": true
      }
    ]
  }
}`
}

const openMetricsFixtureCommand = `mkdir -p /www
while true; do
  counter=$(date +%s)
  cat > /www/metrics <<EOF
# HELP prometheus_target_interval_length_seconds Target interval.
# TYPE prometheus_target_interval_length_seconds summary
prometheus_target_interval_length_seconds{quantile="0.5"} 10
prometheus_target_interval_length_seconds_sum 20
prometheus_target_interval_length_seconds_count 2
# HELP prometheus_http_request_duration_seconds HTTP request duration.
# TYPE prometheus_http_request_duration_seconds histogram
prometheus_http_request_duration_seconds_bucket{le="0.5"} 4
prometheus_http_request_duration_seconds_bucket{le="+Inf"} 4
prometheus_http_request_duration_seconds_sum 1.4
prometheus_http_request_duration_seconds_count 4
# HELP go_memstats_mallocs_total Total mallocs.
# TYPE go_memstats_mallocs_total counter
go_memstats_mallocs_total ${counter}
# HELP go_memstats_alloc_bytes Alloc bytes.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 123
EOF
  sleep 1
done &
exec httpd -f -p 8080 -h /www`

const openMetricsCoreLoaderHelmValues = `
datadog:
  envDict:
    DD_OPENMETRICS_USE_CORE_LOADER: 'true'
clusterChecksRunner:
  envDict:
    DD_OPENMETRICS_USE_CORE_LOADER: 'true'
`

const openMetricsLocalAgentImageHelmValues = `
agents:
  image:
    pullPolicy: Never
clusterChecksRunner:
  image:
    pullPolicy: Never
`
