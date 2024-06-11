// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"fmt"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Dump implements Store#Dump
func (w *workloadmeta) Dump(verbose bool) wmdef.WorkloadDumpResponse {
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
		case *wmdef.KubernetesNode:
			info = e.String(verbose)
		case *wmdef.ECSTask:
			info = e.String(verbose)
		case *wmdef.ContainerImageMetadata:
			info = e.String(verbose)
		case *wmdef.Process:
			info = e.String(verbose)
		case *wmdef.KubernetesDeployment:
			info = e.String(verbose)
		case *wmdef.KubernetesNamespace:
			info = e.String(verbose)
		case *wmdef.KubernetesMetadata:
			info = e.String(verbose)
		default:
			return "", fmt.Errorf("unsupported type %T", e)
		}

		return info, nil
	}

	w.storeMut.RLock()
	defer w.storeMut.RUnlock()

	for kind, store := range w.store {
		entities := wmdef.WorkloadEntity{Infos: make(map[string]string)}
		for id, cachedEntity := range store {
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

		workloadList.Entities[string(kind)] = entities
	}

	return workloadList
}
