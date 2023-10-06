// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

var GitCommit string

type k8sSuite struct {
	suite.Suite

	AgentLinuxHelmInstallName   string
	AgentWindowsHelmInstallName string

	Fakeintake *fakeintake.Client
	K8sConfig  *restclient.Config
	K8sClient  *kubernetes.Clientset
}

func (suite *k8sSuite) TestAgent() {
	ctx := context.Background()

	suite.Run("agent pods are ready and not restarting", func() {
		linuxNodes, err := suite.K8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "linux").String(),
		})
		suite.NoError(err)

		windowsNodes, err := suite.K8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "windows").String(),
		})
		suite.NoError(err)

		linuxPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog").String(),
		})
		suite.NoError(err)

		windowsPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", suite.AgentWindowsHelmInstallName+"-datadog").String(),
		})
		suite.NoError(err)

		clusterAgentPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog-cluster-agent").String(),
		})
		suite.NoError(err)

		clusterChecksPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog-clusterchecks").String(),
		})
		suite.NoError(err)

		suite.Equalf(len(linuxNodes.Items), len(linuxPods.Items), "There isn’t exactly one Linux pod per Linux node.")
		suite.Equalf(len(windowsNodes.Items), len(windowsPods.Items), "There isn’t exactly one Windows pod per Windows node.")
		suite.Greaterf(len(clusterAgentPods.Items), 0, "There isn’t any cluster agent pod.")
		suite.Greaterf(len(clusterChecksPods.Items), 0, "There isn’t any cluster checks worker pod.")

		for _, podList := range []*corev1.PodList{linuxPods, windowsPods, clusterAgentPods, clusterChecksPods} {
			for _, pod := range podList.Items {
				for _, containerStatus := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
					suite.Truef(containerStatus.Ready, "Container %s of pod %s isn’t ready", containerStatus.Name, pod.Name)
					suite.EqualValuesf(containerStatus.RestartCount, 0, "Container %s of pod %s has restarted %d times.", containerStatus.Name, pod.Name, containerStatus.RestartCount)
				}
			}
		}

	})

	versionExtractor := regexp.MustCompile(`Commit: ([[:xdigit:]]+)`)

	for _, tt := range []struct {
		podType     string
		appSelector string
		container   string
	}{
		{
			"Linux agent",
			suite.AgentLinuxHelmInstallName + "-datadog",
			"agent",
		},
		{
			"Windows agent",
			suite.AgentWindowsHelmInstallName + "-datadog",
			"agent",
		},
		{
			"cluster agent",
			suite.AgentLinuxHelmInstallName + "-datadog-cluster-agent",
			"cluster-agent",
		},
		{
			"cluster checks",
			suite.AgentLinuxHelmInstallName + "-datadog-clusterchecks",
			"agent",
		},
	} {
		suite.Run(tt.podType+" pods are running the good version", func() {
			linuxPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", tt.appSelector).String(),
				Limit:         1,
			})
			if suite.NoError(err) && len(linuxPods.Items) >= 1 {
				stdout, stderr, err := suite.podExec("datadog", linuxPods.Items[0].Name, tt.container, []string{"agent", "version"})
				if suite.NoError(err) {
					suite.Equalf(stderr, "", "Standard error of `agent version` should be empty,")
					match := versionExtractor.FindStringSubmatch(stdout)
					if suite.Equalf(2, len(match), "'Commit' not found in the output of `agent version`.") {
						if len(GitCommit) == 10 && len(match[1]) == 7 {
							suite.Equalf(GitCommit[:7], match[1], "Agent isn’t running the expected version")
						} else {
							suite.Equalf(GitCommit, match[1], "Agent isn’t running the expected version")
						}
					}
				}
			}
		})
	}
}

func (suite *k8sSuite) TestNginx() {
	// suite.T().Parallel()

	// `nginx` check is configured via AD annotation on pods
	// Test it is properly scheduled
	suite.testMetric("nginx.net.request_per_s",
		[]string{},
		[]*regexp.Regexp{
			regexp.MustCompile(`^container_id:`),
			regexp.MustCompile(`^container_name:nginx$`),
			regexp.MustCompile(`^display_container_name:nginx`),
			regexp.MustCompile(`^git\.commit\.sha:`),                                                       // org.opencontainers.image.revision docker image label
			regexp.MustCompile(`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`), // org.opencontainers.image.source   docker image label
			regexp.MustCompile(`^image_id:ghcr\.io/datadog/apps-nginx-server@sha256:`),
			regexp.MustCompile(`^image_name:ghcr\.io/datadog/apps-nginx-server$`),
			regexp.MustCompile(`^image_tag:main$`),
			regexp.MustCompile(`^kube_container_name:nginx$`),
			regexp.MustCompile(`^kube_deployment:nginx$`),
			regexp.MustCompile(`^kube_namespace:workload-nginx$`),
			regexp.MustCompile(`^kube_ownerref_kind:replicaset$`),
			regexp.MustCompile(`^kube_ownerref_name:nginx-[[:alnum:]]+$`),
			regexp.MustCompile(`^kube_qos:Burstable$`),
			regexp.MustCompile(`^kube_replica_set:nginx-[[:alnum:]]+$`),
			regexp.MustCompile(`^kube_service:nginx$`),
			regexp.MustCompile(`^pod_name:nginx-[[:alnum:]]+-[[:alnum:]]+$`),
			regexp.MustCompile(`^pod_phase:running$`),
			regexp.MustCompile(`^short_image:apps-nginx-server$`),
		},
	)

	// `http_check` is configured via AD annotation on service
	// Test it is properly scheduled
	suite.testMetric("network.http.response_time",
		[]string{},
		[]*regexp.Regexp{
			regexp.MustCompile(`^cluster_name:`),
			regexp.MustCompile(`^instance:My_Nginx$`),
			regexp.MustCompile(`^kube_cluster_name:`),
			regexp.MustCompile(`^kube_namespace:workload-nginx$`),
			regexp.MustCompile(`^kube_service:nginx$`),
			regexp.MustCompile(`^url:http://`),
		},
	)

	// Test KSM metrics for the nginx deployment
	suite.testMetric("kubernetes_state.deployment.replicas_available",
		[]string{
			"kube_deployment:nginx",
			"kube_namespace:workload-nginx",
		},
		[]*regexp.Regexp{
			regexp.MustCompile(`^kube_cluster_name:`),
			regexp.MustCompile(`^kube_deployment:nginx$`),
			regexp.MustCompile(`^kube_namespace:workload-nginx$`),
		},
	)

	// Check HPA is properly scaling up and down
	// This indirectly tests the cluster-agent external metrics server
	suite.testHPA("workload-nginx", "nginx")
}

func (suite *k8sSuite) TestRedis() {
	// suite.T().Parallel()

	// `redis` check is auto-configured due to image name
	// Test it is properly scheduled
	suite.testMetric("redis.net.instantaneous_ops_per_sec",
		[]string{},
		[]*regexp.Regexp{
			regexp.MustCompile(`^container_id:`),
			regexp.MustCompile(`^container_name:redis$`),
			regexp.MustCompile(`^display_container_name:redis`),
			regexp.MustCompile(`^image_id:docker.io/library/redis@sha256:`),
			regexp.MustCompile(`^image_name:redis$`),
			regexp.MustCompile(`^image_tag:latest$`),
			regexp.MustCompile(`^kube_container_name:redis$`),
			regexp.MustCompile(`^kube_deployment:redis$`),
			regexp.MustCompile(`^kube_namespace:workload-redis$`),
			regexp.MustCompile(`^kube_ownerref_kind:replicaset$`),
			regexp.MustCompile(`^kube_ownerref_name:redis-[[:alnum:]]+$`),
			regexp.MustCompile(`^kube_qos:Burstable$`),
			regexp.MustCompile(`^kube_replica_set:redis-[[:alnum:]]+$`),
			regexp.MustCompile(`^kube_service:redis$`),
			regexp.MustCompile(`^pod_name:redis-[[:alnum:]]+-[[:alnum:]]+$`),
			regexp.MustCompile(`^pod_phase:running$`),
			regexp.MustCompile(`^redis_host:`),
			regexp.MustCompile(`^redis_port:6379$`),
			regexp.MustCompile(`^redis_role:master$`),
			regexp.MustCompile(`^short_image:redis$`),
		},
	)

	// Test KSM metrics for the redis deployment
	suite.testMetric("kubernetes_state.deployment.replicas_available",
		[]string{
			"kube_deployment:redis",
			"kube_namespace:workload-redis",
		},
		[]*regexp.Regexp{
			regexp.MustCompile(`^kube_cluster_name:`),
			regexp.MustCompile(`^kube_deployment:redis$`),
			regexp.MustCompile(`^kube_namespace:workload-redis$`),
		},
	)

	// Check HPA is properly scaling up and down
	// This indirectly tests the cluster-agent external metrics server
	suite.testHPA("workload-redis", "redis")
}

func (suite *k8sSuite) TestDogstatsd() {
	// suite.T().Parallel()

	// Test dogstatsd origin detection with UDS
	suite.testMetric("custom.metric",
		[]string{
			"kube_deployment:dogstatsd-uds",
			"kube_namespace:workload-dogstatsd",
		},
		[]*regexp.Regexp{
			regexp.MustCompile(`^container_id:`),
			regexp.MustCompile(`^container_name:dogstatsd$`),
			regexp.MustCompile(`^display_container_name:dogstatsd`),
			regexp.MustCompile(`^git.commit.sha:`),                                                       // org.opencontainers.image.revision docker image label
			regexp.MustCompile(`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`), // org.opencontainers.image.source   docker image label
			regexp.MustCompile(`^image_id:ghcr.io/datadog/apps-dogstatsd@sha256:`),
			regexp.MustCompile(`^image_name:ghcr.io/datadog/apps-dogstatsd$`),
			regexp.MustCompile(`^image_tag:main$`),
			regexp.MustCompile(`^kube_container_name:dogstatsd$`),
			regexp.MustCompile(`^kube_deployment:dogstatsd-uds$`),
			regexp.MustCompile(`^kube_namespace:workload-dogstatsd$`),
			regexp.MustCompile(`^kube_ownerref_kind:replicaset$`),
			regexp.MustCompile(`^kube_ownerref_name:dogstatsd-uds-[[:alnum:]]+$`),
			regexp.MustCompile(`^kube_qos:Burstable$`),
			regexp.MustCompile(`^kube_replica_set:dogstatsd-uds-[[:alnum:]]+$`),
			regexp.MustCompile(`^pod_name:dogstatsd-uds-[[:alnum:]]+-[[:alnum:]]+$`),
			regexp.MustCompile(`^pod_phase:running$`),
			regexp.MustCompile(`^series:`),
			regexp.MustCompile(`^short_image:apps-dogstatsd$`),
		},
	)

	// Test dogstatsd origin detection with UDP
	suite.testMetric("custom.metric",
		[]string{
			"kube_deployment:dogstatsd-udp",
			"kube_namespace:workload-dogstatsd",
		},
		[]*regexp.Regexp{
			regexp.MustCompile(`^kube_deployment:dogstatsd-udp$`),
			regexp.MustCompile(`^kube_namespace:workload-dogstatsd$`),
			regexp.MustCompile(`^kube_ownerref_kind:replicaset$`),
			regexp.MustCompile(`^kube_ownerref_name:dogstatsd-udp-[[:alnum:]]+$`),
			regexp.MustCompile(`^kube_qos:Burstable$`),
			regexp.MustCompile(`^kube_replica_set:dogstatsd-udp-[[:alnum:]]+$`),
			regexp.MustCompile(`^pod_name:dogstatsd-udp-[[:alnum:]]+-[[:alnum:]]+$`),
			regexp.MustCompile(`^pod_phase:running$`),
			regexp.MustCompile(`^series:`),
		},
	)
}

func (suite *k8sSuite) TestPrometheus() {
	// suite.T().Parallel()

	// Test Prometheus check
	suite.testMetric("prom_gauge",
		[]string{
			"kube_deployment:prometheus",
			"kube_namespace:workload-prometheus",
		},
		[]*regexp.Regexp{
			regexp.MustCompile(`^container_id:`),
			regexp.MustCompile(`^container_name:prometheus$`),
			regexp.MustCompile(`^display_container_name:prometheus`),
			regexp.MustCompile(`^endpoint:http://.*:8080/metrics$`),
			regexp.MustCompile(`^git.commit.sha:`),                                                       // org.opencontainers.image.revision docker image label
			regexp.MustCompile(`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`), // org.opencontainers.image.source   docker image label
			regexp.MustCompile(`^image_id:ghcr.io/datadog/apps-prometheus@sha256:`),
			regexp.MustCompile(`^image_name:ghcr.io/datadog/apps-prometheus$`),
			regexp.MustCompile(`^image_tag:main$`),
			regexp.MustCompile(`^kube_container_name:prometheus$`),
			regexp.MustCompile(`^kube_deployment:prometheus$`),
			regexp.MustCompile(`^kube_namespace:workload-prometheus$`),
			regexp.MustCompile(`^kube_ownerref_kind:replicaset$`),
			regexp.MustCompile(`^kube_ownerref_name:prometheus-[[:alnum:]]+$`),
			regexp.MustCompile(`^kube_qos:Burstable$`),
			regexp.MustCompile(`^kube_replica_set:prometheus-[[:alnum:]]+$`),
			regexp.MustCompile(`^pod_name:prometheus-[[:alnum:]]+-[[:alnum:]]+$`),
			regexp.MustCompile(`^pod_phase:running$`),
			regexp.MustCompile(`^series:`),
			regexp.MustCompile(`^short_image:apps-prometheus$`),
		},
	)
}

func (suite *k8sSuite) testMetric(metricName string, filterTags []string, expectedTags []*regexp.Regexp) {
	suite.Run(fmt.Sprintf("%s{%s}", metricName, strings.Join(filterTags, ",")), func() {
		// suite.T().Parallel()

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			metrics, err := suite.Fakeintake.FilterMetrics(
				metricName,
				fakeintake.WithTags[*aggregator.MetricSeries](filterTags),
			)
			if err != nil {
				collect.Errorf("%w", err)
				return
			}
			if len(metrics) == 0 {
				collect.Errorf("No `%s{%s}` metrics yet", metricName, strings.Join(filterTags, ","))
				return
			}

			// Check tags
			if err := assertTags(metrics[len(metrics)-1].GetTags(), expectedTags); err != nil {
				collect.Errorf("Tags mismatch on `%s`: %w", metricName, err)
				return
			}
		}, 2*time.Minute, 10*time.Second, "Failed finding %s{%s} with proper tags", metricName, strings.Join(filterTags, ","))
	})
}

func (suite *k8sSuite) testHPA(namespace, deployment string) {
	suite.Run(fmt.Sprintf("kubernetes_state.deployment.replicas_available{kube_namespace:%s,kube_deployment:%s}", namespace, deployment), func() {
		// suite.T().Parallel()

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			metrics, err := suite.Fakeintake.FilterMetrics(
				"kubernetes_state.deployment.replicas_available",
				fakeintake.WithTags[*aggregator.MetricSeries]([]string{
					"kube_namespace:" + namespace,
					"kube_deployment:" + deployment,
				}),
			)
			if err != nil {
				collect.Errorf("%w", err)
				return
			}
			if len(metrics) == 0 {
				collect.Errorf("No `kubernetes_state.deployment.replicas_available{kube_namespace:%s,kube_deployment:%s}` metrics yet", namespace, deployment)
				return
			}

			// Check HPA is properly scaling up and down
			// This indirectly tests the cluster-agent external metrics server
			scaleUp := false
			scaleDown := false
			prevValue := 0.0
		out:
			for _, metric := range metrics {
				for _, value := range lo.Map(metric.GetPoints(), func(point *gogen.MetricPayload_MetricPoint, _ int) float64 { return point.GetValue() }) {
					if almostEqual(value-prevValue, 1) {
						scaleUp = true
						if scaleDown {
							break out
						}
					} else if almostEqual(value-prevValue, -1) {
						scaleDown = true
						if scaleUp {
							break out
						}
					}
					prevValue = value
				}
			}
			if !scaleUp {
				collect.Errorf("No scale up detected")
			}
			if !scaleDown {
				collect.Errorf("No scale down detected")
			}
		}, 20*time.Minute, 10*time.Second, "Failed to witness scale up and scale down of %s.%s", namespace, deployment)
	})
}

func (suite *k8sSuite) podExec(namespace, pod, container string, cmd []string) (stdout, stderr string, err error) {
	req := suite.K8sClient.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod).SubResource("exec")
	option := &corev1.PodExecOptions{
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
		Container: container,
		Command:   cmd,
	}

	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(suite.K8sConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}

	var stdoutSb, stderrSb strings.Builder
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdoutSb,
		Stderr: &stderrSb,
	})
	if err != nil {
		return "", "", err
	}

	return stdoutSb.String(), stderrSb.String(), nil
}
