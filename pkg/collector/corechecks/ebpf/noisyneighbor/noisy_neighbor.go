// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	cpuutil "github.com/shirou/gopsutil/v4/cpu"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	defaultPSIThreshold     = 5.0  // PSI avg10 "some" percentage
	defaultThrottleRatio    = 0.10 // 10% of wall clock spent throttled
	defaultStealThreshold   = 5.0  // steal time percentage
	maxWatchlistSize        = 100
	maxTopNPreemptors       = 5
	maxNonContainerCgroups  = 100
	checkIntervalNs         = float64(10 * time.Second)
)

// NoisyNeighborConfig holds check configuration
type NoisyNeighborConfig struct {
	PSIThreshold   float64 `yaml:"psi_threshold"`
	ThrottleRatio  float64 `yaml:"throttle_ratio"`
	StealThreshold float64 `yaml:"steal_threshold"`
}

// NoisyNeighborCheck implements the 3-layer noisy neighbor detection check
type NoisyNeighborCheck struct {
	core.CheckBase
	config         *NoisyNeighborConfig
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
	cgroupReader   *cgroups.Reader     // container cgroups (for tagging)
	allCgroupReader *cgroups.Reader    // all cgroups (for Layer 1 PSI reading)

	// Layer 1 state
	lastThrottledNs map[uint64]uint64 // cgroup inode -> last throttled_time in ns
	lastStealTime   float64
	lastTotalCPU    float64
	firstRun        bool
}

func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) check.Check {
	return &NoisyNeighborCheck{
		CheckBase:       core.NewCheckBaseWithInterval(CheckName, 10*time.Second),
		config:          &NoisyNeighborConfig{},
		tagger:          tagger,
		lastThrottledNs: make(map[uint64]uint64),
		firstRun:        true,
	}
}

func (c *NoisyNeighborConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}
	if c.PSIThreshold == 0 {
		c.PSIThreshold = defaultPSIThreshold
	}
	if c.ThrottleRatio == 0 {
		c.ThrottleRatio = defaultThrottleRatio
	}
	if c.StealThreshold == 0 {
		c.StealThreshold = defaultStealThreshold
	}
	return nil
}

func (n *NoisyNeighborCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := n.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	if err := n.config.Parse(config); err != nil {
		return fmt.Errorf("noisy_neighbor check config: %s", err)
	}
	n.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))

	// Container reader for tag resolution
	reader, err := cgroups.NewReader(cgroups.WithReaderFilter(cgroups.ContainerFilter))
	if err != nil {
		return fmt.Errorf("noisy_neighbor: container cgroup reader init failed: %s", err)
	}
	n.cgroupReader = reader

	// All-cgroup reader for Layer 1 PSI/stat reading
	allReader, err := cgroups.NewReader()
	if err != nil {
		return fmt.Errorf("noisy_neighbor: all cgroup reader init failed: %s", err)
	}
	n.allCgroupReader = allReader

	return nil
}

func (n *NoisyNeighborCheck) Run() error {
	s, err := n.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %s", err)
	}

	// Refresh both cgroup readers
	if err := n.allCgroupReader.RefreshCgroups(0); err != nil {
		return fmt.Errorf("unable to refresh all cgroups: %s", err)
	}
	if err := n.cgroupReader.RefreshCgroups(0); err != nil {
		return fmt.Errorf("unable to refresh container cgroups: %s", err)
	}

	// Layer 1: Read canary signals from filesystem
	flaggedCgroupIDs, stealPct := n.runLayer1(s)

	// Emit system-wide steal time
	if !n.firstRun {
		s.Gauge("noisy_neighbor.system.steal_time_pct", stealPct, "", nil)
	}

	// Update watchlist in system-probe
	watchlistReq := model.WatchlistRequest{CgroupIDs: flaggedCgroupIDs}
	if _, err := sysprobeclient.Post[struct{}](n.sysProbeClient, "/watchlist", watchlistReq, sysconfig.NoisyNeighborModule); err != nil {
		log.Warnf("noisy_neighbor: failed to update watchlist: %v", err)
	}

	// Layer 2+3: Fetch eBPF stats from system-probe
	resp, err := sysprobeclient.GetCheck[model.CheckResponse](n.sysProbeClient, sysconfig.NoisyNeighborModule)
	if err != nil {
		log.Warnf("noisy_neighbor: failed to get check data: %v", err)
	} else {
		n.emitLayer2Metrics(s, resp.CgroupStats)
		n.emitLayer3Metrics(s, resp.PreemptorStats)
	}

	n.firstRun = false
	s.Commit()
	return nil
}

// runLayer1 reads PSI and throttle data from cgroup filesystem, emits Layer 1 metrics,
// and returns the list of cgroup IDs that should be watched by Layer 2.
func (n *NoisyNeighborCheck) runLayer1(s sender.Sender) (flaggedIDs []uint64, stealPct float64) {
	stealPct = n.readStealTime()

	nonContainerCount := 0
	for _, cg := range n.allCgroupReader.ListCgroups() {
		inode := cg.Inode()
		tags, isContainer := n.resolveTagsForCgroup(inode)

		if !isContainer {
			nonContainerCount++
			if nonContainerCount > maxNonContainerCgroups {
				continue
			}
		}

		var cpuStats cgroups.CPUStats
		if err := cg.GetCPUStats(&cpuStats); err != nil {
			continue
		}

		// PSI
		var psiAvg10 float64
		if cpuStats.PSISome.Avg10 != nil {
			psiAvg10 = *cpuStats.PSISome.Avg10
		}

		// Throttle delta
		var throttledDeltaNs uint64
		if cpuStats.ThrottledTime != nil {
			prev, hasPrev := n.lastThrottledNs[inode]
			n.lastThrottledNs[inode] = *cpuStats.ThrottledTime
			if hasPrev && *cpuStats.ThrottledTime >= prev {
				throttledDeltaNs = *cpuStats.ThrottledTime - prev
			}
		}

		// Emit Layer 1 metrics (always, for all cgroups)
		if !n.firstRun {
			s.Gauge("noisy_neighbor.cpu_pressure", psiAvg10, "", tags)
			s.Count("noisy_neighbor.cpu_throttled_ns", float64(throttledDeltaNs), "", tags)
		}

		// Triage: should this cgroup be flagged for Layer 2?
		if n.firstRun {
			continue
		}
		throttleRatio := float64(throttledDeltaNs) / checkIntervalNs
		highPSI := psiAvg10 >= n.config.PSIThreshold
		highThrottle := throttleRatio >= n.config.ThrottleRatio
		highSteal := stealPct >= n.config.StealThreshold

		// Flag for Layer 2 when:
		// - PSI high + throttle low + steal low → probable noisy neighbor
		// - PSI high + throttle high + steal high → multiple factors, neighbor may contribute
		// Don't flag when pressure is fully explained by one cause:
		// - PSI high + throttle high + steal low → self-inflicted (quota too low)
		// - PSI high + throttle low + steal high → hypervisor-level neighbor
		if highPSI && len(flaggedIDs) < maxWatchlistSize {
			selfInflicted := highThrottle && !highSteal
			hypervisorOnly := highSteal && !highThrottle
			if !selfInflicted && !hypervisorOnly {
				flaggedIDs = append(flaggedIDs, inode)
			}
		}
	}

	// Clean up stale throttle entries for cgroups that no longer exist
	activeCgroups := make(map[uint64]struct{})
	for _, cg := range n.allCgroupReader.ListCgroups() {
		activeCgroups[cg.Inode()] = struct{}{}
	}
	for inode := range n.lastThrottledNs {
		if _, exists := activeCgroups[inode]; !exists {
			delete(n.lastThrottledNs, inode)
		}
	}

	return flaggedIDs, stealPct
}

// readStealTime reads /proc/stat and computes steal time percentage since last call
func (n *NoisyNeighborCheck) readStealTime() float64 {
	cpuTimes, err := cpuutil.Times(false)
	if err != nil || len(cpuTimes) == 0 {
		return 0
	}
	t := cpuTimes[0]
	total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice

	var stealPct float64
	if !n.firstRun {
		totalDelta := total - n.lastTotalCPU
		stealDelta := t.Steal - n.lastStealTime
		if totalDelta > 0 {
			stealPct = (stealDelta / totalDelta) * 100
		}
	}

	n.lastStealTime = t.Steal
	n.lastTotalCPU = total
	return stealPct
}

// resolveTagsForCgroup returns tags and whether the cgroup is a container
func (n *NoisyNeighborCheck) resolveTagsForCgroup(cgroupInode uint64) ([]string, bool) {
	// Try container tag resolution first
	if cg := n.cgroupReader.GetCgroupByInode(cgroupInode); cg != nil {
		containerID := cg.Identifier()
		if containerID != "" {
			entityID := types.NewEntityID(types.ContainerID, containerID)
			if !entityID.Empty() {
				taggerTags, err := n.tagger.Tag(entityID, types.HighCardinality)
				if err == nil && len(taggerTags) > 0 {
					return taggerTags, true
				}
			}
		}
	}

	// Fallback: use cgroup path basename as tag
	if cg := n.allCgroupReader.GetCgroupByInode(cgroupInode); cg != nil {
		identifier := cg.Identifier()
		if identifier != "" {
			name := filepath.Base(identifier)
			return []string{"cgroup_name:" + name}, false
		}
	}

	return nil, false
}

// emitLayer2Metrics emits detailed scheduling metrics for watched cgroups
// and the synthesized noisy_neighbor.impacted signal.
func (n *NoisyNeighborCheck) emitLayer2Metrics(s sender.Sender, stats []model.NoisyNeighborStats) {
	for _, stat := range stats {
		tags, _ := n.resolveTagsForCgroup(stat.CgroupID)

		// Synthesized impact signal: 1.0 if foreign preemptions are elevated, 0.0 otherwise.
		// This is the top-level "is this container being harmed by another container?" answer.
		var impacted float64
		if stat.ForeignPreemptionCount > 0 {
			impacted = 1.0
		}
		s.Gauge("noisy_neighbor.impacted", impacted, "", tags)

		if stat.TaskCount > 0 {
			psl := float64(stat.SumLatenciesNs) / float64(stat.TaskCount)
			s.Gauge("noisy_neighbor.scheduling_latency.per_process", psl, "", tags)

			foreignPsp := float64(stat.ForeignPreemptionCount) / float64(stat.TaskCount)
			s.Gauge("noisy_neighbor.preemptions.foreign.per_process", foreignPsp, "", tags)

			selfPsp := float64(stat.SelfPreemptionCount) / float64(stat.TaskCount)
			s.Gauge("noisy_neighbor.preemptions.self.per_process", selfPsp, "", tags)
		}

		totalPreemptions := stat.ForeignPreemptionCount + stat.SelfPreemptionCount
		if totalPreemptions > 0 {
			latPerPreempt := float64(stat.SumLatenciesNs) / float64(totalPreemptions)
			s.Gauge("noisy_neighbor.latency_per_preemption", latPerPreempt, "", tags)
		}

		// Latency histogram buckets
		s.Count("noisy_neighbor.scheduling_latency.bucket.lt_100us", float64(stat.LatencyBucketLt100us), "", tags)
		s.Count("noisy_neighbor.scheduling_latency.bucket.100us_1ms", float64(stat.LatencyBucket100us1ms), "", tags)
		s.Count("noisy_neighbor.scheduling_latency.bucket.1ms_10ms", float64(stat.LatencyBucket1ms10ms), "", tags)
		s.Count("noisy_neighbor.scheduling_latency.bucket.gt_10ms", float64(stat.LatencyBucketGt10ms), "", tags)

		// Raw counters
		s.Count("noisy_neighbor.events.total", float64(stat.EventCount), "", tags)
		s.Gauge("noisy_neighbor.cgroup_task_count", float64(stat.TaskCount), "", tags)
		s.Count("noisy_neighbor.cpu_migrations", float64(stat.CpuMigrations), "", tags)
	}
}

// emitLayer3Metrics emits top-N preemptor identification metrics
func (n *NoisyNeighborCheck) emitLayer3Metrics(s sender.Sender, preemptorStats []model.PreemptorStats) {
	if len(preemptorStats) == 0 {
		return
	}

	// Group by victim, sort by count descending, take top N
	byVictim := make(map[uint64][]model.PreemptorStats)
	for _, ps := range preemptorStats {
		byVictim[ps.VictimCgroupID] = append(byVictim[ps.VictimCgroupID], ps)
	}

	for victimCgroupID, entries := range byVictim {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Count > entries[j].Count
		})

		victimTags, _ := n.resolveTagsForCgroup(victimCgroupID)

		limit := maxTopNPreemptors
		if len(entries) < limit {
			limit = len(entries)
		}
		for _, ps := range entries[:limit] {
			preemptorTags, _ := n.resolveTagsForCgroup(ps.PreemptorCgroupID)

			// Combine victim tags with a preemptor identifier tag
			metricTags := make([]string, len(victimTags))
			copy(metricTags, victimTags)
			if len(preemptorTags) > 0 {
				metricTags = append(metricTags, "preemptor:"+preemptorTags[0])
			} else {
				metricTags = append(metricTags, fmt.Sprintf("preemptor_cgroup_id:%d", ps.PreemptorCgroupID))
			}

			s.Gauge("noisy_neighbor.top_preemptor.count", float64(ps.Count), "", metricTags)
		}
	}
}
