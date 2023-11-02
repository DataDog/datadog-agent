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

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

var GitCommit string

type k8sSuite struct {
	baseSuite

	KubeClusterName             string
	AgentLinuxHelmInstallName   string
	AgentWindowsHelmInstallName string

	K8sConfig *restclient.Config
	K8sClient *kubernetes.Clientset
}

func (suite *k8sSuite) SetupSuite() {
	suite.clusterName = suite.KubeClusterName

	suite.baseSuite.SetupSuite()
}

func (suite *k8sSuite) TearDownSuite() {
	suite.baseSuite.TearDownSuite()

	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	suite.T().Log(c("The data produced and asserted by these tests can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/qcp-brm-ysc/e2e-tests-containers-k8s?refresh_mode=paused&tpl_var_kube_cluster_name%%5B0%%5D=%s&from_ts=%d&to_ts=%d&live=false",
		suite.KubeClusterName,
		suite.startTime.UnixMilli(),
		suite.endTime.UnixMilli(),
	))
}

// Once pulumi has finished to create a stack, it can still take some time for the images to be pulled,
// for the containers to be started, for the agent collectors to collect workload information
// and to feed workload meta and the tagger.
//
// We could increase the timeout of all tests to cope with the agent tagger warmup time.
// But in case of a single bug making a single tag missing from every metric,
// all the tests would time out and that would be a waste of time.
//
// It’s better to have the first test having a long timeout to wait for the agent to warmup,
// and to have the following tests with a smaller timeout.
//
// Inside a testify test suite, tests are executed in alphabetical order.
// The 00 in Test00UpAndRunning is here to guarantee that this test, waiting for the agent pods to be ready
// is run first.
func (suite *k8sSuite) Test00UpAndRunning() {
	ctx := context.Background()

	suite.Run("agent pods are ready and not restarting", func() {
		suite.EventuallyWithTf(func(collect *assert.CollectT) {

			linuxNodes, err := suite.K8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "linux").String(),
			})
			if err != nil {
				collect.Errorf("Failed to list Linux nodes: %w", err)
				return
			}

			windowsNodes, err := suite.K8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "windows").String(),
			})
			if err != nil {
				collect.Errorf("Failed to list Windows nodes: %w", err)
				return
			}

			linuxPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog").String(),
			})
			if err != nil {
				collect.Errorf("Failed to list Linux datadog agent pods: %w", err)
				return
			}

			windowsPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.AgentWindowsHelmInstallName+"-datadog").String(),
			})
			if err != nil {
				collect.Errorf("Failed to list Windows datadog agent pods: %w", err)
				return
			}

			clusterAgentPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog-cluster-agent").String(),
			})
			if err != nil {
				collect.Errorf("Failed to list datadog cluster agent pods: %w", err)
				return
			}

			clusterChecksPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog-clusterchecks").String(),
			})
			if err != nil {
				collect.Errorf("Failed to list datadog cluster checks runner pods: %w", err)
				return
			}

			if len(linuxPods.Items) != len(linuxNodes.Items) {
				collect.Errorf("There is only %d Linux datadog agent pods for %d Linux nodes.", len(linuxPods.Items), len(linuxNodes.Items))
			}
			if len(windowsPods.Items) != len(windowsNodes.Items) {
				collect.Errorf("There is only %d Windows datadog agent pods for %d Windows nodes.", len(windowsPods.Items), len(windowsNodes.Items))
			}
			if len(clusterAgentPods.Items) == 0 {
				collect.Errorf("There isn’t any cluster agent pod.")
			}
			if len(clusterChecksPods.Items) == 0 {
				collect.Errorf("There isn’t any cluster checks worker pod.")
			}

			for _, podList := range []*corev1.PodList{linuxPods, windowsPods, clusterAgentPods, clusterChecksPods} {
				for _, pod := range podList.Items {
					for _, containerStatus := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
						if !containerStatus.Ready {
							collect.Errorf("Container %s of pod %s isn’t ready.", containerStatus.Name, pod.Name)
						}
						if containerStatus.RestartCount > 0 {
							collect.Errorf("Container %s of pod %s has restarted %d times.", containerStatus.Name, pod.Name, containerStatus.RestartCount)
						}
					}
				}
			}
		}, 5*time.Minute, 10*time.Second, "Not all agents eventually became ready in time.")
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
	// `nginx` check is configured via AD annotation on pods
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "nginx.net.request_per_s",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:nginx$`,
				`^display_container_name:nginx`,
				`^git\.commit\.sha:`, // org.opencontainers.image.revision docker image label
				`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:ghcr\.io/datadog/apps-nginx-server@sha256:`,
				`^image_name:ghcr\.io/datadog/apps-nginx-server$`,
				`^image_tag:main$`,
				`^kube_container_name:nginx$`,
				`^kube_deployment:nginx$`,
				`^kube_namespace:workload-nginx$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:nginx-[[:alnum:]]+$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:nginx-[[:alnum:]]+$`,
				`^kube_service:nginx$`,
				`^pod_name:nginx-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^short_image:apps-nginx-server$`,
			},
		},
	})

	// `http_check` is configured via AD annotation on service
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "network.http.response_time",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^cluster_name:`,
				`^instance:My_Nginx$`,
				`^kube_cluster_name:`,
				`^kube_namespace:workload-nginx$`,
				`^kube_service:nginx$`,
				`^url:http://`,
			},
		},
	})

	// Test KSM metrics for the nginx deployment
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kubernetes_state.deployment.replicas_available",
			Tags: []string{
				"kube_deployment:nginx",
				"kube_namespace:workload-nginx",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^kube_cluster_name:`,
				`^kube_deployment:nginx$`,
				`^kube_namespace:workload-nginx$`,
			},
		},
	})

	// Check HPA is properly scaling up and down
	// This indirectly tests the cluster-agent external metrics server
	suite.testHPA("workload-nginx", "nginx")
}

func (suite *k8sSuite) TestRedis() {
	// `redis` check is auto-configured due to image name
	// Test it is properly scheduled
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "redis.net.instantaneous_ops_per_sec",
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:redis$`,
				`^display_container_name:redis`,
				`^image_id:docker.io/library/redis@sha256:`,
				`^image_name:redis$`,
				`^image_tag:latest$`,
				`^kube_container_name:redis$`,
				`^kube_deployment:redis$`,
				`^kube_namespace:workload-redis$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:redis-[[:alnum:]]+$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:redis-[[:alnum:]]+$`,
				`^kube_service:redis$`,
				`^pod_name:redis-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^redis_host:`,
				`^redis_port:6379$`,
				`^redis_role:master$`,
				`^short_image:redis$`,
			},
		},
	})

	// Test KSM metrics for the redis deployment
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kubernetes_state.deployment.replicas_available",
			Tags: []string{
				"kube_deployment:redis",
				"kube_namespace:workload-redis",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^kube_cluster_name:`,
				`^kube_deployment:redis$`,
				`^kube_namespace:workload-redis$`,
			},
		},
	})

	// Check HPA is properly scaling up and down
	// This indirectly tests the cluster-agent external metrics server
	suite.testHPA("workload-redis", "redis")
}

func (suite *k8sSuite) TestDogstatsd() {
	// Test dogstatsd origin detection with UDS
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				"kube_deployment:dogstatsd-uds",
				"kube_namespace:workload-dogstatsd",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:dogstatsd$`,
				`^display_container_name:dogstatsd`,
				`^git.commit.sha:`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:ghcr.io/datadog/apps-dogstatsd@sha256:`,
				`^image_name:ghcr.io/datadog/apps-dogstatsd$`,
				`^image_tag:main$`,
				`^kube_container_name:dogstatsd$`,
				`^kube_deployment:dogstatsd-uds$`,
				`^kube_namespace:workload-dogstatsd$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:dogstatsd-uds-[[:alnum:]]+$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:dogstatsd-uds-[[:alnum:]]+$`,
				`^pod_name:dogstatsd-uds-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^series:`,
				`^short_image:apps-dogstatsd$`,
			},
		},
	})

	// Test dogstatsd origin detection with UDP
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				"kube_deployment:dogstatsd-udp",
				"kube_namespace:workload-dogstatsd",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^kube_deployment:dogstatsd-udp$`,
				`^kube_namespace:workload-dogstatsd$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:dogstatsd-udp-[[:alnum:]]+$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:dogstatsd-udp-[[:alnum:]]+$`,
				`^pod_name:dogstatsd-udp-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^series:`,
			},
		},
	})
}

func (suite *k8sSuite) TestPrometheus() {
	// Test Prometheus check
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "prom_gauge",
			Tags: []string{
				"kube_deployment:prometheus",
				"kube_namespace:workload-prometheus",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:prometheus$`,
				`^display_container_name:prometheus`,
				`^endpoint:http://.*:8080/metrics$`,
				`^git.commit.sha:`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source   docker image label
				`^image_id:ghcr.io/datadog/apps-prometheus@sha256:`,
				`^image_name:ghcr.io/datadog/apps-prometheus$`,
				`^image_tag:main$`,
				`^kube_container_name:prometheus$`,
				`^kube_deployment:prometheus$`,
				`^kube_namespace:workload-prometheus$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:prometheus-[[:alnum:]]+$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:prometheus-[[:alnum:]]+$`,
				`^pod_name:prometheus-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^series:`,
				`^short_image:apps-prometheus$`,
			},
		},
	})
}

func (suite *k8sSuite) testHPA(namespace, deployment string) {
	suite.Run(fmt.Sprintf("kubernetes_state.deployment.replicas_available{kube_namespace:%s,kube_deployment:%s}", namespace, deployment), func() {
		sendEvent := func(alertType, text string, time *int) {
			if _, err := suite.datadogClient.PostEvent(&datadog.Event{
				Title: pointer.Ptr(fmt.Sprintf("testHPA %s/%s", namespace, deployment)),
				Text: pointer.Ptr(fmt.Sprintf(`%%%%%%
%s
 %%%%%%`, text)),
				Time:      time,
				AlertType: &alertType,
				Tags: []string{
					"app:agent-new-e2e-tests-containers",
					"cluster_name:" + suite.clusterName,
					"metric:kubernetes_state.deployment.replicas_available",
					"filter_tag_kube_namespace:" + namespace,
					"filter_tag_kube_deployment:" + deployment,
					"test:" + suite.T().Name(),
				},
			}); err != nil {
				suite.T().Logf("Failed to post event: %s", err)
			}
		}

		defer func() {
			if suite.T().Failed() {
				sendEvent("error", "Failed to witness scale up *and* scale down events.", nil)
			} else {
				sendEvent("success", "Scale up and scale down events detected.", nil)
			}
		}()

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
				sendEvent("error", fmt.Sprintf("No `kubernetes_state.deployment.replicas_available{kube_namespace:%s,kube_deployment:%s}` metrics yet", namespace, deployment), nil)
				return
			}

			// Check HPA is properly scaling up and down
			// This indirectly tests the cluster-agent external metrics server
			scaleUp := false
			scaleDown := false
			prevValue := -1.0
		out:
			for _, metric := range metrics {
				for _, point := range metric.GetPoints() {
					if prevValue == -1.0 {
						prevValue = point.Value
						continue
					}

					if point.Value > prevValue+0.5 {
						scaleUp = true
						sendEvent("success", "Scale up detected.", pointer.Ptr(int(point.Timestamp)))
						if scaleDown {
							break out
						}
					} else if point.Value < prevValue-0.5 {
						scaleDown = true
						sendEvent("success", "Scale down detected.", pointer.Ptr(int(point.Timestamp)))
						if scaleUp {
							break out
						}
					}
					prevValue = point.Value
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
