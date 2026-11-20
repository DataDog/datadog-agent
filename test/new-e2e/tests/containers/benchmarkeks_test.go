// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package containers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/cenkalti/backoff/v7"
	"github.com/fatih/color"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/churn"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	scenbenchmarkeks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/benchmarkeks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provbenchmarkeks "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/benchmarkeks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/containers/benchmark"
)

const (
	// benchmarkPhasesEnvVar overrides the number of measurement phases, even and at least 2:
	// the installations swap their images between consecutive phases.
	benchmarkPhasesEnvVar = "E2E_BENCHMARKEKS_PHASES"
	// benchmarkWarmupEnvVar overrides, as a Go duration, how long the Agents run before
	// each measurement phase starts.
	benchmarkWarmupEnvVar = "E2E_BENCHMARKEKS_WARMUP"
	// benchmarkDurationEnvVar overrides, as a Go duration, how long each measurement phase
	// lasts.
	benchmarkDurationEnvVar = "E2E_BENCHMARKEKS_DURATION"
	// benchmarkReportDirEnvVar is the directory the reports are written to, the session
	// output directory of the suite by default.
	benchmarkReportDirEnvVar = "E2E_BENCHMARKEKS_REPORT_DIR"
	// benchmarkBaselineAgentImageEnvVar and benchmarkBaselineClusterAgentImageEnvVar set
	// the images of the baseline. They take precedence over the ddagent:baseline* config
	// keys, so that a CI job can pass the baseline it resolves, along with
	// benchmarkBaselineCommitEnvVar, the commit the baseline images were built from.
	benchmarkBaselineAgentImageEnvVar        = "E2E_BENCHMARKEKS_BASELINE_AGENT_IMAGE"
	benchmarkBaselineClusterAgentImageEnvVar = "E2E_BENCHMARKEKS_BASELINE_CLUSTER_AGENT_IMAGE"
	benchmarkBaselineCommitEnvVar            = "E2E_BENCHMARKEKS_BASELINE_SHA"

	// With the defaults, the test lasts about 110 minutes once the Agents are ready: four
	// phases of warm-up and measurement, and the swaps of the images in between. Why four:
	// see doc/how-to/test/e2e/benchmarkeks/how-it-works.md.
	defaultBenchmarkPhases   = 4
	defaultBenchmarkWarmup   = 10 * time.Minute
	defaultBenchmarkDuration = 15 * time.Minute
	// benchmarkBlock is the duration of the blocks whose means the significance test
	// compares. It is shortened for a measurement too short to hold benchmark.MinBlocks. The
	// run times of the checks are read at the end of each block, so a block must hold at
	// most as many runs of a check as the 32 run times the Agent keeps: 20 for a check every
	// 15 s.
	benchmarkBlock = 5 * time.Minute
	// benchmarkRollup is the interval of the points of the queried timeseries, on which the
	// measurement windows and their blocks are aligned.
	benchmarkRollup = time.Minute

	benchmarkReadinessTimeout = 15 * time.Minute
	benchmarkChecksTimeout    = 10 * time.Minute
	benchmarkIngestionTimeout = 15 * time.Minute
	benchmarkSwapTimeout      = 10 * time.Minute
	benchmarkEventTimeout     = time.Minute

	nodeAgentComponent     = "Node Agent"
	clusterAgentComponent  = "Cluster Agent"
	clusterChecksComponent = "Cluster checks runner"

	// checkSamplesTolerance is the maximum relative difference between the metric samples
	// per run of a check of the comparison and of the baseline.
	checkSamplesTolerance = 0.25

	// runTimeMetric is the metric of the run times of the checks in the report.
	runTimeMetric = "check run time"
	// statusAttempts is the number of attempts to get the runner stats of an Agent.
	statusAttempts = 3
)

// The thresholds are to be calibrated with A/A runs, comparing an Agent to itself.
var (
	cpuThresholds     = benchmark.Thresholds{Relative: 0.05, Absolute: 5e6}      // nanocores: 5 millicores
	memoryThresholds  = benchmark.Thresholds{Relative: 0.05, Absolute: 10 << 20} // bytes: 10 MiB
	runTimeThresholds = benchmark.Thresholds{Relative: 0.10, Absolute: 5}        // milliseconds
	// runTimeRelativeThresholds override the relative threshold of runTimeThresholds for the
	// checks whose run time varies the most from an Agent pod to another, which no number of
	// blocks of a phase averages out: up to 12% in a phase for kubelet in A/A runs.
	runTimeRelativeThresholds = map[string]float64{
		"kubelet": 0.20,
	}
	// untimedChecks are the stable checks whose run times aren't compared: container and
	// containerd share a 2-s cache of the container stats, filled by whichever runs first, an
	// order set anew at each start of the Agent, so that their run times differed by up to 97%
	// in a phase between the two installations in A/B runs whose profiles showed no change. A
	// change of their cost still shows, diluted, in the CPU usage of the node Agents, and their
	// metric samples are still compared.
	untimedChecks = []string{"container", "containerd"}
)

// benchmarkComponent is one of benchmarkComponents.
type benchmarkComponent struct {
	name     string
	workload func(*components.KubernetesAgent) kubeComp.KubernetesObjRefOutput
	// role restricts the measured and checked pods to the nodes of the node pools of this
	// role, if not empty.
	role string
	// container is the measured container of the pods of the workload.
	container string
	// scope are the tags selecting the measured containers in dddev, and their profiles,
	// besides those of the cluster and of the namespace of the variant, or of the image of
	// the version for the profiles.
	scope []string
	// image returns the image of the measured container among images.
	image  func(images scenbenchmarkeks.Images) string
	checks []benchmark.CheckRequirement
}

// benchmarkComponents are the Agent workloads whose resource usage is compared, and whose
// checks the functional tests verify. The run times of their checks with a stable set of
// instances are compared too, but for untimedChecks (see stableChecks).
var benchmarkComponents = []benchmarkComponent{
	{
		name: nodeAgentComponent,
		workload: func(agent *components.KubernetesAgent) kubeComp.KubernetesObjRefOutput {
			return agent.LinuxNodeAgent
		},
		// The node Agents of the other node pools don't monitor the benchmark workload.
		role:      scenbenchmarkeks.RoleNodeAgent,
		container: "agent",
		scope:     []string{"kube_container_name:agent", "kube_app_component:agent", scenbenchmarkeks.RoleTag + ":" + scenbenchmarkeks.RoleNodeAgent},
		image:     agentImage,
		checks: []benchmark.CheckRequirement{
			{Name: "kubelet", MinRuns: 2},
			{Name: "container", MinRuns: 2},
			{Name: "containerd", MinRuns: 2},
			// Scheduled through the AD annotations of the churning sleeper pods.
			{Name: "simple_check", MinRuns: 1, AnyInstance: true},
		},
	},
	{
		name: clusterAgentComponent,
		workload: func(agent *components.KubernetesAgent) kubeComp.KubernetesObjRefOutput {
			return agent.LinuxClusterAgent
		},
		container: "cluster-agent",
		scope:     []string{"kube_container_name:cluster-agent"},
		image:     clusterAgentImage,
	},
	{
		name: clusterChecksComponent,
		workload: func(agent *components.KubernetesAgent) kubeComp.KubernetesObjRefOutput {
			return agent.LinuxClusterChecks
		},
		container: "agent",
		scope:     []string{"kube_container_name:agent", "kube_app_component:clusterchecks-agent"},
		// The cluster checks runner runs the image of the node Agent (see the Helm values of
		// components/datadog/agent/kubernetes_helm.go): the swaps can't set it apart.
		image: agentImage,
		checks: []benchmark.CheckRequirement{
			{Name: "kubernetes_state_core", MinRuns: 2},
		},
	},
}

// agentImage returns the image of the node Agent among images.
func agentImage(images scenbenchmarkeks.Images) string {
	return images.AgentFullImagePath
}

// clusterAgentImage returns the image of the Cluster Agent among images.
func clusterAgentImage(images scenbenchmarkeks.Images) string {
	return images.ClusterAgentFullImagePath
}

// benchmarkMetrics are the compared metrics of every component.
var benchmarkMetrics = []struct {
	name       string
	unit       benchmark.Unit
	thresholds benchmark.Thresholds
}{
	{name: "container.cpu.usage", unit: benchmark.UnitNanocores, thresholds: cpuThresholds},
	{name: "container.memory.working_set", unit: benchmark.UnitBytes, thresholds: memoryThresholds},
	{name: "container.memory.rss", unit: benchmark.UnitBytes, thresholds: memoryThresholds},
}

// stableCheck is a check of an Agent workload whose instances don't come and go with the
// churning pods.
type stableCheck struct {
	component string
	check     string
}

// stableChecks returns the stable checks of benchmarkComponents, whose metric samples and,
// but for untimedChecks, run times are compared. A check is a small part of the CPU usage of
// its container, so a large change of its cost can barely show in the container metrics.
func stableChecks() []stableCheck {
	return lo.FlatMap(benchmarkComponents, func(component benchmarkComponent, _ int) []stableCheck {
		return lo.FilterMap(component.checks, func(check benchmark.CheckRequirement, _ int) (stableCheck, bool) {
			return stableCheck{component: component.name, check: check.Name}, !check.AnyInstance
		})
	})
}

// benchmarkEKSSuite compares the resource usage of a baseline and a comparison Agent
// monitoring the same workload, after verifying both work. The comparison Agent is the
// Agent of the pipeline under test unless set with the ddagent:comparison* config keys; the
// baseline is set with the E2E_BENCHMARKEKS_BASELINE_*_IMAGE environment variables or the
// ddagent:baseline* config keys, e.g.
// -c ddagent:baselineFullImagePath=<image> -c ddagent:baselineClusterAgentFullImagePath=<image>.
//
// Each of the two installations, of variants a and b, runs on node pools of its own, whose
// machines can differ by up to 20% in CPU time for the same work. So the measurement runs in
// an even number of phases, between which the installations swap their images: each version
// runs half of the phases on the node pools of each variant, which cancels this bias out (see
// benchmark.Compare).
type benchmarkEKSSuite struct {
	baseSuite[environments.BenchmarkEKS]

	dddev *dddevClient
	// readyAt is the time all the Agents were ready, from which the warm-up is counted.
	readyAt time.Time
	// readyRestarts are the restart counts of the containers of the measured pods at readyAt.
	readyRestarts map[string]int32
	// failedTest is the first test that failed.
	failedTest string
	// baselineImages and comparisonImages are the images of the two versions, once
	// Test10Performance has read them.
	baselineImages, comparisonImages scenbenchmarkeks.Images
	// profiles is the window over which the dashboard linked when the suite ends compares the
	// profiles of the versions: from the start of the first measurement phase to the end of the
	// last one computed, once Test10Performance has computed one.
	profiles benchmark.Window
}

func TestBenchmarkEKSSuite(t *testing.T) {
	e2e.Run(t, &benchmarkEKSSuite{}, e2e.WithProvisioner(provbenchmarkeks.Provisioner(
		provbenchmarkeks.WithRunOptions(scenbenchmarkeks.WithVariantA(scenbenchmarkeks.Images{
			AgentFullImagePath:        os.Getenv(benchmarkBaselineAgentImageEnvVar),
			ClusterAgentFullImagePath: os.Getenv(benchmarkBaselineClusterAgentImageEnvVar),
		})),
	)))
}

// benchmarkVariant is one of the two Agent installations of the benchmark. In the first phase,
// the installation of variant a runs the baseline images, and that of variant b the
// comparison images; they swap them between consecutive phases (see benchmark.SwapInPhase).
type benchmarkVariant struct {
	name  string
	agent *components.KubernetesAgent
}

func (s *benchmarkEKSSuite) variantA() benchmarkVariant {
	return benchmarkVariant{name: scenbenchmarkeks.VariantA, agent: s.Env().AgentA}
}

func (s *benchmarkEKSSuite) variantB() benchmarkVariant {
	return benchmarkVariant{name: scenbenchmarkeks.VariantB, agent: s.Env().AgentB}
}

func (s *benchmarkEKSSuite) variants() []benchmarkVariant {
	return []benchmarkVariant{s.variantA(), s.variantB()}
}

// installationStats are the runner stats of the components with checks of an installation,
// by component name, then by pod name.
type installationStats map[string]map[string]benchmark.RunnerStats

// installationSnapshots are the snapshots of the runner stats of the components with checks
// of an installation, by component name.
type installationSnapshots map[string][]benchmark.Snapshot

func (s *benchmarkEKSSuite) SetupSuite() {
	s.baseSuite.SetupSuite()
	s.clusterName = s.Env().KubernetesCluster.ClusterName
	// A cleanup rather than TearDownSuite, which testify doesn't call when SetupSuite fails,
	// e.g. when the Agents don't become ready: the dashboard helps most when a step fails.
	t := s.T()
	t.Cleanup(func() { s.logDashboardURL(t) })

	var err error
	s.dddev, err = newDDDevClient()
	s.Require().NoError(err)

	s.Require().NoError(s.waitForAgents(benchmarkReadinessTimeout), "the Agents didn't become ready")
	s.Require().NoError(s.waitForWorkload(benchmarkReadinessTimeout), "the workload didn't become ready")
	s.readyAt = time.Now()
	s.readyRestarts, err = s.restarts(s.T().Context())
	s.Require().NoError(err)
	s.T().Logf("All the Agents are ready at %s", s.readyAt.UTC().Format(time.RFC3339))
}

func (s *benchmarkEKSSuite) AfterTest(suiteName, testName string) {
	if s.T().Failed() && s.failedTest == "" {
		s.failedTest = testName
	}
	s.baseSuite.AfterTest(suiteName, testName)
}

// Test00ChecksRun verifies that the checks whose cost the benchmark measures run fine in
// both variants: a broken check must never pass for a performance improvement.
func (s *benchmarkEKSSuite) Test00ChecksRun() {
	for _, variant := range s.variants() {
		s.Run(variant.name, func() {
			s.checksRun(variant)
		})
	}
}

// checksRun waits for the checks of the installation of variant to meet their requirements
// (see benchmarkComponents), and reports whether they did.
func (s *benchmarkEKSSuite) checksRun(variant benchmarkVariant) bool {
	ctx := s.T().Context()
	return s.EventuallyWithTf(func(c *assert.CollectT) {
		stats, err := s.runnerStats(ctx, variant)
		require.NoError(c, err)

		var errs []error
		for _, component := range benchmarkComponents {
			for _, pod := range slices.Sorted(maps.Keys(stats[component.name])) {
				for _, check := range component.checks {
					if err := check.Verify(stats[component.name][pod]); err != nil {
						errs = append(errs, fmt.Errorf("%s %s: %w", component.name, pod, err))
					}
				}
			}
		}
		require.NoError(c, errors.Join(errs...))
	}, benchmarkChecksTimeout, 30*time.Second, "the checks of installation %s don't run as expected", variant.name)
}

// Test01ChecksMatchBaseline verifies that the checks of the comparison emit about as many
// metric samples per run as those of the baseline: a check the comparison broke, or made
// emit far fewer metrics, would otherwise pass for a performance improvement.
func (s *benchmarkEKSSuite) Test01ChecksMatchBaseline() {
	s.checksMatchBaseline()
}

// checksMatchBaseline waits for the checks of installation b to emit about as many metric
// samples per run as those of installation a, and reports whether they did.
func (s *benchmarkEKSSuite) checksMatchBaseline() bool {
	ctx := s.T().Context()
	// EventuallyWithT runs the condition in a goroutine, which it doesn't wait for on timeout:
	// the summary of the last condition is shared under a lock.
	var mu sync.Mutex
	var summary []string
	matched := s.EventuallyWithTf(func(c *assert.CollectT) {
		_, stats, err := s.snapshot(ctx)
		require.NoError(c, err)

		var lines []string
		var errs []error
		// stableChecks leaves out the checks whose instances come and go with the churning
		// pods: their number at the time of a snapshot varies between installations.
		for _, stable := range stableChecks() {
			baselineSamples := totalMetricSamples(stats[scenbenchmarkeks.VariantA][stable.component], stable.check)
			comparisonSamples := totalMetricSamples(stats[scenbenchmarkeks.VariantB][stable.component], stable.check)
			lines = append(lines, fmt.Sprintf("%s check %s: baseline %d, comparison %d", stable.component, stable.check, baselineSamples, comparisonSamples))
			if err := benchmark.CompareMetricSamples(baselineSamples, comparisonSamples, checkSamplesTolerance); err != nil {
				errs = append(errs, fmt.Errorf("%s check %s: %w", stable.component, stable.check, err))
			}
		}
		mu.Lock()
		summary = lines
		mu.Unlock()
		require.NoError(c, errors.Join(errs...))
	}, benchmarkChecksTimeout, 30*time.Second, "the checks of the comparison don't emit as many metric samples as those of the baseline")
	mu.Lock()
	defer mu.Unlock()
	if len(summary) > 0 {
		s.T().Logf("Metric samples per run:\n%s", strings.Join(summary, "\n"))
	}
	return matched
}

// Test02DataInDDDev verifies that the data of the checks of both variants reaches dddev,
// where the performance test measures the Agents.
func (s *benchmarkEKSSuite) Test02DataInDDDev() {
	for _, variant := range s.variants() {
		s.Run(variant.name, func() {
			s.dataInDDDev(variant)
		})
	}
}

// dataInDDDev waits for the data of the checks of the installation of variant to reach
// dddev, and reports whether it did.
func (s *benchmarkEKSSuite) dataInDDDev(variant benchmarkVariant) bool {
	ctx := s.T().Context()
	queries := []string{
		// Kubelet check of the node Agents, whose host tags include the variant
		fmt.Sprintf("avg:kubernetes.cpu.usage.total{%s,%s:%s}", s.clusterTag(), scenbenchmarkeks.VariantTag, variant.name),
		// KSM check of the cluster checks runner
		fmt.Sprintf("sum:kubernetes_state.pod.count{%s,%s:%s}", s.clusterTag(), scenbenchmarkeks.AgentVariantTag, variant.name),
	}
	// The queries the performance test compares, so that one selecting no data, e.g. for lack
	// of a tag, fails now rather than after the measurement.
	for _, component := range benchmarkComponents {
		for _, metric := range benchmarkMetrics {
			queries = append(queries, s.benchmarkQuery(metric.name, variant.name, component.scope))
		}
	}
	return s.EventuallyWithTf(func(c *assert.CollectT) {
		now := time.Now()
		for _, query := range queries {
			points, err := s.dddev.query(ctx, query, now.Add(-5*time.Minute), now)
			require.NoError(c, err)
			require.NotEmptyf(c, points, "no data for %s", query)
		}
	}, benchmarkIngestionTimeout, 30*time.Second, "the data of installation %s doesn't reach dddev", variant.name)
}

// measurementIngested waits for dddev to hold the last bucket of each of windows of the
// timeseries of every container whose average the performance test compares, ingested with a
// delay, and reports whether it does.
func (s *benchmarkEKSSuite) measurementIngested(windows []benchmark.Window) bool {
	ctx := s.T().Context()
	return s.EventuallyWithTf(func(c *assert.CollectT) {
		// The last window first: until it is ingested, a failing query ends the attempt before
		// querying the earlier ones, ingested long ago.
		for _, window := range slices.Backward(windows) {
			lastBucket := window.To.Add(-benchmarkRollup)
			for _, variant := range s.variants() {
				for _, component := range benchmarkComponents {
					for _, metric := range benchmarkMetrics {
						query := s.benchmarkQueryBy(metric.name, variant.name, component.scope, "container_id")
						series, err := s.dddev.querySeries(ctx, query, window.From, window.To)
						require.NoError(c, err)
						require.NotEmptyf(c, series, "no data for %s", query)
						for _, points := range series {
							require.NotEmptyf(c, points, "no data for %s", query)
							require.Falsef(c, points[len(points)-1].Timestamp.Before(lastBucket), "no data for a container of %s from %s", query, lastBucket.UTC().Format(time.RFC3339))
						}
					}
				}
			}
		}
	}, benchmarkIngestionTimeout, 30*time.Second, "the last points of the measurement phases don't reach dddev")
}

// Test10Performance compares the CPU and memory usage of the components of the two
// variants, and the run times of their checks, over an even number of phases, between which
// the installations swap their images, writes the report, and fails on a regression. A test
// running after it would get the original images back: BaseSuite.BeforeTest reprovisions the
// stack with the original provisioner, without waiting for the Agents to be ready.
func (s *benchmarkEKSSuite) Test10Performance() {
	if s.failedTest != "" {
		s.T().Skipf("%s failed: the performance of an Agent failing the functional tests is meaningless", s.failedTest)
	}
	// The functional tests may not have run yet, e.g. when this test is selected alone: what
	// they verify is verified again, which is quick once it holds.
	// Their helpers log why they fail.
	for _, variant := range s.variants() {
		s.Require().True(s.checksRun(variant))
		s.Require().True(s.dataInDDDev(variant))
	}
	s.Require().True(s.checksMatchBaseline())

	phases, err := phasesFromEnv(benchmarkPhasesEnvVar, defaultBenchmarkPhases)
	s.Require().NoError(err)
	warmup, err := durationFromEnv(benchmarkWarmupEnvVar, defaultBenchmarkWarmup)
	s.Require().NoError(err)
	duration, err := durationFromEnv(benchmarkDurationEnvVar, defaultBenchmarkDuration)
	s.Require().NoError(err)
	// The window starts on a bucket: it must also end on one, so that its last bucket doesn't
	// hold data from after it.
	s.Require().Zerof(duration%benchmarkRollup, "%s=%s must be a multiple of %s", benchmarkDurationEnvVar, duration, benchmarkRollup)
	block := min(benchmarkBlock, (duration / benchmark.MinBlocks).Truncate(benchmarkRollup))
	s.Require().GreaterOrEqualf(block, benchmarkRollup, "%s=%s is too short: each phase must hold %d blocks of at least %s", benchmarkDurationEnvVar, duration, benchmark.MinBlocks, benchmarkRollup)

	ctx := s.T().Context()
	// The images are those of the specs of the pods, so that the swaps also work when an
	// installation runs the default images, e.g. those of the pipeline in CI.
	baselineImages, err := s.installationImages(ctx, s.variantA())
	s.Require().NoError(err)
	comparisonImages, err := s.installationImages(ctx, s.variantB())
	s.Require().NoError(err)
	s.baselineImages, s.comparisonImages = baselineImages, comparisonImages
	// The commit of the pipeline images, which the comparison runs by default, e.g. in CI.
	comparisonCommit, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.CommitSHA, "")
	s.Require().NoError(err)

	// The events of the phases are posted in the background (see postPhaseEvent): the test
	// waits for them before it ends, not to log after it.
	var eventPosts sync.WaitGroup
	defer eventPosts.Wait()
	baselineEventImages := []string{baselineImages.AgentFullImagePath, baselineImages.ClusterAgentFullImagePath}
	comparisonEventImages := []string{comparisonImages.AgentFullImagePath, comparisonImages.ClusterAgentFullImagePath}

	report := benchmark.Report{ClusterName: s.clusterName, Phases: make([]benchmark.Window, phases)}
	snapshots := make([]map[string]installationSnapshots, phases)
	snapshotErrs := make([]error, phases)
	readyAt, readyRestarts := s.readyAt, s.readyRestarts
	for phase := range phases {
		if phase > 0 {
			imagesA, imagesB := benchmark.SwapInPhase(phase, baselineImages, comparisonImages)
			readyAt, readyRestarts = s.swapImages(ctx, imagesA, imagesB)
		}

		window := measurementWindow(readyAt, warmup, duration, block)
		report.Phases[phase] = window
		baselineVariant, comparisonVariant := benchmark.SwapInPhase(phase, s.variantA(), s.variantB())
		s.profiles = benchmark.Window{From: report.Phases[0].From, To: window.To}
		s.T().Logf("Phase %d of %d: measuring the baseline images on installation %s and the comparison images on installation %s from %s to %s",
			phase+1, phases, baselineVariant.name, comparisonVariant.name, window.From.UTC().Format(time.RFC3339), window.To.UTC().Format(time.RFC3339))
		event := benchmark.PhaseEvent{
			Boundary:   benchmark.PhaseStart,
			Phase:      phase,
			Phases:     phases,
			Window:     window,
			Baseline:   baselineEventImages,
			Comparison: comparisonEventImages,
		}
		// At the start of the window rather than in advance, not to post an event dated in the
		// future.
		s.sleepUntil(window.From)
		s.postPhaseEvent(ctx, &eventPosts, event)
		snapshots[phase], snapshotErrs[phase] = s.runSnapshots(ctx, window)
		event.Boundary = benchmark.PhaseEnd
		s.postPhaseEvent(ctx, &eventPosts, event)
		// A restart during the measurement skews it, and hints at a crash or an OOM kill.
		s.Require().NoErrorf(s.compareRestarts(ctx, readyRestarts), "an Agent restarted during phase %d or the warm-up before it", phase+1)
		s.Require().NoErrorf(s.waitForWorkload(time.Minute), "the workload stopped during phase %d", phase+1)
		// After the measurement, not to delay its first snapshot when the window starts right away.
		if phase == 0 {
			report.Baseline, err = s.reportVersion(ctx, "baseline", baselineVariant, os.Getenv(benchmarkBaselineCommitEnvVar))
			s.Require().NoError(err)
			report.Comparison, err = s.reportVersion(ctx, "comparison", comparisonVariant, comparisonCommit)
			s.Require().NoError(err)
		}
	}
	s.Require().True(s.measurementIngested(report.Phases))

	first, last := report.Phases[0], report.Phases[len(report.Phases)-1]
	profiles := benchmark.Window{From: first.From, To: last.To}
	report.DashboardURL = s.dashboardURL(s.readyAt, last.To, profiles)
	report.ProfileComparisons = s.profileComparisons(baselineImages, comparisonImages, profiles)
	timedChecks := slices.DeleteFunc(stableChecks(), func(stable stableCheck) bool {
		return slices.Contains(untimedChecks, stable.check)
	})
	// Bonferroni correction: the probability of a false verdict among all the compared metrics
	// stays below benchmark.SignificanceLevel.
	significance := benchmark.SignificanceLevel / float64(len(benchmarkComponents)*len(benchmarkMetrics)+len(timedChecks))
	for _, component := range benchmarkComponents {
		for _, metric := range benchmarkMetrics {
			thresholds := metric.thresholds
			thresholds.Significance = significance
			// The explorer link splits the metric by image, i.e. by version: the curve of each
			// version is continuous across the swaps.
			result := benchmark.Result{
				Component:   component.name,
				Metric:      metric.name,
				Unit:        metric.unit,
				Thresholds:  thresholds,
				QueryA:      s.benchmarkQuery(metric.name, scenbenchmarkeks.VariantA, component.scope),
				QueryB:      s.benchmarkQuery(metric.name, scenbenchmarkeks.VariantB, component.scope),
				ExplorerURL: benchmark.MetricsExplorerURL(dddevAppURL, metric.name, append([]string{s.clusterTag()}, component.scope...), []string{"image_tag"}, first.From, last.To),
			}
			report.Results = append(report.Results, compareResult(result, report.Phases, func(_ int, window benchmark.Window) ([]benchmark.Point, []benchmark.Point, error) {
				installationA, err := s.dddev.query(ctx, result.QueryA, window.From, window.To)
				if err != nil {
					return nil, nil, err
				}
				installationB, err := s.dddev.query(ctx, result.QueryB, window.From, window.To)
				return installationA, installationB, err
			}))
		}
	}
	for _, timed := range timedChecks {
		thresholds := runTimeThresholds
		if relative, found := runTimeRelativeThresholds[timed.check]; found {
			thresholds.Relative = relative
		}
		thresholds.Significance = significance
		result := benchmark.Result{
			Component:  timed.component,
			Metric:     runTimeMetric,
			Check:      timed.check,
			Unit:       benchmark.UnitMilliseconds,
			Thresholds: thresholds,
		}
		report.Results = append(report.Results, compareResult(result, report.Phases, func(i int, _ benchmark.Window) ([]benchmark.Point, []benchmark.Point, error) {
			if snapshotErrs[i] != nil {
				return nil, nil, snapshotErrs[i]
			}
			installationA, err := benchmark.RunTimePoints(timed.check, snapshots[i][scenbenchmarkeks.VariantA][timed.component])
			if err != nil {
				return nil, nil, fmt.Errorf("installation %s: %w", scenbenchmarkeks.VariantA, err)
			}
			installationB, err := benchmark.RunTimePoints(timed.check, snapshots[i][scenbenchmarkeks.VariantB][timed.component])
			if err != nil {
				return nil, nil, fmt.Errorf("installation %s: %w", scenbenchmarkeks.VariantB, err)
			}
			return installationA, installationB, nil
		}))
	}

	reportDir := os.Getenv(benchmarkReportDirEnvVar)
	if reportDir == "" {
		reportDir = s.SessionOutputDir()
	}
	s.Require().NoError(report.Write(reportDir))
	s.T().Logf("Report written to %s:\n%s", reportDir, report.Markdown())

	for _, result := range report.Results {
		s.Emptyf(result.Error, "%s %s couldn't be compared", result.Component, result.Name())
		s.NotEqualf(benchmark.VerdictRegression, result.Verdict, "%s %s regressed by %+.1f%% (p=%.3g)", result.Component, result.Name(), 100*result.RelativeChange, result.PValue)
	}
}

// measurementWindow returns the window of a measurement phase of Agents ready at readyAt. It
// starts after the warm-up, on a bucket of the timeseries, so that their buckets fit in it.
func measurementWindow(readyAt time.Time, warmup, duration, block time.Duration) benchmark.Window {
	from := readyAt.Add(warmup)
	if now := time.Now(); from.Before(now) {
		from = now
	}
	if truncated := from.Truncate(benchmarkRollup); truncated.Before(from) {
		from = truncated.Add(benchmarkRollup)
	}
	return benchmark.Window{From: from, To: from.Add(duration), Block: block}
}

// swapImages has installation a run imagesA and installation b imagesB, through a Helm
// upgrade of both, then waits for all the Agents to be ready and verifies their checks again.
// It returns the time the Agents were ready at, and the restart counts of the containers of
// the measured pods then.
func (s *benchmarkEKSSuite) swapImages(ctx context.Context, imagesA, imagesB scenbenchmarkeks.Images) (time.Time, map[string]int32) {
	s.T().Logf("Swapping the images: %+v on installation a, %+v on installation b", imagesA, imagesB)
	s.UpdateEnv(provbenchmarkeks.Provisioner(provbenchmarkeks.WithRunOptions(
		scenbenchmarkeks.WithVariantA(imagesA),
		scenbenchmarkeks.WithVariantB(imagesB),
	)))
	s.Require().True(s.EventuallyWithTf(func(c *assert.CollectT) {
		images, err := s.installationImages(ctx, s.variantA())
		require.NoError(c, err)
		require.Equal(c, imagesA, images, "images of installation a")
		images, err = s.installationImages(ctx, s.variantB())
		require.NoError(c, err)
		require.Equal(c, imagesB, images, "images of installation b")
	}, benchmarkSwapTimeout, 10*time.Second, "the installations didn't swap their images"))
	s.Require().NoError(s.waitForAgents(benchmarkReadinessTimeout), "the Agents didn't become ready after swapping their images")
	readyAt := time.Now()
	restarts, err := s.restarts(ctx)
	s.Require().NoError(err)
	s.T().Logf("All the Agents are ready after the swap at %s", readyAt.UTC().Format(time.RFC3339))
	// An image working on a single node pool, or a single installation, must not slip through.
	for _, variant := range s.variants() {
		s.Require().Truef(s.checksRun(variant), "the checks of installation %s don't run as expected after the swap to %+v on installation a and %+v on installation b", variant.name, imagesA, imagesB)
	}
	return readyAt, restarts
}

// compareResult returns result compared over the phases of windows, from the points of the
// installations of variants a and b points returns for each phase.
func compareResult(result benchmark.Result, windows []benchmark.Window, points func(phase int, window benchmark.Window) (installationA, installationB []benchmark.Point, err error)) benchmark.Result {
	phases := make([]benchmark.Phase, len(windows))
	for i, window := range windows {
		installationA, installationB, err := points(i, window)
		if err != nil {
			result.Fail(fmt.Errorf("phase %d: %w", i+1, err))
			return result
		}
		baseline, comparison := benchmark.SwapInPhase(i, installationA, installationB)
		phases[i] = benchmark.Phase{Window: window, Baseline: baseline, Comparison: comparison}
	}
	result.Compare(phases)
	return result
}

// reportVersion describes, as version name, the images the installation of variant runs.
func (s *benchmarkEKSSuite) reportVersion(ctx context.Context, name string, variant benchmarkVariant, commit string) (benchmark.Version, error) {
	images, err := s.images(ctx, variant)
	if err != nil {
		return benchmark.Version{}, err
	}
	return benchmark.Version{Name: name, Commit: commit, Images: images}, nil
}

// profileComparisons returns the links comparing the CPU profiles of each component of the
// baseline, running baselineImages, with those of the comparison, running comparisonImages,
// over window.
func (s *benchmarkEKSSuite) profileComparisons(baselineImages, comparisonImages scenbenchmarkeks.Images, window benchmark.Window) []benchmark.ProfileComparison {
	return lo.Map(benchmarkComponents, func(component benchmarkComponent, _ int) benchmark.ProfileComparison {
		return benchmark.ProfileComparison{
			Component: component.name,
			URL:       benchmark.ProfileComparisonURL(dddevAppURL, s.profileQuery(component, baselineImages), s.profileQuery(component, comparisonImages), window.From, window.To),
		}
	})
}

// profileQuery returns the query of the profiles of the measured containers of component
// running the images of a version. They are selected by image rather than by installation,
// since each version runs on both installations over the phases.
func (s *benchmarkEKSSuite) profileQuery(component benchmarkComponent, images scenbenchmarkeks.Images) string {
	return strings.Join(slices.Concat([]string{s.clusterTag()}, component.scope, []string{"image_tag:" + imageTag(component.image(images))}), " ")
}

// imageTag returns the tag the Agents tag the metrics and the profiles of the containers of
// image with: that of the image path once split into repository and tag, as the scenario does
// for the Helm values, but for a digest, which the Agents leave out.
func imageTag(image string) string {
	image, _, _ = strings.Cut(image, "@")
	_, tag := utils.ParseImageReference(image)
	return tag
}

// postPhaseEvent posts event to dddev, where the dashboard of the benchmark overlays the
// boundaries of the phases on its graphs. It posts it in a goroutine of posts, not to delay
// the snapshots of the measurement while dddev retries. A failure, or a post outlasting
// benchmarkEventTimeout, is only logged: the events only help to read the graphs.
func (s *benchmarkEKSSuite) postPhaseEvent(ctx context.Context, posts *sync.WaitGroup, event benchmark.PhaseEvent) {
	t := s.T()
	request := datadogV1.EventCreateRequest{
		Title:        event.Title(),
		Text:         event.Text(),
		DateHappened: datadog.PtrInt64(event.Time().Unix()),
		AlertType:    datadogV1.EVENTALERTTYPE_INFO.Ptr(),
		Tags:         append([]string{"app:agent-new-e2e-tests-containers", "test:" + t.Name(), s.clusterTag()}, event.Tags()...),
	}
	posts.Go(func() {
		ctx, cancel := context.WithTimeout(ctx, benchmarkEventTimeout)
		defer cancel()
		if err := s.dddev.postEvent(ctx, request); err != nil {
			t.Logf("No event for the %s of phase %d: %v", event.Boundary, event.Phase+1, err)
		}
	})
}

// runSnapshots takes snapshots of the runner stats of the components with checks of both
// installations at the start of window and at the end of each of its blocks, so that the run
// times of the checks make a point per block (see benchmark.RunTimePoints), and returns them
// by installation. It returns at the end of window: an error ends the snapshots, not the
// wait. Getting the status of the Agents costs those of both installations a little CPU
// alike.
func (s *benchmarkEKSSuite) runSnapshots(ctx context.Context, window benchmark.Window) (map[string]installationSnapshots, error) {
	snapshots := make(map[string]installationSnapshots, 2)
	for at := window.From; ; at = at.Add(window.Block) {
		if at.After(window.To) {
			at = window.To
		}
		s.sleepUntil(at)
		taken, stats, err := s.snapshot(ctx)
		if err != nil {
			err = fmt.Errorf("snapshot at %s: %w", at.UTC().Format(time.RFC3339), err)
			s.T().Logf("No more snapshots of the runner stats in this phase: %v", err)
			s.sleepUntil(window.To)
			return nil, err
		}
		for variant, componentStats := range stats {
			if snapshots[variant] == nil {
				snapshots[variant] = make(installationSnapshots, len(componentStats))
			}
			for component, podStats := range componentStats {
				snapshots[variant][component] = append(snapshots[variant][component], benchmark.Snapshot{Time: taken, Stats: podStats})
			}
		}
		if !at.Before(window.To) {
			return snapshots, nil
		}
	}
}

// snapshot returns the runner stats of the components with checks of both installations,
// taken concurrently, by installation, and the time it launched them at.
func (s *benchmarkEKSSuite) snapshot(ctx context.Context) (time.Time, map[string]installationStats, error) {
	taken := time.Now()
	variants := s.variants()
	stats := make([]installationStats, len(variants))
	errs := make([]error, len(variants))
	var wg sync.WaitGroup
	for i, variant := range variants {
		wg.Go(func() {
			if stats[i], errs[i] = s.runnerStats(ctx, variant); errs[i] != nil {
				errs[i] = fmt.Errorf("installation %s: %w", variant.name, errs[i])
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return taken, nil, err
	}

	byVariant := make(map[string]installationStats, len(variants))
	for i, variant := range variants {
		byVariant[variant.name] = stats[i]
	}
	return taken, byVariant, nil
}

// installationImages returns the images the installation of variant runs, as set in the
// specs of the pods of its components.
func (s *benchmarkEKSSuite) installationImages(ctx context.Context, variant benchmarkVariant) (scenbenchmarkeks.Images, error) {
	images := make(map[string]string, len(benchmarkComponents))
	for _, component := range benchmarkComponents {
		pods, err := s.pods(ctx, component.workload(variant.agent), "")
		if err != nil {
			return scenbenchmarkeks.Images{}, err
		}
		var componentImages []string
		for _, pod := range pods {
			for _, container := range pod.Spec.Containers {
				if container.Name == component.container && !slices.Contains(componentImages, container.Image) {
					componentImages = append(componentImages, container.Image)
				}
			}
		}
		if len(componentImages) != 1 {
			return scenbenchmarkeks.Images{}, fmt.Errorf("the %s of installation %s runs %d images: %v", component.name, variant.name, len(componentImages), componentImages)
		}
		images[component.name] = componentImages[0]
	}

	installation := scenbenchmarkeks.Images{
		AgentFullImagePath:        images[nodeAgentComponent],
		ClusterAgentFullImagePath: images[clusterAgentComponent],
	}
	// The swaps set the images of an installation as a whole: each component must run the one
	// of them it is meant to, e.g. the cluster checks runner that of the node Agent.
	for _, component := range benchmarkComponents {
		if expected := component.image(installation); images[component.name] != expected {
			return scenbenchmarkeks.Images{}, fmt.Errorf("the %s of installation %s runs %s instead of %s", component.name, variant.name, images[component.name], expected)
		}
	}
	return installation, nil
}

// waitForAgents waits for the node Agents, Cluster Agent and cluster checks runner of both
// installations to be ready. They may have restarted, e.g. node Agents failing to get their
// hostname on nodes still starting: what matters is that they don't restart during a
// measurement (see compareRestarts).
func (s *benchmarkEKSSuite) waitForAgents(timeout time.Duration) error {
	for _, variant := range s.variants() {
		if err := environments.WaitForKubernetesAgentReady(s.T().Context(), s.Env().KubernetesCluster, variant.agent,
			environments.WithLinuxNodeAgentReady(),
			// The node Agent runs on all the node pools of its variant, and only there.
			environments.WithLinuxNodeAgentNodeSelector(labels.SelectorFromSet(labels.Set{scenbenchmarkeks.VariantLabel: variant.name}).String()),
			environments.WithClusterAgentReady(),
			environments.WithClusterChecksReady(),
			environments.WithAgentRestartsAllowed(),
			environments.WithAgentReadinessTimeout(timeout),
		); err != nil {
			return fmt.Errorf("installation %s: %w", variant.name, err)
		}
	}
	return nil
}

// waitForWorkload waits for the churn orchestrator generating the workload to be ready and
// never restarted: a churn that crashed leaves a frozen workload, which the Agents barely
// work on.
func (s *benchmarkEKSSuite) waitForWorkload(timeout time.Duration) error {
	return environments.WaitForKubernetesAgentReady(s.T().Context(), s.Env().KubernetesCluster, nil,
		environments.WithPodsReady("churn orchestrator", churn.Namespace, churn.LabelSelector, 1),
		environments.WithAgentReadinessTimeout(timeout),
	)
}

// restarts returns the restart counts of the containers of the measured pods of the
// components of both installations, by "<pod>/<container>".
func (s *benchmarkEKSSuite) restarts(ctx context.Context) (map[string]int32, error) {
	restarts := make(map[string]int32)
	for _, variant := range s.variants() {
		for _, component := range benchmarkComponents {
			pods, err := s.pods(ctx, component.workload(variant.agent), component.role)
			if err != nil {
				return nil, err
			}
			for _, pod := range pods {
				for _, status := range pod.Status.ContainerStatuses {
					restarts[pod.Name+"/"+status.Name] = status.RestartCount
				}
			}
		}
	}
	return restarts, nil
}

// compareRestarts returns an error if a container of the measured pods restarted since the
// restart counts before were recorded, or if a pod was replaced.
func (s *benchmarkEKSSuite) compareRestarts(ctx context.Context, before map[string]int32) error {
	after, err := s.restarts(ctx)
	if err != nil {
		return err
	}
	return benchmark.CompareRestarts(before, after)
}

// pods returns the pods of an Agent workload, on the nodes of the node pools of role if
// not empty.
func (s *benchmarkEKSSuite) pods(ctx context.Context, workload kubeComp.KubernetesObjRefOutput, role string) ([]corev1.Pod, error) {
	client := s.Env().KubernetesCluster.Client()
	pods, err := client.CoreV1().Pods(workload.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(workload.LabelSelectors).String(),
	})
	if err != nil {
		return nil, err
	}
	if role == "" {
		return pods.Items, nil
	}

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{scenbenchmarkeks.RoleLabel: role}).String(),
	})
	if err != nil {
		return nil, err
	}
	nodeNames := lo.SliceToMap(nodes.Items, func(node corev1.Node) (string, bool) { return node.Name, true })
	return slices.DeleteFunc(pods.Items, func(pod corev1.Pod) bool { return !nodeNames[pod.Spec.NodeName] }), nil
}

// runnerStats returns the runner stats of the Agents of the components of variant that have
// checks, queried concurrently.
func (s *benchmarkEKSSuite) runnerStats(ctx context.Context, variant benchmarkVariant) (installationStats, error) {
	type agentPod struct {
		component string
		container string
		pod       corev1.Pod
	}
	var agentPods []agentPod
	for _, component := range benchmarkComponents {
		if len(component.checks) == 0 {
			continue
		}
		pods, err := s.pods(ctx, component.workload(variant.agent), component.role)
		if err != nil {
			return nil, err
		}
		if len(pods) == 0 {
			return nil, fmt.Errorf("no %s pod", component.name)
		}
		for _, pod := range pods {
			agentPods = append(agentPods, agentPod{component: component.name, container: component.container, pod: pod})
		}
	}

	podStats := make([]benchmark.RunnerStats, len(agentPods))
	errs := make([]error, len(agentPods))
	var wg sync.WaitGroup
	for i, agentPod := range agentPods {
		wg.Go(func() {
			if podStats[i], errs[i] = s.podRunnerStats(ctx, agentPod.pod, agentPod.container); errs[i] != nil {
				errs[i] = fmt.Errorf("%s %s: %w", agentPod.component, agentPod.pod.Name, errs[i])
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	stats := make(installationStats)
	for i, agentPod := range agentPods {
		if stats[agentPod.component] == nil {
			stats[agentPod.component] = make(map[string]benchmark.RunnerStats)
		}
		stats[agentPod.component][agentPod.pod.Name] = podStats[i]
	}
	return stats, nil
}

// podRunnerStats returns the runner stats of the Agent of container in pod, from the
// collector section of its status, retrying on failure. The command runs without the
// DD_LOG_LEVEL of the container, which would override its default log level, off: it then
// logs nothing on its standard output, the JSON document alone.
func (s *benchmarkEKSSuite) podRunnerStats(ctx context.Context, pod corev1.Pod, container string) (benchmark.RunnerStats, error) {
	return backoff.Retry(ctx, func() (benchmark.RunnerStats, error) {
		stdout, stderr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(pod.Namespace, pod.Name, container, []string{"env", "-u", "DD_LOG_LEVEL", "agent", "status", "collector", "--json"})
		if err != nil {
			return nil, fmt.Errorf("agent status failed: %w, stderr: %s", err, stderr)
		}
		return benchmark.ParseRunnerStats(stdout)
	}, backoff.WithMaxTries(statusAttempts), backoff.WithBackOff(backoff.NewConstantBackOff(5*time.Second)))
}

// totalMetricSamples returns the metric samples the instances of check emitted in their
// last run that emitted any, summed over the pods.
func totalMetricSamples(stats map[string]benchmark.RunnerStats, check string) int64 {
	return lo.SumBy(lo.Values(stats), func(podStats benchmark.RunnerStats) int64 { return podStats.MetricSamples(check) })
}

// images returns the images the measured containers of the components of variant run.
func (s *benchmarkEKSSuite) images(ctx context.Context, variant benchmarkVariant) ([]benchmark.Image, error) {
	var images []benchmark.Image
	for _, component := range benchmarkComponents {
		pods, err := s.pods(ctx, component.workload(variant.agent), "")
		if err != nil {
			return nil, err
		}
		for _, pod := range pods {
			for _, status := range pod.Status.ContainerStatuses {
				if status.Name != component.container {
					continue
				}
				if image := (benchmark.Image{Component: component.name, Image: status.Image, ImageID: status.ImageID}); !slices.Contains(images, image) {
					images = append(images, image)
				}
			}
		}
	}
	return images, nil
}

// logDashboardURL logs to t the URL of the dashboard of the benchmark over the run of the
// suite.
func (s *benchmarkEKSSuite) logDashboardURL(t *testing.T) {
	color.NoColor = false
	c := color.New(color.Bold).SprintfFunc()
	t.Log(c("The data of the benchmark can be viewed on this dashboard:"))
	c = color.New(color.Bold, color.FgBlue).SprintfFunc()
	// TearDownSuite sets the end time. When SetupSuite fails, the cleanup of BaseSuite calls it
	// after this one, registered later.
	from, to := s.StartTime(), cmp.Or(s.EndTime(), time.Now())
	// The profiles of the whole run if no phase was computed
	profiles := cmp.Or(s.profiles, benchmark.Window{From: from, To: to})
	t.Log(c("%s", s.dashboardURL(from, to, profiles)))
}

// dashboardURL returns the URL of the dashboard of the benchmark over [from, to], showing the
// cluster of the suite and linking to the comparison of the profiles of the versions over the
// window profiles, selected by the tags of their images once known.
func (s *benchmarkEKSSuite) dashboardURL(from, to time.Time, profiles benchmark.Window) string {
	templateVariables := map[string]string{
		"kube_cluster_name": s.kubeClusterName(),
		"profiles_from":     strconv.FormatInt(profiles.From.UnixMilli(), 10),
		"profiles_to":       strconv.FormatInt(profiles.To.UnixMilli(), 10),
	}
	for version, images := range map[string]scenbenchmarkeks.Images{"baseline": s.baselineImages, "comparison": s.comparisonImages} {
		if images.AgentFullImagePath != "" {
			templateVariables[version+"_agent_image_tag"] = imageTag(images.AgentFullImagePath)
		}
		if images.ClusterAgentFullImagePath != "" {
			templateVariables[version+"_cluster_agent_image_tag"] = imageTag(images.ClusterAgentFullImagePath)
		}
	}
	return benchmark.DashboardURL(dddevAppURL, benchmarkDashboardID, templateVariables, from, to)
}

// kubeClusterName is the value of the kube_cluster_name tag of the cluster of the suite.
func (s *benchmarkEKSSuite) kubeClusterName() string {
	return strings.ToLower(s.clusterName)
}

func (s *benchmarkEKSSuite) clusterTag() string {
	return "kube_cluster_name:" + s.kubeClusterName()
}

// variantScope returns the tags of the containers of the Agent installation of variant.
func (s *benchmarkEKSSuite) variantScope(variant string) []string {
	return []string{s.clusterTag(), "kube_namespace:" + scenbenchmarkeks.Namespace(variant)}
}

// benchmarkQuery returns the query of the average of metric over the containers of a
// component of variant, selected by scope, in buckets of benchmarkRollup.
func (s *benchmarkEKSSuite) benchmarkQuery(metric, variant string, scope []string) string {
	return s.benchmarkQueryBy(metric, variant, scope, "")
}

// benchmarkQueryBy is benchmarkQuery, with a timeseries per value of the tag groupBy if not
// empty.
func (s *benchmarkEKSSuite) benchmarkQueryBy(metric, variant string, scope []string, groupBy string) string {
	query := fmt.Sprintf("avg:%s{%s}", metric, strings.Join(append(s.variantScope(variant), scope...), ","))
	if groupBy != "" {
		query += " by {" + groupBy + "}"
	}
	return fmt.Sprintf("%s.rollup(avg, %d)", query, int(benchmarkRollup.Seconds()))
}

// sleepUntil waits until deadline.
func (s *benchmarkEKSSuite) sleepUntil(deadline time.Time) {
	wait := time.Until(deadline)
	if wait <= 0 {
		return
	}
	s.T().Logf("Waiting %s, until %s", wait.Round(time.Second), deadline.UTC().Format(time.RFC3339))
	time.Sleep(wait)
}

// phasesFromEnv returns the number of measurement phases the environment variable name is
// set to, or def.
func phasesFromEnv(name string, def int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return def, nil
	}
	phases, err := benchmark.ParsePhaseCount(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return phases, nil
}

// durationFromEnv returns the duration the environment variable name is set to, or def.
func durationFromEnv(name string, def time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return def, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if duration < 0 {
		return 0, fmt.Errorf("invalid %s: %s is negative", name, value)
	}
	return duration, nil
}
