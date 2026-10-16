// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package containers

import (
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/jvmgc"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/oomkill"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

const (
	ebpfChecksOOMKillNamespace = "workload-oomkill"
	ebpfChecksJVMNamespace     = "workload-jvm-gc"
)

// ebpfChecksHelmValues enables system-probe with the modules of the eBPF checks under test.
const ebpfChecksHelmValues = `
datadog:
  systemProbe:
    # Runs the full system-probe with the oom_kill module, and makes the core
    # agent schedule the oom_kill check (_oom_kill AD identifier).
    enableOOMKill: true
  envDict:
    # jvm_memory is not a Helm chart option. It is enabled in the environment of
    # every agent container, as both system-probe (which loads the module) and
    # the core agent (which schedules the check through the _jvm_memory AD
    # identifier) need it. A system-probe container specific variable would be
    # ignored, as system-probe gets its configuration streamed by the core agent.
    DD_JVM_MEMORY_ENABLED: "true"
`

// kindEBPFChecksSuite tests the eBPF-based core checks, which need system-probe.
// It is separate from kindSuite so that a system-probe issue cannot make the
// tests of kindSuite fail.
type kindEBPFChecksSuite struct {
	baseSuite[environments.Kubernetes]
}

func TestKindEBPFChecksSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &kindEBPFChecksSuite{}, e2e.WithProvisioner(provkind.Provisioner(
		provkind.WithRunOptions(
			scenkind.WithVMOptions(
				// The agent with system-probe plus the JVM do not fit comfortably in the 4 GiB of the default t3.medium.
				scenec2.WithInstanceType("t3.large"),
			),
			scenkind.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(ebpfChecksHelmValues),
			),
			scenkind.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return oomkill.K8sAppDefinition(e, kubeProvider, ebpfChecksOOMKillNamespace)
			}),
			scenkind.WithWorkloadApp(func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
				return jvmgc.K8sAppDefinition(e, kubeProvider, ebpfChecksJVMNamespace)
			}),
		),
	)))
}

func (suite *kindEBPFChecksSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.clusterName = suite.Env().KubernetesCluster.ClusterName
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}

// Test00UpAndRunning waits for the agent pods, system-probe included, to be ready.
// Inside a testify suite, tests run in alphabetical order: this one runs first so
// that it absorbs the warm-up time of the agent.
func (suite *kindEBPFChecksSuite) Test00UpAndRunning() {
	suite.testAgentUpAndRunning(10 * time.Minute)
}

// TestZZUpAndRunning runs last and checks that no agent container, system-probe
// included, restarted during the tests.
func (suite *kindEBPFChecksSuite) TestZZUpAndRunning() {
	suite.testAgentUpAndRunning(1 * time.Minute)
}

func (suite *kindEBPFChecksSuite) testAgentUpAndRunning(waitFor time.Duration) {
	suite.Run("agent pods are ready and not restarting", func() {
		err := suite.Env().WaitForAgentReady(
			suite.T().Context(),
			environments.WithLinuxNodeAgentReady(),
			environments.WithClusterAgentReady(),
			environments.WithAgentReadinessTimeout(waitFor),
		)
		suite.Require().NoError(err, "Not all agents eventually became ready in time.")
	})
}

// waitForFirstMetric waits up to timeout for a first series of the given metric,
// and returns whether it arrived. The detailed assertions that follow then use the
// default, shorter, timeout of testMetric: if the check does not report anything,
// the test fails once instead of waiting for every assertion.
func (suite *kindEBPFChecksSuite) waitForFirstMetric(name string, filterTags []string, timeout time.Duration) bool {
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: name,
			Tags: filterTags,
		},
		Timeout: timeout,
	})
	return !suite.T().Failed()
}

// TestOOMKill checks that the oom_kill check reports the OOM kills of the
// oomkill workload, as a metric and as an event.
func (suite *kindEBPFChecksSuite) TestOOMKill() {
	filterTags := []string{
		`^kube_namespace:` + ebpfChecksOOMKillNamespace + `$`,
		`^kube_deployment:` + oomkill.Name + `$`,
	}

	// With cgroup v2 and Kubernetes >= 1.28, the whole container is OOM-killed and
	// the kubelet restart backoff spaces the OOM kills by up to 5 minutes.
	if !suite.waitForFirstMetric("oom_kill.oom_process.count", filterTags, 7*time.Minute) {
		return
	}

	// The tags of the OOM-killed container. Its id changes at every restart, so it is not pinned.
	expectedTags := []string{
		`^container_id:`,
		`^container_name:` + oomkill.Name + `$`,
		`^display_container_name:` + oomkill.Name,
		`^git\.commit\.sha:[[:xdigit:]]{40}$`,                                      // org.opencontainers.image.revision docker image label
		`^git\.repository_url:https://github\.com/DataDog/test-infra-definitions$`, // org.opencontainers.image.source docker image label
		`^image_id:.*/apps-stress-ng@sha256:`,
		`^image_name:.*/apps-stress-ng$`,
		`^image_tag:` + regexp.QuoteMeta(apps.Version) + `$`,
		`^kube_container_name:` + oomkill.Name + `$`,
		`^kube_deployment:` + oomkill.Name + `$`,
		`^kube_namespace:` + ebpfChecksOOMKillNamespace + `$`,
		`^kube_ownerref_kind:replicaset$`,
		`^kube_ownerref_name:` + oomkill.Name + `-[[:alnum:]]+$`,
		`^kube_qos:Guaranteed$`,
		`^kube_replica_set:` + oomkill.Name + `-[[:alnum:]]+$`,
		`^pod_name:` + oomkill.Name + `-[[:alnum:]]+-[[:alnum:]]+$`,
		`^short_image:apps-stress-ng$`,
		`^trigger_type:cgroup$`,
		`^trigger_process_name:` + oomkill.ProcessName + `$`,
		`^process_name:` + oomkill.ProcessName + `$`,
	}
	optionalTags := []string{
		`^stackid:`,
		// The pod is crash looping on some configurations, see oomkill.K8sAppDefinition.
		`^pod_phase:`,
	}

	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "oom_kill.oom_process.count",
			Tags: filterTags,
		},
		Expect: testMetricExpectArgs{
			Tags: &expectedTags,
			Value: &testMetricExpectValueArgs{
				Min: 1,
				Max: 100,
			},
		},
		Optional: testMetricExpectArgs{
			Tags: &optionalTags,
		},
	})

	suite.testEvent(&testEventArgs{
		Filter: testEventFilterArgs{
			Source: "oom_kill",
			Tags:   filterTags,
		},
		Expect: testEventExpectArgs{
			Tags:      &expectedTags,
			Title:     `^Process OOM Killed: oom_kill_process called on ` + oomkill.ProcessName + ` \(pid: [[:digit:]]+\)$`,
			Text:      "(?s)Process `" + oomkill.ProcessName + "` \\(pid: [[:digit:]]+.*This OOM kill was invoked by a cgroup, containerID: [[:xdigit:]]+\\.",
			Priority:  event.PriorityNormal,
			AlertType: event.AlertTypeError,
		},
		Optional: testEventExpectArgs{
			Tags: &optionalTags,
		},
	})
}

// TestJVMMemory checks that the jvm_memory check reports the memory pools and
// the GC pauses of the JVM of the jvmgc workload, which runs the G1 collector.
func (suite *kindEBPFChecksSuite) TestJVMMemory() {
	filterTags := func(extraTags ...string) []string {
		return append([]string{
			`^kube_namespace:` + ebpfChecksJVMNamespace + `$`,
			`^kube_deployment:` + jvmgc.Name + `$`,
		}, extraTags...)
	}

	// Before reporting anything, system-probe must find the JVM, attach to its
	// USDT probes and see a GC end, and the check must run.
	if !suite.waitForFirstMetric("jvm_memory.pool.used", filterTags(), 5*time.Minute) {
		return
	}

	containerTags := []string{
		`^container_id:`,
		`^container_name:` + jvmgc.Name + `$`,
		`^display_container_name:` + jvmgc.Name,
		`^image_id:(?:.*/)?` + jvmgc.ImageName + `@sha256:`,
		`^image_name:(?:.*/)?` + jvmgc.ImageName + `$`,
		`^image_tag:` + regexp.QuoteMeta(jvmgc.ImageTag) + `$`,
		`^kube_container_name:` + jvmgc.Name + `$`,
		`^kube_deployment:` + jvmgc.Name + `$`,
		`^kube_namespace:` + ebpfChecksJVMNamespace + `$`,
		`^kube_ownerref_kind:replicaset$`,
		`^kube_ownerref_name:` + jvmgc.Name + `-[[:alnum:]]+$`,
		`^kube_qos:Burstable$`,
		`^kube_replica_set:` + jvmgc.Name + `-[[:alnum:]]+$`,
		`^pod_name:` + jvmgc.Name + `-[[:alnum:]]+-[[:alnum:]]+$`,
		`^pod_phase:running$`,
		`^short_image:` + jvmgc.ImageName + `$`,
	}
	edenTags := append(slices.Clone(containerTags), `^pool:g1_eden_space$`, `^pool_type:heap$`)

	// The full tag set of a memory pool series.
	suite.testMetric(&testMetricArgs{
		Filter: testMetricFilterArgs{
			Name: "jvm_memory.pool.used",
			Tags: filterTags(`^pool:g1_eden_space$`),
		},
		Expect: testMetricExpectArgs{
			Tags: &edenTags,
			// The eden is empty right after a young collection.
			Value: &testMetricExpectValueArgs{
				Min: 0,
				Max: jvmgc.MaxHeapBytes,
			},
		},
	})

	// The values of the other pools: as the container tags are checked above,
	// only the pool tags are checked below.
	for _, tc := range []struct {
		metric   string
		pool     string
		poolType string
		minValue float64
		maxValue float64
	}{
		// The workload retains data that gets promoted to the old generation.
		{"jvm_memory.pool.used", "g1_old_gen", "heap", 1, jvmgc.MaxHeapBytes},
		{"jvm_memory.pool.committed", "g1_old_gen", "heap", 1, jvmgc.MaxHeapBytes},
		// The maximum size of the old generation is the maximum size of the heap.
		{"jvm_memory.pool.max", "g1_old_gen", "heap", jvmgc.MaxHeapBytes / 2, jvmgc.MaxHeapBytes},
		{"jvm_memory.pool.used", "metaspace", "non_heap", 1, 1 << 30},
	} {
		poolTags := []string{`^pool:` + tc.pool + `$`, `^pool_type:` + tc.poolType + `$`}
		suite.testMetric(&testMetricArgs{
			Filter: testMetricFilterArgs{
				Name: tc.metric,
				Tags: filterTags(`^pool:` + tc.pool + `$`),
			},
			Expect: testMetricExpectArgs{
				Tags: &poolTags,
				Value: &testMetricExpectValueArgs{
					Min: tc.minValue,
					Max: tc.maxValue,
				},
				AcceptUnexpectedTags: true,
			},
		})
	}

	// The GC pause counts carry the container tags only. The workload triggers a
	// young collection every few seconds and a full one every 30 seconds, so every
	// check run (15 seconds interval) sees pauses, and every other one a full pause.
	for _, tc := range []struct {
		metric   string
		minValue float64
		maxValue float64
	}{
		{"jvm_memory.gc.pauses", 1, 1000},
		{"jvm_memory.gc.full_pauses", 1, 1000},
		// In seconds.
		{"jvm_memory.gc.pause_time", 1e-6, 60},
	} {
		suite.testMetric(&testMetricArgs{
			Filter: testMetricFilterArgs{
				Name: tc.metric,
				Tags: filterTags(),
			},
			Expect: testMetricExpectArgs{
				Tags: &containerTags,
				Value: &testMetricExpectValueArgs{
					Min: tc.minValue,
					Max: tc.maxValue,
				},
			},
		})
	}
}
