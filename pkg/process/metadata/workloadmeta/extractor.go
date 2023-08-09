// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/server/process"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Extractor does these two things:
//   - Detecting the language of new processes and sending them to WorkloadMeta
//   - Detecting the processes that terminate and sending their PID to WorkloadMeta
type Extractor struct {
	// Cache is a map from process hash to the workloadmeta entity
	// The cache key takes the form of `pid:<pid>|createTime:<createTime>`. See hashProcess
	cache        map[string]*process.Entity
	cacheVersion int32
	cacheMutex   sync.RWMutex

	diffChan chan *process.CacheDiff

	pidToCid map[int]string
}

const subsystem = "WorkloadMetaExtractor"

var (
	cacheSizeGuage = telemetry.NewGauge(subsystem, "cache_size", nil, "The cache size for the workloadMetaExtractor")
	oldDiffDropped = telemetry.NewSimpleCounter(subsystem, "diff_dropped", "The number of times a diff is removed from the queue due to the diffChan being full.")
	diffChanFull   = telemetry.NewSimpleCounter(subsystem, "diff_chan_full", "The number of times the extractor was unable to write to the diffChan due to it being full. This should never happen.")
)

// NewExtractor constructs the Extractor.
func NewExtractor(config config.ConfigReader) *Extractor {
	log.Info("Instantiating a new Extractor")

	return &Extractor{
		cache:        make(map[string]*process.Entity),
		cacheVersion: 0,
		// Keep only the latest diff in memory in case there's no consumer for it
		diffChan: make(chan *process.CacheDiff, 1),
	}
}

// SetLastPidToCid is a utility function that should be called from either the process collector, or the process check.
// pidToCid will be used by the extractor to add enrich process entities with their associated container id.
// This method was added to avoid the cost of reaching out to workloadMeta on a hot path where `GetContainers` will do an O(n) copy of its entire store.
// Note that this method is not thread safe.
func (w *Extractor) SetLastPidToCid(pidToCid map[int]string) {
	w.pidToCid = pidToCid
}

// Extract detects the process language, creates a process entity, and sends that entity to WorkloadMeta
func (w *Extractor) Extract(procs map[int32]*procutil.Process) {
	defer w.reportTelemetry()

	newEntities := make([]*process.Entity, 0, len(procs))
	newProcs := make([]*procutil.Process, 0, len(procs))
	newCache := make(map[string]*process.Entity, len(procs))
	for pid, proc := range procs {
		hash := hashProcess(pid, proc.Stats.CreateTime)
		if entity, ok := w.cache[hash]; ok {
			newCache[hash] = entity

			// Sometimes the containerID can be late to initialize. If this is the case add it to the list of changed procs
			if cid, ok := w.pidToCid[int(proc.Pid)]; ok && entity.ContainerId == "" {
				entity.ContainerId = cid
				newEntities = append(newEntities, entity)
			}
			continue
		}

		newProcs = append(newProcs, proc)
	}

	deadProcs := getDifference(w.cache, newCache)

	// If no process has been created, terminated, or updated, there's no need to update the cache
	// or generate a new diff
	if len(newProcs) == 0 && len(deadProcs) == 0 && len(newEntities) == 0 {
		return
	}

	languages := languagedetection.DetectLanguage(newProcs)
	for i, lang := range languages {
		pid := newProcs[i].Pid
		proc := procs[pid]

		var creationTime int64
		if proc.Stats != nil {
			creationTime = proc.Stats.CreateTime
		}

		entity := &process.Entity{
			Pid:          pid,
			NsPid:        proc.NsPid,
			CreationTime: creationTime,
			Language:     lang,
			ContainerId:  w.pidToCid[int(pid)],
		}
		newEntities = append(newEntities, entity)
		newCache[hashProcess(pid, proc.Stats.CreateTime)] = entity

		log.Trace("detected language", lang.Name, "for pid", pid)
	}

	w.cacheMutex.Lock()
	w.cache = newCache
	w.cacheVersion++
	w.cacheMutex.Unlock()

	// Drop previous cache diff if it hasn't been consumed yet
	select {
	case <-w.diffChan:
		// drop message
		oldDiffDropped.Inc()
		log.Debug("Dropping old process diff in WorkloadMetaExtractor")
		break
	default:
	}

	diff := &process.CacheDiff{
		CacheVersion: w.cacheVersion,
		Creation:     newEntities,
		Deletion:     deadProcs,
	}

	// Do not block on write to prevent Extract caller from hanging e.g. process check
	select {
	case w.diffChan <- diff:
		break
	default:
		diffChanFull.Inc()
		log.Error("Dropping newer process diff in WorkloadMetaExtractor")
	}
}

func getDifference(oldCache, newCache map[string]*process.Entity) []*process.Entity {
	oldProcs := make([]*process.Entity, 0, len(oldCache))
	for key, entity := range oldCache {
		if _, ok := newCache[key]; ok {
			continue
		}
		oldProcs = append(oldProcs, entity)
	}
	return oldProcs
}

// Enabled returns whether the extractor should be enabled
func Enabled(ddconfig config.ConfigReader) bool {
	return ddconfig.GetBool("process_config.language_detection.enabled")
}

func hashProcess(pid int32, createTime int64) string {
	return "pid:" + strconv.Itoa(int(pid)) + "|createTime:" + strconv.Itoa(int(createTime))
}

// ListProcesses returns all processes Entities stored in the Extractor cache and the version
// of the cache at the moment of the read
func (w *Extractor) ListProcesses() (map[string]*process.Entity, int32) {
	w.cacheMutex.RLock()
	defer w.cacheMutex.RUnlock()

	// Store pointers in map to avoid duplicating ProcessEntity data
	snapshot := make(map[string]*process.Entity)
	for id, proc := range w.cache {
		snapshot[id] = proc
	}

	return snapshot, w.cacheVersion
}

// Subscribe returns a channel to consume process diffs from
func (w *Extractor) Subscribe() <-chan *process.CacheDiff {
	return w.diffChan
}

func (w *Extractor) reportTelemetry() {
	cacheSizeGuage.Set(float64(len(w.cache)))
}
