// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package tpu

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/amd/sysfsroot"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	tpudevice "github.com/DataDog/datadog-agent/pkg/gpu/tpu"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	prom "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	// collectorName is the collector name used in telemetry for TPU devices.
	collectorName nvidia.CollectorName = "tpu"
	// devicePluginPort is the port of the Prometheus endpoint of the GKE TPU device plugin.
	devicePluginPort = "2112"
	// scrapeTimeout bounds one scrape of the device plugin.
	scrapeTimeout = 5 * time.Second
	// vendor is the gpu_vendor tag value of TPUs.
	vendor = "google"
)

var logLimitCheck = log.NewLogLimit(20, 10*time.Minute)

// Check collects the metrics of Cloud TPUs from the GKE TPU device plugin or, on TPU
// VMs, from the TPU host services, and emits them with the GPU check metric names.
type Check struct {
	core.CheckBase
	tagger             tagger.Component
	wmeta              workloadmeta.Component
	telemetryComponent telemetry.Component
	emitterTelemetry   gpu.EmitterTelemetry
	workloadTagCache   *gpu.WorkloadTagCache
	rateCalculator     *nvidia.RateCalculator
	strictIntervals    *nvidia.StrictIntervalProcessor
	sysRoot            string              // sysRoot is the sysfs root used to detect TPUs
	generation         string              // generation is the TPU generation of the host, e.g. tpu-v6e
	endpoint           string              // endpoint is the device plugin metrics URL, on GKE
	host               *hostSource         // host reads the TPU VM host services, when there is no device plugin
	excludedChips      map[string]struct{} // excludedChips are the lowercased chip IDs in gpu.excluded_devices
	client             *http.Client        // client scrapes the device plugin
}

// Factory creates a new TPU check factory
func Factory(tagger tagger.Component, tm telemetry.Component, wmeta workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger, tm, wmeta)
	})
}

func newCheck(tagger tagger.Component, tm telemetry.Component, wmeta workloadmeta.Component) check.Check {
	tagNames := []string{"collector", "gpu_device"}
	return &Check{
		CheckBase:          core.NewCheckBase(CheckName),
		tagger:             tagger,
		wmeta:              wmeta,
		telemetryComponent: tm,
		emitterTelemetry: gpu.EmitterTelemetry{
			CollectionRuns:   tm.NewCounter(CheckName, "collection_runs", tagNames, "Number of TPU telemetry collections"),
			CollectionErrors: tm.NewCounter(CheckName, "collection_errors", tagNames, "Number of errors collecting TPU telemetry"),
			CollectionTime:   tm.NewHistogram(CheckName, "collection_time_ms", tagNames, "Time taken to collect TPU telemetry, in milliseconds", []float64{10, 100, 500, 1000, 5000}),
			MetricsSent:      tm.NewCounter(CheckName, "metrics_sent", []string{"collector"}, "Number of TPU metrics sent"),
			DuplicateMetrics: tm.NewCounter(CheckName, "duplicate_metrics", []string{"device"}, "Number of duplicate TPU metrics removed by priority de-duplication"),
		},
		rateCalculator:  nvidia.NewRateCalculator(),
		strictIntervals: nvidia.NewStrictIntervalProcessor(0),
		client:          &http.Client{Timeout: scrapeTimeout},
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	cfg := pkgconfigsetup.Datadog()
	if !cfg.GetBool("gpu.enabled") || !cfg.GetBool("gpu.tpu.enabled") {
		return fmt.Errorf("%w: TPU check requires gpu.enabled and gpu.tpu.enabled", check.ErrSkipCheckInstance)
	}
	if c.sysRoot == "" {
		c.sysRoot = sysfsroot.DeviceSysFSRoot()
	}
	if c.generation = tpudevice.Generation(c.sysRoot); c.generation == "" {
		return fmt.Errorf("%w: no TPU found on this host", check.ErrSkipCheckInstance)
	}
	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	c.excludedChips = make(map[string]struct{})
	for _, id := range cfg.GetStringSlice("gpu.excluded_devices") {
		c.excludedChips[strings.ToLower(id)] = struct{}{}
	}
	c.strictIntervals = nvidia.NewStrictIntervalProcessor(gpuconfig.New().StaticMetricsReportingInterval)

	// No container provider: the check only associates container entities, never PIDs.
	workloadTagCache, err := gpu.NewWorkloadTagCacheWithSubsystem(CheckName, c.tagger, c.wmeta, nil, c.telemetryComponent, cfg.GetInt("gpu.workload_tag_cache_size"))
	if err != nil {
		return fmt.Errorf("error creating workload tag cache: %w", err)
	}
	c.workloadTagCache = workloadTagCache

	switch host := cfg.GetString("kubernetes_kubelet_host"); {
	case c.endpoint != "" || c.host != nil: // set by tests
	case host != "":
		c.endpoint = "http://" + net.JoinHostPort(host, devicePluginPort) + "/metrics"
		log.Infof("Collecting TPU metrics from the GKE TPU device plugin at %s", c.endpoint)
	case env.IsFeaturePresent(env.Kubernetes):
		// The TPU VM services are not reachable from a pod, and would lose pod attribution.
		log.Warn("Not collecting TPU metrics: set kubernetes_kubelet_host to the node IP to read the GKE TPU device plugin")
		return fmt.Errorf("%w: kubernetes_kubelet_host is not set", check.ErrSkipCheckInstance)
	default:
		hs, err := newHostSource(instanceIDURL, vbarAddress, runtimeAddress)
		if err != nil {
			return fmt.Errorf("TPU VM host services: %w", err)
		}
		c.host = hs
		log.Infof("Collecting TPU metrics from the TPU VM services at %s and %s", vbarAddress, runtimeAddress)
	}
	return nil
}

// Interval returns the scheduling interval of the check, which follows
// gpu.collection_interval_override like the GPU checks.
func (c *Check) Interval() time.Duration {
	if iv := pkgconfigsetup.Datadog().GetInt("gpu.collection_interval_override"); iv > 0 {
		return time.Duration(iv) * time.Second
	}
	return c.CheckBase.Interval()
}

// Run executes the check
func (c *Check) Run() error {
	snd, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}
	defer snd.Commit()

	start := time.Now()
	var chips []*chip
	if c.host != nil {
		chips, err = c.host.read()
	} else {
		chips, err = c.scrape()
	}
	if err != nil {
		c.emitterTelemetry.CollectionErrors.Inc(string(collectorName), "")
		return fmt.Errorf("read TPU metrics: %w", err)
	}
	chips = slices.DeleteFunc(chips, func(ch *chip) bool {
		_, excluded := c.excludedChips[strings.ToLower(ch.id)]
		return excluded
	})
	c.workloadTagCache.MarkStale()
	results, gpuToContainers := c.collect(chips, time.Since(start))
	emitter := gpu.SampleEmitter{
		WorkloadTagCache: c.workloadTagCache,
		RateCalculator:   c.rateCalculator,
		StrictIntervals:  c.strictIntervals,
		Telemetry:        c.emitterTelemetry,
	}
	tags := func(deviceUUID string) []string { return deviceTags(deviceUUID, c.generation) }
	if err := emitter.Emit(snd, results, gpuToContainers, nil, tags, time.Now()); err != nil && logLimitCheck.ShouldLog() {
		log.Warnf("error while sending TPU metrics: %s", err)
	}
	return nil
}

// scrape reads the device plugin endpoint and groups its series by chip.
func (c *Check) scrape() ([]*chip, error) {
	resp, err := c.client.Get(c.endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseDevicePluginMetrics(body)
}

// Cancel closes the connections to the TPU VM host services.
func (c *Check) Cancel() {
	if c.host != nil {
		c.host.close()
	}
	c.CheckBase.Cancel()
}

// workloadKey identifies the container a device plugin series belongs to.
type workloadKey struct{ namespace, pod, container string }

// chip holds the device plugin series of one TPU chip.
type chip struct {
	id        string
	node      map[string]float64                 // node holds the *_node series, by name without the suffix
	workloads map[workloadKey]map[string]float64 // workloads holds the per-container series
}

// workload returns the series of a workload of the chip, creating them if needed.
func (ch *chip) workload(key workloadKey) map[string]float64 {
	if ch.workloads[key] == nil {
		ch.workloads[key] = map[string]float64{}
	}
	return ch.workloads[key]
}

// chipSeries are the device plugin series read by the check, true for the libtpu
// runtime ones. Each also has a per-node variant with the "_node" suffix and no
// workload labels.
var chipSeries = map[string]bool{
	"tensorcore_utilization":       false,
	"memory_bandwidth_utilization": false,
	"duty_cycle":                   true,
	"memory_used":                  true,
	"memory_total":                 true,
}

// parseDevicePluginMetrics groups the TPU series of a device plugin scrape by chip.
func parseDevicePluginMetrics(data []byte) ([]*chip, error) {
	families, err := prom.ParseMetrics(data)
	if err != nil {
		return nil, err
	}
	host, runtime := map[string]*chip{}, map[string]*chip{}
	for _, family := range families {
		name, isNode := strings.CutSuffix(family.Name, "_node")
		isRuntime, ok := chipSeries[name]
		if !ok {
			continue
		}
		byID := host
		if isRuntime {
			byID = runtime
		}
		for _, s := range family.Samples {
			id := s.Metric["accelerator_id"]
			if id == "" {
				continue
			}
			ch, ok := byID[id]
			if !ok {
				ch = &chip{id: id, node: map[string]float64{}, workloads: map[workloadKey]map[string]float64{}}
				byID[id] = ch
			}
			if isNode {
				ch.node[name] = s.Value
				continue
			}
			key := workloadKey{namespace: s.Metric["namespace"], pod: s.Metric["pod"], container: s.Metric["container"]}
			if key.pod == "" {
				continue
			}
			ch.workload(key)[name] = s.Value
		}
	}
	mergeRuntimeChips(host, runtime)
	return slices.Collect(maps.Values(host)), nil
}

// mergeRuntimeChips adds the runtime series to the host chips. Host series number the
// chips of the node from 0, while the runtime series of multi-host slices number them
// across the slice: runtime IDs the node does not have are mapped by chip index order
// when the chip counts match, and dropped otherwise, as Ray does.
func mergeRuntimeChips(host, runtime map[string]*chip) {
	if len(host) == 0 {
		maps.Copy(host, runtime)
		return
	}
	mapping := map[string]string{}
	for id := range runtime {
		if _, ok := host[id]; ok {
			mapping[id] = id
		}
	}
	if len(mapping) < len(runtime) {
		mapping = map[string]string{}
		if len(runtime) == len(host) {
			hostIDs, runtimeIDs := sortedByIndex(host), sortedByIndex(runtime)
			for i, id := range runtimeIDs {
				mapping[id] = hostIDs[i]
			}
		}
	}
	for from, to := range mapping {
		maps.Copy(host[to].node, runtime[from].node)
		for key, series := range runtime[from].workloads {
			maps.Copy(host[to].workload(key), series)
		}
	}
}

// sortedByIndex returns the chip IDs ("<instance id>-<chip index>") sorted by chip index.
func sortedByIndex(chips map[string]*chip) []string {
	index := func(id string) int {
		i, _ := strconv.Atoi(id[strings.LastIndex(id, "-")+1:])
		return i
	}
	ids := slices.Collect(maps.Keys(chips))
	slices.SortFunc(ids, func(a, b string) int { return cmp.Or(index(a)-index(b), strings.Compare(a, b)) })
	return ids
}

// deviceTags returns the device tags of a chip, with the keys of GPU device tags.
func deviceTags(id, generation string) []string {
	id = strings.ToLower(id)
	return []string{
		"gpu_vendor:" + vendor,
		"gpu_uuid:" + id,
		"gpu_parent_uuid:" + id,
		"gpu_device:" + generation,
		"gpu_type:" + generation,
		"gpu_slicing_mode:none",
		"gpu_sharing_mode:none",
		"gpu_mig_profile:none",
		"gpu_nvlink_capable:false",
	}
}

// collect turns the series of every chip into samples, and returns the containers
// each chip is allocated to.
func (c *Check) collect(chips []*chip, duration time.Duration) ([]gpu.CollectorSamples, map[string][]*workloadmeta.Container) {
	results := make([]gpu.CollectorSamples, 0, len(chips))
	gpuToContainers := make(map[string][]*workloadmeta.Container, len(chips))
	for _, ch := range chips {
		// Device samples have no associated workloads: the emitter tags them with
		// the containers allocated the chip, so memory.limit carries the tags of
		// all of them, as for NVIDIA and AMD devices.
		// The idle inference needs runtime series and utilization from one coherent device
		// plugin scrape: on TPU VMs, the VBAR utilization is cached between polls.
		samples := nodeSamples(ch, c.host == nil)
		for key, series := range ch.workloads {
			container := c.resolveContainer(key)
			if container == nil {
				continue
			}
			gpuToContainers[ch.id] = append(gpuToContainers[ch.id], container)
			samples = append(samples, workloadSamples(series, container.EntityID)...)
		}
		results = append(results, gpu.CollectorSamples{
			Name:          collectorName,
			DeviceUUID:    ch.id,
			TelemetryTags: []string{string(collectorName), c.generation},
			Samples:       samples,
			Duration:      duration,
		})
	}
	return results, gpuToContainers
}

// resolveContainer returns the workloadmeta container of a device plugin series, or
// nil when it is not in the store (yet).
func (c *Check) resolveContainer(key workloadKey) *workloadmeta.Container {
	pod, err := c.wmeta.GetKubernetesPodByName(key.pod, key.namespace)
	if err != nil {
		return nil
	}
	for _, oc := range pod.Containers {
		if oc.Name != key.container {
			continue
		}
		container, err := c.wmeta.GetContainer(oc.ID)
		if err != nil {
			return nil
		}
		return container
	}
	return nil
}

// nodeSamples returns the device-level samples of a chip.
func nodeSamples(ch *chip, inferIdle bool) []nvidia.Sample {
	gauge := func(name string, value float64) nvidia.Sample {
		return nvidia.NewMetric(name, value, metrics.GaugeType, nvidia.Low, nil, nil)
	}
	samples := []nvidia.Sample{gauge("device.total", 1)}
	utilization, hasUtilization := ch.node["tensorcore_utilization"]
	if hasUtilization {
		samples = append(samples, gauge("sm_active", utilization))
	}
	if v, ok := ch.node["duty_cycle"]; ok {
		samples = append(samples, gauge("gr_engine_active", v))
	} else if inferIdle && hasUtilization && utilization == 0 && len(ch.workloads) == 0 {
		// The runtime series disappear when no workload uses the chip: it is idle.
		samples = append(samples, gauge("gr_engine_active", 0))
	}
	if v, ok := ch.node["memory_bandwidth_utilization"]; ok {
		samples = append(samples, gauge("dram_active", v))
	}
	total, hasTotal := ch.node["memory_total"]
	used, hasUsed := ch.node["memory_used"]
	if hasTotal {
		samples = append(samples, gauge("memory.limit", total))
	}
	if hasTotal && hasUsed && used <= total {
		samples = append(samples, gauge("memory.free", total-used))
		if total > 0 {
			samples = append(samples, gauge("memory.utilization", used/total))
		}
	}
	return samples
}

// workloadSamples returns the samples of one container using a chip.
func workloadSamples(series map[string]float64, workload workloadmeta.EntityID) []nvidia.Sample {
	workloads := []workloadmeta.EntityID{workload}
	var samples []nvidia.Sample
	add := func(name, source string) {
		if v, ok := series[source]; ok {
			samples = append(samples, nvidia.NewMetric(name, v, metrics.GaugeType, nvidia.Medium, nil, workloads))
		}
	}
	add("process.memory.usage", "memory_used")
	add("process.sm_active", "tensorcore_utilization")
	add("process.dram_active", "memory_bandwidth_utilization")
	return samples
}
