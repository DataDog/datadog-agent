// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"fmt"
	"strings"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Dump implements Store#Dump
func (w *workloadmeta) Dump(verbose bool) wmdef.WorkloadDumpResponse {
	return w.dump(verbose, "")
}

// DumpFiltered implements Store#DumpFiltered
func (w *workloadmeta) DumpFiltered(verbose bool, search string) wmdef.WorkloadDumpResponse {
	return w.dump(verbose, search)
}

// dump is the internal implementation that supports optional filtering
func (w *workloadmeta) dump(verbose bool, search string) wmdef.WorkloadDumpResponse {
	workloadList := wmdef.WorkloadDumpResponse{
		Entities: make(map[string]wmdef.WorkloadEntity),
	}

	entityToString := func(entity wmdef.Entity) (string, error) {
		var info string
		switch e := entity.(type) {
		case *wmdef.Container:
			info = e.String(verbose)
		case *wmdef.KubernetesPod:
			info = e.String(verbose)
		case *wmdef.ECSTask:
			info = e.String(verbose)
		case *wmdef.ContainerImageMetadata:
			info = e.String(verbose)
		case *wmdef.Process:
			info = e.String(verbose)
		case *wmdef.KubernetesDeployment:
			info = e.String(verbose)
		case *wmdef.KubernetesMetadata:
			info = e.String(verbose)
		case *wmdef.GPU:
			info = e.String(verbose)
		case *wmdef.Kubelet:
			info = e.String(verbose)
		case *wmdef.CRD:
			info = e.String(verbose)
		case *wmdef.KubeCapabilities:
			info = e.String(verbose)
		default:
			return "", fmt.Errorf("unsupported type %T", e)
		}

		return info, nil
	}

	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	for kind, store := range w.store {
		// Apply kind filter if search is provided
		kindStr := string(kind)
		if search != "" && !strings.Contains(kindStr, search) {
			// Kind doesn't match, check if any entities match by ID
			hasMatch := false
			for id := range store {
				if strings.Contains(id, search) {
					hasMatch = true
					break
				}
			}
			if !hasMatch {
				continue
			}
		}

		entities := wmdef.WorkloadEntity{Infos: make(map[string]string)}
		for id, cachedEntity := range store {
			// Apply entity ID filter if search is provided and kind didn't match
			if search != "" && !strings.Contains(kindStr, search) && !strings.Contains(id, search) {
				continue
			}

			if verbose && len(cachedEntity.sources) > 1 {
				for source, entity := range cachedEntity.sources {
					info, err := entityToString(entity)
					if err != nil {
						log.Debugf("Ignoring entity %s: %v", entity.GetID().ID, err)
						continue
					}

					entities.Infos["source:"+string(source)+" id: "+id] = info
				}
			}

			e := cachedEntity.cached
			info, err := entityToString(e)
			if err != nil {
				log.Debugf("Ignoring entity %s: %v", e.GetID().ID, err)
				continue
			}

			entities.Infos[fmt.Sprintf("sources(merged):%v", cachedEntity.sortedSources)+" id: "+id] = info
		}

		if len(entities.Infos) > 0 {
			workloadList.Entities[kindStr] = entities
		}
	}

	return workloadList
}

// DumpStructured implements Store#DumpStructured
func (w *workloadmeta) DumpStructured(verbose bool) wmdef.WorkloadDumpStructuredResponse {
	workloadList := wmdef.WorkloadDumpStructuredResponse{
		Entities: make(map[string][]wmdef.Entity),
	}

	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	for kind, store := range w.store {
		entities := make([]wmdef.Entity, 0, len(store))
		for _, cachedEntity := range store {
			// For verbose mode, include all source entities
			if verbose && len(cachedEntity.sources) > 1 {
				for _, entity := range cachedEntity.sources {
					entities = append(entities, entity)
				}
			}

			// Always include the merged entity
			entities = append(entities, cachedEntity.cached)
		}

		if len(entities) > 0 {
			workloadList.Entities[string(kind)] = entities
		}
	}

	return workloadList
}
