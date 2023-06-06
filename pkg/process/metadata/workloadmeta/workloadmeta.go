// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// ProcessEntity is a placeholder workloadmeta.ProcessEntity.
// It does not contain all the fields that the final entity will contain.
type ProcessEntity struct {
	pid      int32
	cmdline  []string
	language *languagedetection.Language
}

// WorkloadMetaExtractor handles enriching processes with
type WorkloadMetaExtractor struct {
	cache      map[string]*ProcessEntity
	cacheMutex sync.RWMutex

	grpcListener mockableGrpcListener
}

// NewWorkloadMetaExtractor constructs the WorkloadMetaExtractor.
func NewWorkloadMetaExtractor() *WorkloadMetaExtractor {
	return &WorkloadMetaExtractor{
		cache:        make(map[string]*ProcessEntity),
		grpcListener: newGrpcListener(),
	}
}

// Extract detects the process language, creates a process entity, and sends that entity to WorkloadMeta
func (w *WorkloadMetaExtractor) Extract(procs map[int32]*procutil.Process) {
	procsToDetect := make([]*languagedetection.Process, 0, len(procs))
	newCache := make(map[string]*ProcessEntity, len(procs))
	for pid, proc := range procs {
		hash := hashProcess(pid, proc.Stats.CreateTime)
		if entity, ok := w.cache[hash]; ok {
			newCache[hash] = entity
			continue
		}

		procsToDetect = append(procsToDetect, &languagedetection.Process{
			Pid:     pid,
			Cmdline: proc.Cmdline,
		})
	}

	newProcs := make([]*ProcessEntity, 0, len(procsToDetect))
	languages := languagedetection.DetectLanguage(procsToDetect)
	for i, lang := range languages {
		pid := procsToDetect[i].Pid
		proc := procs[procsToDetect[i].Pid]
		entity := &ProcessEntity{
			pid:      pid,
			cmdline:  proc.Cmdline,
			language: lang,
		}
		newProcs = append(newProcs, entity)
		newCache[hashProcess(pid, proc.Stats.CreateTime)] = entity
	}

	oldProcs := getOldProcs(w.cache, newCache)
	w.grpcListener.writeEvents(oldProcs, newProcs)

	w.cacheMutex.Lock()
	w.cache = newCache
	w.cacheMutex.Unlock()
}

func getOldProcs(oldCache, newCache map[string]*ProcessEntity) []*ProcessEntity {
	oldProcs := make([]*ProcessEntity, 0, len(oldCache))
	for key, entity := range oldCache {
		if _, ok := newCache[key]; ok {
			continue
		}
		oldProcs = append(oldProcs, entity)
	}
	return oldProcs
}

// Enabled returns wheither or not the extractor should be enabled
func Enabled(ddconfig config.ConfigReader) bool {
	return ddconfig.GetBool("process_config.language_detection.enabled")
}

func hashProcess(pid int32, createTime int64) string {
	return fmt.Sprintf("pid:%v|createTime:%v", pid, createTime)
}
