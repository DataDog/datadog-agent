// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/jvmmemory/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	metricPoolUsed      = "jvm_memory.pool.used"
	metricPoolCommitted = "jvm_memory.pool.committed"
	metricPoolMax       = "jvm_memory.pool.max"
	metricGCPauses      = "jvm_memory.gc.pauses"
	metricGCFullPauses  = "jvm_memory.gc.full_pauses"
	metricGCPauseTime   = "jvm_memory.gc.pause_time"

	poolTypeHeap    = "heap"
	poolTypeNonHeap = "non_heap"
)

// Check reports the memory pools and GC pauses of the HotSpot JVMs tracked by
// the jvm_memory system-probe module.
type Check struct {
	core.CheckBase
	tagger         tagger.Component
	sysProbeClient *sysprobeclient.CheckClient
	// now returns the current time; it is a field so that tests can control it.
	now func() time.Time
	// previousRunTime is the time of the start of the previous successful run.
	// A pool series is reported only if one of its rows was updated since then.
	previousRunTime time.Time
	// gcBaselines holds, per JVM PID, the GC counters seen during the previous
	// successful run, used to compute the deltas submitted as counts.
	gcBaselines map[uint32]model.GCStats
}

// Factory creates a new check factory
func Factory(tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger)
	})
}

func newCheck(tagger tagger.Component) *Check {
	return &Check{
		CheckBase:   core.NewCheckBase(CheckName),
		tagger:      tagger,
		now:         time.Now,
		gcBaselines: make(map[uint32]model.GCStats),
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	c.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))
	return nil
}

// poolKey identifies an aggregated memory pool series: the pool of all the
// JVMs sharing the same set of tags (i.e. running in the same container).
type poolKey struct {
	tagSet string
	pool   string
}

// poolSeries sums the rows of all the JVMs contributing to a pool series. The
// series is reported only if at least one of its rows is fresh; its stale rows
// then contribute their latest known values, so that the sum does not jump
// depending on which JVMs collected garbage during the interval.
type poolSeries struct {
	tags         []string
	fresh        bool
	used         uint64
	committed    uint64
	max          uint64
	maxUndefined bool
}

// gcSeries sums the GC counters increases of all the JVMs contributing to a
// GC series.
type gcSeries struct {
	tags  []string
	total model.GCStats
}

// Run executes the check
func (c *Check) Run() error {
	// Take the reference time before querying system-probe: a GC ending while
	// the request is in flight is then reported again by the next run instead
	// of being missed.
	now := c.now()

	stats, err := sysprobeclient.GetCheck[[]model.JVMStats](c.sysProbeClient, sysconfig.JVMMemoryModule)
	if err != nil {
		return sysprobeclient.IgnoreStartupError(err)
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	if c.previousRunTime.IsZero() {
		c.previousRunTime = now.Add(-c.Interval())
	}
	freshAfter := c.previousRunTime.UnixNano()

	pools := make(map[poolKey]*poolSeries)
	gcs := make(map[string]*gcSeries)
	containerTags := make(map[string]containerTagsResult)
	alive := make(map[uint32]struct{}, len(stats))

	for _, jvm := range stats {
		alive[jvm.PID] = struct{}{}

		ct, cached := containerTags[jvm.ContainerID]
		if !cached {
			ct.tags, ct.ok = c.getContainerTags(jvm.ContainerID)
			if ct.ok {
				ct.tagSet = tagSetKey(ct.tags)
			}
			containerTags[jvm.ContainerID] = ct
		}
		if !ct.ok {
			// The tagger does not know the container yet: skip the JVM rather
			// than reporting it with the JVMs of the host. Its GC baseline is
			// kept, so the next run reports the pauses of both intervals.
			continue
		}

		for _, pool := range jvm.Pools {
			name := normalizePoolName(pool.Name)
			if name == "" {
				log.Debugf("jvm_memory: ignoring pool with unsupported name %q of pid %d", pool.Name, jvm.PID)
				continue
			}
			key := poolKey{tagSet: ct.tagSet, pool: name}
			series, ok := pools[key]
			if !ok {
				series = &poolSeries{tags: slices.Concat(ct.tags, []string{"pool:" + name, "pool_type:" + poolType(name)})}
				pools[key] = series
			}
			series.fresh = series.fresh || pool.LastUpdate > freshAfter
			series.used += pool.Used
			series.committed += pool.Committed
			if pool.Max == model.UndefinedSize {
				series.maxUndefined = true
			} else {
				series.max += pool.Max
			}
		}

		series, ok := gcs[ct.tagSet]
		if !ok {
			series = &gcSeries{tags: ct.tags}
			gcs[ct.tagSet] = series
		}
		delta := c.gcDelta(jvm.PID, jvm.GC)
		series.total.Pauses += delta.Pauses
		series.total.FullPauses += delta.FullPauses
		series.total.PauseTimeNs += delta.PauseTimeNs
	}

	for pid := range c.gcBaselines {
		if _, ok := alive[pid]; !ok {
			delete(c.gcBaselines, pid)
		}
	}

	for _, series := range pools {
		if !series.fresh {
			continue
		}
		sender.Gauge(metricPoolUsed, float64(series.used), "", series.tags)
		sender.Gauge(metricPoolCommitted, float64(series.committed), "", series.tags)
		if !series.maxUndefined {
			sender.Gauge(metricPoolMax, float64(series.max), "", series.tags)
		}
	}
	for _, series := range gcs {
		sender.Count(metricGCPauses, float64(series.total.Pauses), "", series.tags)
		sender.Count(metricGCFullPauses, float64(series.total.FullPauses), "", series.tags)
		sender.Count(metricGCPauseTime, float64(series.total.PauseTimeNs)/float64(time.Second), "", series.tags)
	}

	c.previousRunTime = now
	sender.Commit()
	return nil
}

// gcDelta returns the GC counters increase of the JVM with the given PID since
// the previous run, and records the new baseline. The first time a PID is
// seen, or when a counter decreased (the PID was reused by another JVM), the
// counters only become the new baseline and the returned delta is zero.
func (c *Check) gcDelta(pid uint32, current model.GCStats) model.GCStats {
	previous, known := c.gcBaselines[pid]
	c.gcBaselines[pid] = current
	if !known ||
		current.Pauses < previous.Pauses ||
		current.FullPauses < previous.FullPauses ||
		current.PauseTimeNs < previous.PauseTimeNs {
		return model.GCStats{}
	}
	return model.GCStats{
		Pauses:      current.Pauses - previous.Pauses,
		FullPauses:  current.FullPauses - previous.FullPauses,
		PauseTimeNs: current.PauseTimeNs - previous.PauseTimeNs,
	}
}

// containerTagsResult caches the result of getContainerTags for a container.
type containerTagsResult struct {
	tags []string
	// tagSet is the tagSetKey of tags, only computed when ok.
	tagSet string
	ok     bool
}

// getContainerTags returns the tags of the given container, or no tag for a
// JVM running directly on the host. ok is false when the tagger does not know
// the container (no tag, or an error).
func (c *Check) getContainerTags(containerID string) (tags []string, ok bool) {
	if containerID == "" {
		return nil, true
	}
	tags, err := c.tagger.Tag(types.NewEntityID(types.ContainerID, containerID), types.HighCardinality)
	if err != nil {
		log.Debugf("jvm_memory: unable to get tags of container %s: %v", containerID, err)
		return nil, false
	}
	return tags, len(tags) > 0
}

// tagSetKey returns a key identifying a set of tags, independently of their order.
func tagSetKey(tags []string) string {
	sorted := slices.Clone(tags)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x00")
}

// normalizePoolName lowercases a HotSpot memory pool name and replaces every
// run of non-alphanumeric characters with a single underscore, trimmed at both
// ends: "CodeHeap 'profiled nmethods'" becomes "codeheap_profiled_nmethods".
func normalizePoolName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	separator := false
	for i := 0; i < len(name); i++ {
		ch := name[i]
		switch {
		case ch >= 'A' && ch <= 'Z':
			ch += 'a' - 'A'
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
		default:
			separator = true
			continue
		}
		if separator && b.Len() > 0 {
			b.WriteByte('_')
		}
		separator = false
		b.WriteByte(ch)
	}
	return b.String()
}

// poolType classifies a normalized memory pool name as a heap or non-heap pool.
func poolType(normalizedName string) string {
	switch {
	case normalizedName == "metaspace",
		normalizedName == "compressed_class_space",
		normalizedName == "code_cache",
		strings.HasPrefix(normalizedName, "codeheap_"):
		return poolTypeNonHeap
	default:
		return poolTypeHeap
	}
}
