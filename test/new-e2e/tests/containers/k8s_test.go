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

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/agent-payload/v5/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	kubeNamespaceDogstatsWorkload           = "workload-dogstatsd"
	kubeNamespaceDogstatsStandaloneWorkload = "workload-dogstatsd-standalone"
	kubeNamespaceTracegenWorkload           = "workload-tracegen"
	kubeDeploymentDogstatsdUDPOrigin        = "dogstatsd-udp-origin-detection"
	kubeDeploymentDogstatsdUDS              = "dogstatsd-uds"
	kubeDeploymentTracegenTCPWorkload       = "tracegen-tcp"
	kubeDeploymentTracegenUDSWorkload       = "tracegen-uds"
)

var GitCommit string

type k8sSuite struct {
	baseSuite

	KubeClusterName             string
	AgentLinuxHelmInstallName   string
	AgentWindowsHelmInstallName string

	K8sConfig *restclient.Config
	K8sClient kubernetes.Interface
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
	suite.T().Log(c("https://dddev.datadoghq.com/dashboard/qcp-brm-ysc/e2e-tests-containers-k8s?refresh_mode=paused&tpl_var_kube_cluster_name%%5B0%%5D=%s&tpl_var_fake_intake_task_family%%5B0%%5D=%s-fakeintake-ecs&from_ts=%d&to_ts=%d&live=false",
		suite.KubeClusterName,
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
// The 00 in Test00UpAndRunning is here to guarantee that this test, waiting for the agent pods to be ready,
// is run first.
func (suite *k8sSuite) Test00UpAndRunning() {
	suite.testUpAndRunning(10 * time.Minute)
}

// An agent restart (because of a health probe failure or because of a OOM kill for ex.)
// can cause a completely random failure on a completely random test.
// A metric can be fully missing if the agent is restarted when the metric is checked.
// Only a subset of tags can be missing if the agent has just restarted, but not all the
// collectors have finished to feed workload meta and the tagger.
// So, checking if any agent has restarted during the tests can be valuable for investigations.
//
// Inside a testify test suite, tests are executed in alphabetical order.
// The ZZ in TestZZUpAndRunning is here to guarantee that this test, is run last.
func (suite *k8sSuite) TestZZUpAndRunning() {
	suite.testUpAndRunning(1 * time.Minute)
}

func (suite *k8sSuite) testUpAndRunning(waitFor time.Duration) {
	ctx := context.Background()

	suite.Run("agent pods are ready and not restarting", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			linuxNodes, err := suite.K8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "linux").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list Linux nodes") {
				return
			}

			windowsNodes, err := suite.K8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("kubernetes.io/os", "windows").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list Windows nodes") {
				return
			}

			linuxPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list Linux datadog agent pods") {
				return
			}

			windowsPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.AgentWindowsHelmInstallName+"-datadog").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list Windows datadog agent pods") {
				return
			}

			clusterAgentPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog-cluster-agent").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list datadog cluster agent pods") {
				return
			}

			clusterChecksPods, err := suite.K8sClient.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", suite.AgentLinuxHelmInstallName+"-datadog-clusterchecks").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list datadog cluster checks runner pods") {
				return
			}

			dogstatsdPods, err := suite.K8sClient.CoreV1().Pods("dogstatsd-standalone").List(ctx, metav1.ListOptions{
				LabelSelector: fields.OneTermEqualSelector("app", "dogstatsd-standalone").String(),
			})
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to list dogstatsd standalone pods") {
				return
			}

			assert.Len(c, linuxPods.Items, len(linuxNodes.Items))
			assert.Len(c, windowsPods.Items, len(windowsNodes.Items))
			assert.NotEmpty(c, clusterAgentPods.Items)
			assert.NotEmpty(c, clusterChecksPods.Items)
			assert.Len(c, dogstatsdPods.Items, len(linuxNodes.Items))

			for _, podList := range []*corev1.PodList{linuxPods, windowsPods, clusterAgentPods, clusterChecksPods, dogstatsdPods} {
				for _, pod := range podList.Items {
					for _, containerStatus := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
						assert.Truef(c, containerStatus.Ready, "Container %s of pod %s isn’t ready", containerStatus.Name, pod.Name)
						assert.Zerof(c, containerStatus.RestartCount, "Container %s of pod %s has restarted", containerStatus.Name, pod.Name)
					}
				}
			}
		}, waitFor, 10*time.Second, "Not all agents eventually became ready in time.")
	})
}

func (suite *k8sSuite) TestVersion() {
	ctx := context.Background()
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
					suite.Emptyf(stderr, "Standard error of `agent version` should be empty,")
					match := versionExtractor.FindStringSubmatch(stdout)
					if suite.Equalf(2, len(match), "'Commit' not found in the output of `agent version`.") {
						if suite.Greaterf(len(GitCommit), 6, "Couldn’t guess the expected version of the agent.") &&
							suite.Greaterf(len(match[1]), 6, "Couldn’t find the version of the agent.") {

							size2compare := len(GitCommit)
							if len(match[1]) < size2compare {
								size2compare = len(match[1])
							}

							suite.Equalf(GitCommit[:size2compare], match[1][:size2compare], "Agent isn’t running the expected version")
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
				`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source docker image label
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
			AcceptUnexpectedTags: true,
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
				"^kube_deployment:nginx$",
				"^kube_namespace:workload-nginx$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^kube_cluster_name:`,
				`^kube_deployment:nginx$`,
				`^kube_namespace:workload-nginx$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 5,
				Min: 1,
			},
		},
	})

	// Test Nginx logs
	suite.testLog(&testLogArgs{
		Filter: testLogFilterArgs{
			Service: "apps-nginx-server",
		},
		Expect: testLogExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:nginx$`,
				`^dirname:/var/log/pods/workload-nginx_nginx-`,
				`^display_container_name:nginx`,
				`^filename:[[:digit:]]+.log$`,
				`^git\.commit\.sha:`, // org.opencontainers.image.revision docker image label
				`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source docker image label
				`^image_id:ghcr.io/datadog/apps-nginx-server@sha256:`,
				`^image_name:ghcr.io/datadog/apps-nginx-server$`,
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
			Message: `GET / HTTP/1\.1`,
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
				`^image_id:public.ecr.aws/docker/library/redis@sha256:`,
				`^image_name:public.ecr.aws/docker/library/redis$`,
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
				`^short_image:redis$`,
			},
			AcceptUnexpectedTags: true,
		},
	})

	// Test KSM metrics for the redis deployment
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kubernetes_state.deployment.replicas_available",
			Tags: []string{
				"^kube_deployment:redis$",
				"^kube_namespace:workload-redis$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^kube_cluster_name:`,
				`^kube_deployment:redis$`,
				`^kube_namespace:workload-redis$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 5,
				Min: 1,
			},
		},
	})

	// Test Redis logs
	suite.testLog(&testLogArgs{
		Filter: testLogFilterArgs{
			Service: "redis",
		},
		Expect: testLogExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:redis$`,
				`^dirname:/var/log/pods/workload-redis_redis-`,
				`^display_container_name:redis`,
				`^filename:[[:digit:]]+.log$`,
				`^image_id:public.ecr.aws/docker/library/redis@sha256:`,
				`^image_name:public.ecr.aws/docker/library/redis$`,
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
				`^short_image:redis$`,
			},
			Message: `Accepted`,
		},
	})

	// Check HPA is properly scaling up and down
	// This indirectly tests the cluster-agent external metrics server
	suite.testHPA("workload-redis", "redis")
}

func (suite *k8sSuite) TestCPU() {
	// Test CPU metrics
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "container.cpu.usage",
			Tags: []string{
				"^kube_deployment:stress-ng$",
				"^kube_namespace:workload-cpustress$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:stress-ng$`,
				`^display_container_name:stress-ng`,
				`^git.commit.sha:`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/ColinIanKing/stress-ng$`, // org.opencontainers.image.source   docker image label
				`^image_id:ghcr.io/colinianking/stress-ng@sha256:`,
				`^image_name:ghcr.io/colinianking/stress-ng$`,
				`^image_tag:`,
				`^kube_container_name:stress-ng$`,
				`^kube_deployment:stress-ng$`,
				`^kube_namespace:workload-cpustress$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:stress-ng-[[:alnum:]]+$`,
				`^kube_qos:Guaranteed$`,
				`^kube_replica_set:stress-ng-[[:alnum:]]+$`,
				`^pod_name:stress-ng-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^runtime:containerd$`,
				`^short_image:stress-ng$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 155000000,
				Min: 145000000,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "container.cpu.limit",
			Tags: []string{
				"^kube_deployment:stress-ng$",
				"^kube_namespace:workload-cpustress$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:stress-ng$`,
				`^display_container_name:stress-ng`,
				`^git.commit.sha:`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/ColinIanKing/stress-ng$`, // org.opencontainers.image.source   docker image label
				`^image_id:ghcr.io/colinianking/stress-ng@sha256:`,
				`^image_name:ghcr.io/colinianking/stress-ng$`,
				`^image_tag:`,
				`^kube_container_name:stress-ng$`,
				`^kube_deployment:stress-ng$`,
				`^kube_namespace:workload-cpustress$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:stress-ng-[[:alnum:]]+$`,
				`^kube_qos:Guaranteed$`,
				`^kube_replica_set:stress-ng-[[:alnum:]]+$`,
				`^pod_name:stress-ng-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^runtime:containerd$`,
				`^short_image:stress-ng$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 200000000,
				Min: 200000000,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kubernetes.cpu.usage.total",
			Tags: []string{
				"^kube_deployment:stress-ng$",
				"^kube_namespace:workload-cpustress$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:stress-ng$`,
				`^display_container_name:stress-ng`,
				`^git.commit.sha:`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/ColinIanKing/stress-ng$`, // org.opencontainers.image.source   docker image label
				`^image_id:ghcr.io/colinianking/stress-ng@sha256:`,
				`^image_name:ghcr.io/colinianking/stress-ng$`,
				`^image_tag:409201de7458c639c68088d28ec8270ef599fe47$`,
				`^kube_container_name:stress-ng$`,
				`^kube_deployment:stress-ng$`,
				`^kube_namespace:workload-cpustress$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:stress-ng-[[:alnum:]]+$`,
				`^kube_qos:Guaranteed$`,
				`^kube_replica_set:stress-ng-[[:alnum:]]+$`,
				`^pod_name:stress-ng-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^short_image:stress-ng$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 250000000,
				Min: 75000000,
			},
		},
	})

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "kubernetes.cpu.limits",
			Tags: []string{
				"^kube_deployment:stress-ng$",
				"^kube_namespace:workload-cpustress$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^container_id:`,
				`^container_name:stress-ng$`,
				`^display_container_name:stress-ng`,
				`^git.commit.sha:`, // org.opencontainers.image.revision docker image label
				`^git.repository_url:https://github.com/ColinIanKing/stress-ng$`, // org.opencontainers.image.source   docker image label
				`^image_id:ghcr.io/colinianking/stress-ng@sha256:`,
				`^image_name:ghcr.io/colinianking/stress-ng$`,
				`^image_tag:409201de7458c639c68088d28ec8270ef599fe47$`,
				`^kube_container_name:stress-ng$`,
				`^kube_deployment:stress-ng$`,
				`^kube_namespace:workload-cpustress$`,
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:stress-ng-[[:alnum:]]+$`,
				`^kube_qos:Guaranteed$`,
				`^kube_replica_set:stress-ng-[[:alnum:]]+$`,
				`^pod_name:stress-ng-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^short_image:stress-ng$`,
			},
			Value: &testMetricExpectValueArgs{
				Max: 0.2,
				Min: 0.2,
			},
		},
	})
}

func (suite *k8sSuite) TestDogstatsdInAgent() {
	// Test with UDS
	suite.testDogstatsdContainerID(kubeNamespaceDogstatsWorkload, kubeDeploymentDogstatsdUDS)
	// Test with UDP + Origin detection
	suite.testDogstatsdContainerID(kubeNamespaceDogstatsWorkload, kubeDeploymentDogstatsdUDPOrigin)
	// Test with UDP + DD_ENTITY_ID
	suite.testDogstatsdPodUID(kubeNamespaceDogstatsWorkload)
}

func (suite *k8sSuite) TestDogstatsdStandalone() {
	// Test with UDS
	suite.testDogstatsdContainerID(kubeNamespaceDogstatsStandaloneWorkload, kubeDeploymentDogstatsdUDS)
	// Dogstatsd standalone does not support origin detection
	// Test with UDP + DD_ENTITY_ID
	suite.testDogstatsdPodUID(kubeNamespaceDogstatsWorkload)
}

func (suite *k8sSuite) testDogstatsdPodUID(kubeNamespace string) {
	// Test dogstatsd origin detection with UDP + DD_ENTITY_ID
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				"^kube_deployment:dogstatsd-udp$",
				"^kube_namespace:" + regexp.QuoteMeta(kubeNamespace) + "$",
			},
		},
		Expect: testMetricExpectArgs{
			Tags: &[]string{
				`^kube_deployment:dogstatsd-udp$`,
				"^kube_namespace:" + regexp.QuoteMeta(kubeNamespace) + "$",
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

func (suite *k8sSuite) testDogstatsdContainerID(kubeNamespace, kubeDeployment string) {
	if kubeDeployment == kubeDeploymentDogstatsdUDPOrigin {
		// CONTINT-3934: This test is flaky, skip it for now
		suite.T().Skipf("Skipping test for %s/%s as it is currently flaky", kubeNamespace, kubeDeployment)
	}
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "custom.metric",
			Tags: []string{
				"^kube_deployment:" + regexp.QuoteMeta(kubeDeployment) + "$",
				"^kube_namespace:" + regexp.QuoteMeta(kubeNamespace) + "$",
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
				`^kube_deployment:` + regexp.QuoteMeta(kubeDeployment) + `$`,
				"^kube_namespace:" + regexp.QuoteMeta(kubeNamespace) + "$",
				`^kube_ownerref_kind:replicaset$`,
				`^kube_ownerref_name:` + regexp.QuoteMeta(kubeDeployment) + `-[[:alnum:]]+$`,
				`^kube_qos:Burstable$`,
				`^kube_replica_set:` + regexp.QuoteMeta(kubeDeployment) + `-[[:alnum:]]+$`,
				`^pod_name:` + regexp.QuoteMeta(kubeDeployment) + `-[[:alnum:]]+-[[:alnum:]]+$`,
				`^pod_phase:running$`,
				`^series:`,
				`^short_image:apps-dogstatsd$`,
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
				"^kube_deployment:prometheus$",
				"^kube_namespace:workload-prometheus$",
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

func (suite *k8sSuite) TestAdmissionController() {
	ctx := context.Background()

	// Delete the pod to ensure it is recreated after the admission controller is deployed
	err := suite.K8sClient.CoreV1().Pods("workload-mutated").DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", "mutated").String(),
	})
	suite.Require().NoError(err)

	// Wait for the fresh pod to be created
	var pod corev1.Pod
	suite.Require().EventuallyWithTf(func(c *assert.CollectT) {
		pods, err := suite.K8sClient.CoreV1().Pods("workload-mutated").List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", "mutated").String(),
		})
		if !assert.NoError(c, err) {
			return
		}
		if !assert.Len(c, pods.Items, 1) {
			return
		}
		pod = pods.Items[0]
	}, 2*time.Minute, 10*time.Second, "Failed to witness the creation of a pod in workload-mutated")

	suite.Require().Len(pod.Spec.Containers, 1)

	// Assert injected env vars
	env := make(map[string]string)
	for _, envVar := range pod.Spec.Containers[0].Env {
		env[envVar.Name] = envVar.Value
	}

	if suite.Contains(env, "DD_DOGSTATSD_URL") {
		suite.Equal("unix:///var/run/datadog/dsd.socket", env["DD_DOGSTATSD_URL"])
	}
	if suite.Contains(env, "DD_TRACE_AGENT_URL") {
		suite.Equal("unix:///var/run/datadog/apm.socket", env["DD_TRACE_AGENT_URL"])
	}
	suite.Contains(env, "DD_ENTITY_ID")
	if suite.Contains(env, "DD_ENV") {
		suite.Equal("e2e", env["DD_ENV"])
	}
	if suite.Contains(env, "DD_SERVICE") {
		suite.Equal("mutated", env["DD_SERVICE"])
	}
	if suite.Contains(env, "DD_VERSION") {
		suite.Equal("v0.0.1", env["DD_VERSION"])
	}

	// Assert injected volumes and mounts
	hostPathVolumes := make(map[string]*corev1.HostPathVolumeSource)
	for _, volume := range pod.Spec.Volumes {
		if volume.HostPath != nil {
			hostPathVolumes[volume.Name] = volume.HostPath
		}
	}

	if suite.Contains(hostPathVolumes, "datadog") {
		suite.Equal("/var/run/datadog", hostPathVolumes["datadog"].Path)
	}

	volumeMounts := make(map[string]string)
	for _, volumeMount := range pod.Spec.Containers[0].VolumeMounts {
		volumeMounts[volumeMount.Name] = volumeMount.MountPath
	}

	if suite.Contains(volumeMounts, "datadog") {
		suite.Equal("/var/run/datadog", volumeMounts["datadog"])
	}
}

func (suite *k8sSuite) TestContainerImage() {
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		images, err := suite.Fakeintake.FilterContainerImages("ghcr.io/datadog/apps-nginx-server")
		// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NoErrorf(c, err, "Failed to query fake intake") {
			return
		}
		// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NotEmptyf(c, images, "No container_image yet") {
			return
		}

		expectedTags := []*regexp.Regexp{
			regexp.MustCompile(`^architecture:(amd|arm)64$`),
			regexp.MustCompile(`^git\.commit\.sha:`),
			regexp.MustCompile(`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`),
			regexp.MustCompile(`^image_id:ghcr\.io/datadog/apps-nginx-server@sha256:`),
			regexp.MustCompile(`^image_name:ghcr\.io/datadog/apps-nginx-server$`),
			regexp.MustCompile(`^image_tag:main$`),
			regexp.MustCompile(`^os_name:linux$`),
			regexp.MustCompile(`^short_image:apps-nginx-server$`),
		}
		err = assertTags(images[len(images)-1].GetTags(), expectedTags, false)
		assert.NoErrorf(c, err, "Tags mismatch")
	}, 2*time.Minute, 10*time.Second, "Failed finding the container image payload")
}

func (suite *k8sSuite) TestSBOM() {
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		sbomIDs, err := suite.Fakeintake.GetSBOMIDs()
		// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NoErrorf(c, err, "Failed to query fake intake") {
			return
		}

		sbomIDs = lo.Filter(sbomIDs, func(id string, _ int) bool {
			return strings.HasPrefix(id, "ghcr.io/datadog/apps-nginx-server")
		})

		// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NotEmptyf(c, sbomIDs, "No SBOM for ghcr.io/datadog/apps-nginx-server yet") {
			return
		}

		images := lo.FlatMap(sbomIDs, func(id string, _ int) []*aggregator.SBOMPayload {
			images, err := suite.Fakeintake.FilterSBOMs(id)
			assert.NoErrorf(c, err, "Failed to query fake intake")
			return images
		})

		// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NotEmptyf(c, images, "No SBOM payload yet") {
			return
		}

		images = lo.Filter(images, func(image *aggregator.SBOMPayload, _ int) bool {
			return image.Status == sbom.SBOMStatus_SUCCESS
		})

		// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NotEmptyf(c, images, "No successful SBOM yet") {
			return
		}

		images = lo.Filter(images, func(image *aggregator.SBOMPayload, _ int) bool {
			cyclonedx := image.GetCyclonedx()
			return cyclonedx != nil &&
				cyclonedx.Metadata != nil &&
				cyclonedx.Metadata.Component != nil
		})

		// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NotEmptyf(c, images, "No SBOM with complete CycloneDX") {
			return
		}

		for _, image := range images {
			if !assert.NotNil(c, image.GetCyclonedx().Metadata.Component.Properties) {
				continue
			}

			expectedTags := []*regexp.Regexp{
				regexp.MustCompile(`^architecture:(amd|arm)64$`),
				regexp.MustCompile(`^git\.commit\.sha:`),
				regexp.MustCompile(`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`),
				regexp.MustCompile(`^image_id:ghcr\.io/datadog/apps-nginx-server@sha256:`),
				regexp.MustCompile(`^image_name:ghcr\.io/datadog/apps-nginx-server$`),
				regexp.MustCompile(`^image_tag:main$`),
				regexp.MustCompile(`^os_name:linux$`),
				regexp.MustCompile(`^short_image:apps-nginx-server$`),
			}
			err = assertTags(image.GetTags(), expectedTags, false)
			assert.NoErrorf(c, err, "Tags mismatch")

			properties := lo.Associate(image.GetCyclonedx().Metadata.Component.Properties, func(property *cyclonedx_v1_4.Property) (string, string) {
				return property.Name, *property.Value
			})

			if assert.Contains(c, properties, "aquasecurity:trivy:RepoTag") {
				assert.Equal(c, "ghcr.io/datadog/apps-nginx-server:main", properties["aquasecurity:trivy:RepoTag"])
			}

			if assert.Contains(c, properties, "aquasecurity:trivy:RepoDigest") {
				assert.Contains(c, properties["aquasecurity:trivy:RepoDigest"], "ghcr.io/datadog/apps-nginx-server@sha256:")
			}
		}
	}, 2*time.Minute, 10*time.Second, "Failed finding the container image payload")
}

func (suite *k8sSuite) TestContainerLifecycleEvents() {
	var nginxPod corev1.Pod

	suite.Require().EventuallyWithTf(func(c *assert.CollectT) {
		pods, err := suite.K8sClient.CoreV1().Pods("workload-nginx").List(context.Background(), metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", "nginx").String(),
		})
		// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NoErrorf(c, err, "Failed to list nginx pods") {
			return
		}
		if !assert.NotEmptyf(c, pods, "Failed to find an nginx pod") {
			return
		}

		nginxPod = pods.Items[0]
	}, 1*time.Minute, 10*time.Second, "Failed to find an nginx pod")

	err := suite.K8sClient.CoreV1().Pods("workload-nginx").Delete(context.Background(), nginxPod.Name, metav1.DeleteOptions{})
	suite.Require().NoError(err)

	suite.EventuallyWithTf(func(c *assert.CollectT) {
		events, err := suite.Fakeintake.GetContainerLifecycleEvents()
		// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NoErrorf(c, err, "Failed to query fake intake") {
			return
		}

		foundPodEvent := false

		for _, event := range events {
			if podEvent := event.GetPod(); podEvent != nil {
				if types.UID(podEvent.GetPodUID()) == nginxPod.UID {
					foundPodEvent = true
					break
				}
			}
		}

		assert.Truef(c, foundPodEvent, "Failed to find the pod lifecycle event")
	}, 2*time.Minute, 10*time.Second, "Failed to find the container lifecycle events")
}

func (suite *k8sSuite) testHPA(namespace, deployment string) {
	suite.Run(fmt.Sprintf("hpa   kubernetes_state.deployment.replicas_available{kube_namespace:%s,kube_deployment:%s}", namespace, deployment), func() {
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

		suite.EventuallyWithTf(func(c *assert.CollectT) {
			metrics, err := suite.Fakeintake.FilterMetrics(
				"kubernetes_state.deployment.replicas_available",
				fakeintake.WithTags[*aggregator.MetricSeries]([]string{
					"kube_namespace:" + namespace,
					"kube_deployment:" + deployment,
				}),
			)
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to query fake intake") {
				return
			}
			if !assert.NotEmptyf(c, metrics, "No `kubernetes_state.deployment.replicas_available{kube_namespace:%s,kube_deployment:%s}` metrics yet", namespace, deployment) {
				sendEvent("warning", fmt.Sprintf("No `kubernetes_state.deployment.replicas_available{kube_namespace:%s,kube_deployment:%s}` metrics yet", namespace, deployment), nil)
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

					if !scaleUp && point.Value > prevValue+0.5 {
						scaleUp = true
						sendEvent("success", "Scale up detected.", pointer.Ptr(int(point.Timestamp)))
						if scaleDown {
							break out
						}
					} else if !scaleDown && point.Value < prevValue-0.5 {
						scaleDown = true
						sendEvent("success", "Scale down detected.", pointer.Ptr(int(point.Timestamp)))
						if scaleUp {
							break out
						}
					}
					prevValue = point.Value
				}
			}
			assert.Truef(c, scaleUp, "No scale up detected")
			assert.Truef(c, scaleDown, "No scale down detected")
			// The deployments that have an HPA configured (nginx and redis)
			// exhibit a traffic pattern that follows a sine wave with a
			// 20-minute period. This is defined in the test-infra-definitions
			// repo. For this reason, the timeout for this test needs to be a
			// bit higher than 20 min to capture the scale down event.
		}, 25*time.Minute, 10*time.Second, "Failed to witness scale up and scale down of %s.%s", namespace, deployment)
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

func (suite *k8sSuite) TestTraceUDS() {
	suite.testTrace(kubeDeploymentTracegenUDSWorkload)
}

func (suite *k8sSuite) TestTraceTCP() {
	suite.testTrace(kubeDeploymentTracegenTCPWorkload)
}

// testTrace verifies that traces are tagged with container and pod tags.
func (suite *k8sSuite) testTrace(kubeDeployment string) {
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		traces, cerr := suite.Fakeintake.GetTraces()
		// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
		if !assert.NoErrorf(c, cerr, "Failed to query fake intake") {
			return
		}

		var err error
		// Iterate starting from the most recent traces
		for _, trace := range traces {
			tags := lo.MapToSlice(trace.Tags, func(k string, v string) string {
				return k + ":" + v
			})
			// Assert origin detection is working properly
			err = assertTags(tags, []*regexp.Regexp{
				regexp.MustCompile(`^container_id:`),
				regexp.MustCompile(`^container_name:` + kubeDeployment + `$`),
				regexp.MustCompile(`^display_container_name:` + kubeDeployment + `_` + kubeDeployment + `-[[:alnum:]]+-[[:alnum:]]+$`),
				regexp.MustCompile(`^git.commit.sha:`),
				regexp.MustCompile(`^git.repository_url:https://github.com/DataDog/test-infra-definitions$`),
				regexp.MustCompile(`^image_id:`), // field is inconsistent. it can be a hash or an image + hash
				regexp.MustCompile(`^image_name:ghcr.io/datadog/apps-tracegen$`),
				regexp.MustCompile(`^image_tag:main$`),
				regexp.MustCompile(`^kube_container_name:` + kubeDeployment + `$`),
				regexp.MustCompile(`^kube_deployment:` + kubeDeployment + `$`),
				regexp.MustCompile(`^kube_namespace:` + kubeNamespaceTracegenWorkload + `$`),
				regexp.MustCompile(`^kube_ownerref_kind:replicaset$`),
				regexp.MustCompile(`^kube_ownerref_name:` + kubeDeployment + `-[[:alnum:]]+$`),
				regexp.MustCompile(`^kube_replica_set:` + kubeDeployment + `-[[:alnum:]]+$`),
				regexp.MustCompile(`^kube_qos:burstable$`),
				regexp.MustCompile(`^pod_name:` + kubeDeployment + `-[[:alnum:]]+-[[:alnum:]]+$`),
				regexp.MustCompile(`^pod_phase:running$`),
				regexp.MustCompile(`^short_image:apps-tracegen$`),
			}, false)
			if err == nil {
				break
			}
		}
		require.NoErrorf(c, err, "Failed finding trace with proper tags")
	}, 2*time.Minute, 10*time.Second, "Failed finding trace with proper tags")
}
