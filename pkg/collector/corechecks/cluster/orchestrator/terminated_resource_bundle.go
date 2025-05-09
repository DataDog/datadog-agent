// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import (
	"reflect"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TerminatedResourceBundle buffers terminated resources
type TerminatedResourceBundle struct {
	mu                  sync.Mutex
	runCfg              *collectors.CollectorRunConfig
	check               *OrchestratorCheck
	terminatedResources map[collectors.K8sCollector][]interface{}
	manifestBuffer      *ManifestBuffer
}

// NewTerminatedResourceBundle returns a TerminatedResourceBundle
func NewTerminatedResourceBundle(check *OrchestratorCheck, runCfg *collectors.CollectorRunConfig, manifestBuffer *ManifestBuffer) *TerminatedResourceBundle {
	tb := &TerminatedResourceBundle{
		check:               check,
		runCfg:              runCfg,
		terminatedResources: make(map[collectors.K8sCollector][]interface{}, 20),
		manifestBuffer:      manifestBuffer,
	}
	return tb
}

// Add adds a terminated object into TerminatedResourceBundle
func (tb *TerminatedResourceBundle) Add(k8sCollector collectors.K8sCollector, resource interface{}) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if _, ok := tb.terminatedResources[k8sCollector]; !ok {
		tb.terminatedResources[k8sCollector] = []interface{}{}
	}

	tb.terminatedResources[k8sCollector] = append(tb.terminatedResources[k8sCollector], resource)
}

// Run sends all buffered terminated resources
func (tb *TerminatedResourceBundle) Run() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	orchSender, err := tb.check.GetSender()
	if err != nil {
		_ = tb.check.Warnf("Failed to get sender: %s", err)
		return
	}

	for collector := range tb.terminatedResources {
		if len(tb.terminatedResources[collector]) == 0 {
			continue
		}

		result, err := collector.Process(tb.runCfg, toTypedSlice(collector, tb.terminatedResources[collector]))
		if err != nil {
			log.Warnf("Failed to process deletion event: %s", err)
			return
		}

		log.Debugf("Terminated Resource collector %s run stats: listed=%d processed=%d messages=%d", collector.Metadata().FullName(), result.ResourcesListed, result.ResourcesProcessed, len(result.Result.MetadataMessages))

		nt := collector.Metadata().NodeType
		orchestrator.SetCacheStats(result.ResourcesListed, len(result.Result.MetadataMessages), nt)

		if collector.Metadata().IsMetadataProducer { // for CR and CRD we don't have metadata but only manifests
			orchSender.OrchestratorMetadata(result.Result.MetadataMessages, tb.runCfg.ClusterID, int(nt))
		}

		if collector.Metadata().SupportsManifestBuffering {
			BufferManifestProcessResult(result.Result.ManifestMessages, tb.manifestBuffer)
		} else {
			orchSender.OrchestratorManifest(result.Result.ManifestMessages, tb.runCfg.ClusterID)
		}

		tb.terminatedResources[collector] = tb.terminatedResources[collector][:0]
	}
}

// Stop stops TerminatedResourceBundle
func (tb *TerminatedResourceBundle) Stop() {
	// send all buffered terminated resources
	tb.Run()
}

func toTypedSlice(k8sCollector collectors.K8sCollector, list []interface{}) interface{} {
	if len(list) == 0 {
		return nil
	}

	if k8sCollector.Metadata().NodeType == orchestrator.K8sCR || k8sCollector.Metadata().NodeType == orchestrator.K8sCRD {
		typedList := make([]runtime.Object, 0, len(list))
		for i := range list {
			if _, ok := list[i].(runtime.Object); !ok {
				log.Warn("Failed to convert object to runtime.Object")
				continue
			}
			typedList = append(typedList, list[i].(runtime.Object))
		}
		return typedList
	}

	// Create a new slice with the type of the object
	objType := reflect.TypeOf(list[0])
	typedList := reflect.MakeSlice(reflect.SliceOf(objType), 0, len(list))

	for i := range list {
		typedList = reflect.Append(typedList, reflect.ValueOf(list[i]))
	}
	return typedList.Interface()
}
