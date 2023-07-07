// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessEntity represents a process exposed by the WorkloadMeta extractor
type ProcessEntity struct {
	Pid          int32
	NsPid        int32
	CreationTime int64
	Language     *languagemodels.Language
}

// WorkloadMetaExtractor does these two things:
//   - Detecting the language of new processes and sending them to WorkloadMeta
//   - Detecting the processes that terminate and sending their PID to WorkloadMeta
type WorkloadMetaExtractor struct {
	// Cache is a map from process hash to the workloadmeta entity
	// The cache key takes the form of `pid:<pid>|createTime:<createTime>`. See hashProcess
	cache        map[string]*ProcessEntity
	cacheVersion int32
	cacheMutex   sync.RWMutex

	diffChan chan *ProcessCacheDiff
}

// ProcessCacheDiff holds the information about processes that have been created and deleted in the past
// Extract call from the WorkloadMetaExtractor cache
type ProcessCacheDiff struct {
	cacheVersion int32
	creation     []*ProcessEntity
	deletion     []*ProcessEntity
}

// NewWorkloadMetaExtractor constructs the WorkloadMetaExtractor.
func NewWorkloadMetaExtractor(config config.ConfigReader) *WorkloadMetaExtractor {
	log.Info("Instantiating a new WorkloadMetaExtractor")
	return &WorkloadMetaExtractor{
		cache:        make(map[string]*ProcessEntity),
		cacheVersion: 0,
		// Keep only the latest diff in memory in case there's no consumer for it
		diffChan: make(chan *ProcessCacheDiff, 1),
	}
}

// Extract detects the process language, creates a process entity, and sends that entity to WorkloadMeta
func (w *WorkloadMetaExtractor) Extract(procs map[int32]*procutil.Process) {
	newProcs := make([]*procutil.Process, 0, len(procs))
	newCache := make(map[string]*ProcessEntity, len(procs))
	for pid, proc := range procs {
		hash := hashProcess(pid, proc.Stats.CreateTime)
		if entity, ok := w.cache[hash]; ok {
			newCache[hash] = entity
			continue
		}

		newProcs = append(newProcs, proc)
	}

	deadProcs := getDifference(w.cache, newCache)

	// If no process has been created or terminated, there's no need to update the cache
	// or generate a new diff
	if len(newProcs) == 0 && len(deadProcs) == 0 {
		return
	}

	newEntities := make([]*ProcessEntity, 0, len(newProcs))
	languages := languagedetection.DetectLanguage(newProcs)
	for i, lang := range languages {
		pid := newProcs[i].Pid
		proc := procs[pid]

		var creationTime int64
		if proc.Stats != nil {
			creationTime = proc.Stats.CreateTime
		}

		entity := &ProcessEntity{
			Pid:          pid,
			NsPid:        proc.NsPid,
			CreationTime: creationTime,
			Language:     lang,
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
		log.Debug("Dropping old process diff in WorkloadMetaExtractor")
		break
	default:
	}

	diff := &ProcessCacheDiff{
		cacheVersion: w.cacheVersion,
		creation:     newEntities,
		deletion:     deadProcs,
	}

	// Do not block on write to prevent Extract caller from hanging e.g. process check
	select {
	case w.diffChan <- diff:
		break
	default:
		log.Error("Dropping newer process diff in WorkloadMetaExtractor")
	}
}

func getDifference(oldCache, newCache map[string]*ProcessEntity) []*ProcessEntity {
	oldProcs := make([]*ProcessEntity, 0, len(oldCache))
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

// GetAllProcessEntities returns all processes Entities stored in the WorkloadMetaExtractor cache and the version
// of the cache at the moment of the read
func (w *WorkloadMetaExtractor) GetAllProcessEntities() (map[string]*ProcessEntity, int32) {
	w.cacheMutex.RLock()
	defer w.cacheMutex.RUnlock()

	// Store pointers in map to avoid duplicating ProcessEntity data
	snapshot := make(map[string]*ProcessEntity)
	for id, proc := range w.cache {
		snapshot[id] = proc
	}

	return snapshot, w.cacheVersion
}

// ProcessCacheDiff returns a channel to consume process diffs from
func (w *WorkloadMetaExtractor) ProcessCacheDiff() <-chan *ProcessCacheDiff {
	return w.diffChan
}
